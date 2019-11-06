// Copyright (c) 2019 The kaleido Authors
// This file is part of kaleido
//
// kaleido is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// kaleido is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with kaleido. If not, see <https://www.gnu.org/licenses/>.

package core

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/kaleidochain/kaleido/crypto/ed25519"

	"github.com/kaleidochain/kaleido/core"

	"github.com/kaleidochain/kaleido/core/types"

	"github.com/ethereum/go-ethereum/log"

	"github.com/kaleidochain/kaleido/common"
)

type StateEvent int

const (
	Unchanged StateEvent = 0
	NewRound  StateEvent = 1
	HeightEnd StateEvent = 2
)

func (se StateEvent) IsChanged() bool {
	return se != Unchanged
}

// State defines interfaces of state
type State interface {
	SetContext(c *Context)
	OnEnter()
	OnExit()
	OnTimeout()
	OnPotentialVoteEnough(value common.Hash, round uint32, step uint32)
	OnSoftVoteEnough(value common.Hash, round uint32)
	OnCertVoteEnough(value common.Hash, round uint32) StateEvent
	OnNextVoteEnough(value common.Hash, round uint32, step uint32) StateEvent
	OnProposalLeaderReceived(value *ProposalLeaderData)
	OnProposalDataComplete(data *ProposalBlockData) StateEvent
	String() string
}

//-------------------------------------

// StateBase is the base of all states
type StateBase struct {
	*Context
	sortitionWeight uint64
	sortitionHash   ed25519.VrfOutput256
	sortitionProof  ed25519.VrfProof
}

func (s *StateBase) String() string {
	return "StateBase"
}

func (s *StateBase) SetContext(c *Context) {
	s.Context = c
}

func (s *StateBase) OnEnter() {}

func (s *StateBase) OnExit() {}

func (s *StateBase) OnTimeout() {}

func (s *StateBase) OnPotentialVoteEnough(value common.Hash, round, step uint32) {
	if blockData := s.getProposalBlock(value); blockData != nil {
		s.verifyBlockAsync(blockData.Block)
	}
}

func (s *StateBase) OnSoftVoteEnough(value common.Hash, round uint32) {}

func (s *StateBase) OnCertVoteEnough(value common.Hash, round uint32) StateEvent {
	blockData := s.getProposalBlock(value)
	if blockData == nil {
		log.Debug("OnCertVoteEnough no blockData exists",
			"HRS", s.HRS(),
			"cert-Round", round,
			"value", value)

		// waiting until blockData of value received
		s.WaitingBlockForValue = value
		s.WaitingBlockForRound = round
		return Unchanged
	}

	log.Debug("StateMachine OnCertVoteEnough",
		"HRS", s.HRS(),
		"CertifiedValue", value,
		"ForRound", round,
		"BlockRound", blockData.Round)

	header := blockData.Block.Header()
	height := header.Number.Uint64()

	// make block's certificates
	cvs := s.counter.GetCertVoteStorage(round, value)
	sort.Sort(types.CertVoteStorageSlice(cvs))
	if len(cvs) == 0 {
		log.Error("Unexpected error, empty cvs", "value", value, "block", blockData)
		panic("Unexpected error getting cvs or leaderProposalValue")
	}

	proof := types.NewNodeSet()
	err := core.BuildProof(s.config.Algorand, s.parentStatedb, s.parent.Root(), height, blockData.Address, cvs, proof)
	if err != nil {
		log.Error("build proof failed", "err", err)
		panic(fmt.Sprintf("build proof failed: %v", err))
		return Unchanged
	}

	header.Certificate = &types.Certificate{
		Height:      height,
		Round:       round,
		Value:       value,
		Proposal:    blockData.ToProposalStorage(),
		CertVoteSet: cvs,
		TrieProof:   proof.NodeList(),
	}

	sealedBlock := blockData.Block.WithSeal(header)

	err = s.Context.insertIntoBlockChain(sealedBlock)
	switch err {
	case nil:
	case core.ErrKnownBlock:
		log.Debug("Sealed block is preempt by sync",
			"hash", sealedBlock.Hash(), "number", sealedBlock.NumberU64())
	default:
		log.Error("Unexpected error",
			"err", err,
			"block", sealedBlock)
		panic("Unexpected error on insert into blockchain")
	}

	bywho := "other"
	if s.newBlock != nil && s.newBlock.Hash() == value {
		bywho = "me"
	}
	log.Info("🔨 mined new block proposed by "+bywho,
		"number", sealedBlock.Number(), "hash", sealedBlock.Hash(),
		"txs", len(sealedBlock.Transactions()), "gas", sealedBlock.GasUsed(),
		"elapsed", common.PrettyDuration(time.Since(s.startTime)),
		"coinbase", sealedBlock.Coinbase(),
		"leader", blockData.Address,
		"cert-round", round,
		"proposal-round", blockData.Round,
	)

	return HeightEnd
}

func (s *StateBase) OnNextVoteEnough(value common.Hash, round uint32, step uint32) StateEvent {
	if round >= s.Round {
		// goto round `round + 1`
		log.Debug("StateMachine NextVoteEnough for New Round",
			"HRS", s.HRS(),
			"NewRound", round+1,
			"StartingValue", value)

		s.gotoState(&StateProposal{stateRound: round + 1, stateStartingValue: value})
		return NewRound
	}
	return Unchanged
}

// ProposalValue only accept in Step 1.
// 一旦Step1结束了，leader就确定了，也就无需再处理本轮的其它ProposalValue了。
// 同时，确保了在Step 2/3/... 期间传播ProposalData是最终leader的data。
// 这样，就确保了传播出某个Value的SoftVote的节点，一定能传播出对应于该值的Data。
func (s *StateBase) OnProposalLeaderReceived(value *ProposalLeaderData) {
	if value.Round < s.Round || (value.Round == s.Round && s.Step != types.RoundStep1Proposal) {
		// keep leader immutable after Step1 in each round
		return
	}

	if leader := s.getProposalLeader(value.Round); leader == nil || value.LessThan(leader) {
		log.Debug("StateMachine 1 Leader Proposal Value Updated", "HRS", s.HRS(),
			"ProposalLeaderValue", value.Value)
		s.UpdateProposalLeader(value)
	}
}
func (s *StateBase) OnProposalDataComplete(data *ProposalBlockData) StateEvent {
	if s.WaitingBlockForValue == data.Block.Hash() {
		if err := s.verifyBlockSync(data.Block); err == nil {
			return s.OnCertVoteEnough(s.WaitingBlockForValue, s.WaitingBlockForRound)
		}
	}
	return Unchanged
}

//-------------------------------------

// StateNewHeight is the first state of each Height
type StateNewHeight struct {
	StateBase
	chainHeadBlock *types.Block
}

func (s *StateNewHeight) String() string {
	return "StateNewHeight"
}

// OnEnter initializes context for new height
func (s *StateNewHeight) OnEnter() {
	parent := s.chainHeadBlock
	parentStatedb, err := s.eth.BlockChain().StateAt(parent.Root())
	if err != nil {
		panic(fmt.Sprintf("Failed to get statedb, err:%s", err)) // debug
		return
	}

	// init HeightState
	s.hsMutex.Lock()

	s.currentMiner = s.nextMiner

	s.startTime = time.Now()

	num := parent.Number()
	emptyHeader := &types.Header{
		ParentHash: parent.Hash(),
		Number:     num.Add(num, common.Big1),
	}
	emptyValue := emptyHeader.AlgorandEmptyValue()

	s.parent = parent
	s.parentStatedb = parentStatedb
	s.resetParentRoundVoteSet()
	s.resetParentProposalBlockData()

	s.header = emptyHeader

	s.Height = emptyHeader.Number.Uint64()
	s.Round = types.BadRound
	s.Step = types.BadStep

	s.EmptyValue = emptyValue
	s.StartingValue = emptyValue

	s.counter = NewHeightVoteSet()

	s.resetProposalLeader()
	s.resetReceivedProposalBlock()
	s.verifingBlockState = make(map[string]chan *blockState)
	s.verifiedBlockState = make(map[string]*blockState)

	s.WaitingBlockForValue = emptyValue
	s.WaitingBlockForRound = types.BadRound

	s.statedb = nil
	s.receipts = nil
	s.mortgageContractCounter = nil
	s.newBlock = nil

	if parent.NumberU64() > 1000 {
		before1000File := fmt.Sprintf("%s/msg/%d", s.dataDir, parent.NumberU64()-1000)
		_ = os.Remove(before1000File)
	}

	s.hsMutex.Unlock()

	log.Trace("StateMachine NewHeight", "HRS", s.HRS())

	s.gotoState(&StateProposal{stateRound: 1, stateStartingValue: s.EmptyValue})
}

//-------------------------------------

// StateProposal is the first state of each Round
type StateProposal struct {
	StateBase
	stateRound         uint32
	stateStartingValue common.Hash
}

func (s *StateProposal) String() string {
	return "StateProposal"
}

// OnEnter initializes for new Round, and proposes
func (s *StateProposal) OnEnter() {
	s.hsMutex.Lock()
	s.Round = s.stateRound
	s.StartingValue = s.stateStartingValue
	s.Step = types.RoundStep1Proposal
	s.hsMutex.Unlock()

	log.Trace("StateMachine Entering StateProposal",
		"HRS", s.HRS(), "StartingValue", s.StartingValue)

	s.broadcastMsg(StatusMsg, &StatusData{s.Height, s.Round})

	s.startTimer()

	s.sortitionHash, s.sortitionProof, s.sortitionWeight = s.sortition()
	if s.sortitionWeight == 0 {
		log.Trace("StateMachine 1 sortitionWeight == 0",
			"HRS", s.HRS())
		return
	}

	if s.Round == 1 || s.isNextVoteEnoughForEmpty(s.Round-1) {
		if err := s.NewBlock(); err != nil {
			log.Error("StateMachine 1.1 Create new block fail",
				"HRS", s.HRS(), "err", err)
			if err != errStopped {
				panic(err) // 暴露问题
			}
			return // the only thing we can do is abort proposal
		}

		log.Trace("StateMachine 1.1 Send Proposal",
			"HRS", s.HRS(),
			"SelfValue", s.newBlock.Hash())
		s.sendProposal(s.newBlock.Hash(), s.sortitionHash, s.sortitionProof, s.sortitionWeight)
	} else { // the V must exist and must be the StartingValue
		log.Trace("StateMachine 1.2 Send Proposal",
			"HRS", s.HRS(), "StartingValue", s.StartingValue)
		s.sendProposal(s.StartingValue, s.sortitionHash, s.sortitionProof, s.sortitionWeight)
	}
}

func (s *StateProposal) OnExit() {
	s.clearTimer()
}

func (s *StateProposal) OnTimeout() {
	s.gotoState(&StateFiltering{})
}

//-------------------------------------

// StateFiltering sends soft vote
type StateFiltering struct {
	StateBase
}

func (s *StateFiltering) String() string {
	return "StateFiltering"
}

func (s *StateFiltering) OnEnter() {
	s.Step = types.RoundStep2Filtering

	log.Trace("StateMachine Entering StateFiltering", "HRS", s.HRS())

	s.sortitionHash, s.sortitionProof, s.sortitionWeight = s.sortition()
	if s.sortitionWeight == 0 {
		log.Trace("StateMachine 2 sortitionWeight == 0",
			"HRS", s.HRS())
		s.gotoState(&StateCertifying{})
		return
	}

	if s.Round == 1 || s.isNextVoteEnoughForEmpty(s.Round-1) {
		if value := s.getProposalLeader(s.Round); value != nil {
			log.Trace("StateMachine 2.1 Send SoftVote",
				"HRS", s.HRS(), "ProposalLeaderValue", value.Value,
				"Weight", s.sortitionWeight)
			s.sendVote(types.VoteTypeSoft, value.Value, s.sortitionHash, s.sortitionProof, s.sortitionWeight)
		}
	} else { // the V must exist and must be the StartingValue
		log.Trace("StateMachine 2.2 Send SoftVote",
			"HRS", s.HRS(), "StartingValue", s.StartingValue,
			"Weight", s.sortitionWeight)
		s.sendVote(types.VoteTypeSoft, s.StartingValue, s.sortitionHash, s.sortitionProof, s.sortitionWeight)
	}

	s.gotoState(&StateCertifying{})
}

//-------------------------------------

// StateCertifying try to send cert vote
type StateCertifying struct {
	StateBase
	certWaiting common.Hash
}

func (s *StateCertifying) String() string {
	return "StateCertifying"
}

func (s *StateCertifying) OnEnter() {
	s.Step = types.RoundStep3Certifying
	s.certWaiting = common.Hash{}

	log.Trace("StateMachine Entering StateCertifying", "HRS", s.HRS())

	s.startTimer()

	s.sortitionHash, s.sortitionProof, s.sortitionWeight = s.sortition()
	if s.sortitionWeight == 0 {
		log.Trace("StateMachine 3 sortitionWeight == 0",
			"HRS", s.HRS())
		return
	}

	exists, nonEmptyValue := s.counter.IsSoftVoteEnoughForNonEmpty(s.EmptyValue, s.Round)
	if exists {
		s.tryCertValue(nonEmptyValue)
	}
}

func (s *StateCertifying) OnExit() {
	s.clearTimer()
}

func (s *StateCertifying) OnSoftVoteEnough(value common.Hash, round uint32) {
	if s.sortitionWeight > 0 && round == s.Round && value != s.EmptyValue {
		s.tryCertValue(value)
	}
}

func (s *StateCertifying) tryCertValue(value common.Hash) {
	data := s.getProposalBlock(value)
	if data != nil {
		if err := s.verifyBlockSync(data.Block); err != nil {
			log.Warn("StateMachine 3 Verify Block Error",
				"HRS", s.HRS(), "CertifiedValue", value, "err", err)
			return
		}

		log.Trace("StateMachine 3 Send CertVote",
			"HRS", s.HRS(), "CertifiedValue", value,
			"Weight", s.sortitionWeight)
		s.sendVote(types.VoteTypeCert, value, s.sortitionHash, s.sortitionProof, s.sortitionWeight)
	} else {
		log.Trace("StateMachine 3 Hold CertVote Waiting for data",
			"HRS", s.HRS(), "CertifiedValue", value,
			"Weight", s.sortitionWeight)
		s.certWaiting = value
		s.broadcaster.Request(makeChanId(s.Height, s.Round), value)
	}
}

func (s *StateCertifying) OnProposalDataComplete(data *ProposalBlockData) StateEvent {
	if event := s.StateBase.OnProposalDataComplete(data); event.IsChanged() {
		return event
	}

	// s.certWaiting keeps nil if s.sortitionWeight == 0
	if !common.EmptyHash(s.certWaiting) && s.certWaiting == data.Block.Hash() {
		if err := s.verifyBlockSync(data.Block); err != nil {
			log.Warn("StateMachine 3 Verify Block Error",
				"HRS", s.HRS(), "CertifiedValue", s.certWaiting, "err", err)
			return Unchanged
		}

		log.Debug("StateMachine 3 Send CertVote After Waiting",
			"HRS", s.HRS(), "CertifiedValue", s.certWaiting,
			"Weight", s.sortitionWeight)
		s.sendVote(types.VoteTypeCert, s.certWaiting, s.sortitionHash, s.sortitionProof, s.sortitionWeight)
	}

	return Unchanged
}

func (s *StateCertifying) OnTimeout() {
	s.gotoState(&StateFirstFinishing{})
}

//-------------------------------------

type StateFirstFinishing struct {
	StateBase
}

func (s *StateFirstFinishing) String() string {
	return "StateFirstFinishing"
}

func (s *StateFirstFinishing) OnEnter() {
	s.Step = s.Step + 1

	log.Trace("StateMachine Entering StateFirstFinishing", "HRS", s.HRS())

	s.sortitionHash, s.sortitionProof, s.sortitionWeight = s.sortition()
	if s.sortitionWeight == 0 {
		log.Trace("StateMachine 4 sortitionWeight == 0",
			"HRS", s.HRS())
		s.gotoState(&StateSecondFinishing{})
		return
	}

	if exists, value := s.isSoftVoteEnoughForNonEmptyValueWhoseBlockIsReceivedAndValid(s.Round); exists {
		log.Trace("StateMachine 4.1 Send NextVote",
			"HRS", s.HRS(), "NonEmptyValue", value,
			"Weight", s.sortitionWeight)
		s.sendVote(types.VoteTypeNext, value, s.sortitionHash, s.sortitionProof, s.sortitionWeight)
	} else if s.Round >= 2 && s.isNextVoteEnoughForEmpty(s.Round-1) {
		log.Trace("StateMachine 4.2 Send NextVote",
			"HRS", s.HRS(), "EmptyValue", s.EmptyValue,
			"Weight", s.sortitionWeight)
		s.sendVote(types.VoteTypeNext, s.EmptyValue, s.sortitionHash, s.sortitionProof, s.sortitionWeight)
	} else {
		log.Trace("StateMachine 4.3 Send NextVote",
			"HRS", s.HRS(), "StartingValue", s.StartingValue,
			"Weight", s.sortitionWeight)
		s.sendVote(types.VoteTypeNext, s.StartingValue, s.sortitionHash, s.sortitionProof, s.sortitionWeight)
	}

	s.gotoState(&StateSecondFinishing{})
}

//-------------------------------------

type StateSecondFinishing struct {
	StateBase
	waitingBlockOfValueForNextVote common.Hash
}

func (s *StateSecondFinishing) String() string {
	return "StateSecondFinishing"
}

func (s *StateSecondFinishing) OnEnter() {
	s.Step = s.Step + 1

	log.Trace("StateMachine Entering StateSecondFinishing", "HRS", s.HRS())

	s.startTimer()

	s.sortitionHash, s.sortitionProof, s.sortitionWeight = s.sortition()
	if s.sortitionWeight == 0 {
		log.Trace("StateMachine 5 sortitionWeight == 0",
			"HRS", s.HRS())
		return
	}
}

func (s *StateSecondFinishing) OnExit() {
	s.clearTimer()
}

func (s *StateSecondFinishing) OnSoftVoteEnough(value common.Hash, round uint32) {
	if s.sortitionWeight > 0 && round == s.Round && value != s.EmptyValue {
		block := s.getProposalBlock(value)
		if block == nil {
			log.Trace("StateMachine 5.1 Hold NextVote waiting for block of value",
				"HRS", s.HRS(), "value", value)
			s.waitingBlockOfValueForNextVote = value
			return
		}

		if err := s.verifyBlockSync(block.Block); err != nil {
			log.Warn("StateMachine 5.1 Verify Block Error",
				"HRS", s.HRS(), "value", value, "err", err)
			return
		}

		log.Trace("StateMachine 5.1 Send NextVote",
			"HRS", s.HRS(), "value", value,
			"Weight", s.sortitionWeight)
		s.sendVote(types.VoteTypeNext, value, s.sortitionHash, s.sortitionProof, s.sortitionWeight)
	}
}

func (s *StateSecondFinishing) OnProposalDataComplete(data *ProposalBlockData) StateEvent {
	if event := s.StateBase.OnProposalDataComplete(data); event.IsChanged() {
		return event
	}

	if value := data.Block.Hash(); value == s.waitingBlockOfValueForNextVote {
		if err := s.verifyBlockSync(data.Block); err != nil {
			log.Warn("StateMachine 5 Verify Block Error", "HRS", s.HRS(),
				"waitingValue", s.waitingBlockOfValueForNextVote, "err", err)
			return Unchanged
		}

		log.Trace("StateMachine 5.1 Send NextVote",
			"HRS", s.HRS(), "value", value,
			"Weight", s.sortitionWeight)
		s.sendVote(types.VoteTypeNext, value, s.sortitionHash, s.sortitionProof, s.sortitionWeight)
	}
	return Unchanged
}

func (s *StateSecondFinishing) OnNextVoteEnough(value common.Hash, round uint32, step uint32) StateEvent {
	if event := s.StateBase.OnNextVoteEnough(value, round, step); event.IsChanged() {
		return event
	}

	if s.sortitionWeight > 0 && s.Round >= 2 && s.Round-1 == round && value == s.EmptyValue {
		if exists, _ := s.isSoftVoteEnoughForNonEmptyValueWhoseBlockIsReceivedAndValid(s.Round); !exists {
			log.Trace("StateMachine 5.2 Send NextVote",
				"HRS", s.HRS(), "EmptyValue", s.EmptyValue,
				"Weight", s.sortitionWeight)
			s.sendVote(types.VoteTypeNext, s.EmptyValue, s.sortitionHash, s.sortitionProof, s.sortitionWeight)
		}
	}
	return Unchanged
}

func (s *StateSecondFinishing) OnTimeout() {
	s.gotoState(&StateFirstFinishing{})
}
