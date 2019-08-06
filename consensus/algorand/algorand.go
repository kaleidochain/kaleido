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

package algorand

import (
	"fmt"
	"math/big"
	"runtime"
	"time"

	"github.com/pkg/errors"

	"github.com/kaleidochain/kaleido/consensus/algorand/core"

	"github.com/ethereum/go-ethereum/log"
	"github.com/kaleidochain/kaleido/node"
	"github.com/kaleidochain/kaleido/params"

	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/consensus"
	core2 "github.com/kaleidochain/kaleido/core"
	"github.com/kaleidochain/kaleido/core/state"
	"github.com/kaleidochain/kaleido/core/types"
	"github.com/kaleidochain/kaleido/ethdb"
	"github.com/kaleidochain/kaleido/rpc"
)

var (
	allowedFutureBlockTime = 15 * time.Second
)

// Algorand is a consensus engine based byzantine problem
type Algorand struct {
	ctx             *node.ServiceContext
	config          *params.ChainConfig
	db              ethdb.Database
	protocolManager *ProtocolManager
}

func New(ctx *node.ServiceContext, chainConfig *params.ChainConfig, db ethdb.Database) *Algorand {
	algorand := &Algorand{
		ctx:    ctx,
		config: chainConfig,
		db:     db,
	}

	return algorand
}

// -------------

func (ar *Algorand) Author(header *types.Header) (common.Address, error) {
	return header.Certificate.Proposer(), nil
}

func (ar *Algorand) Coinbase(header *types.Header) common.Address {
	return header.Coinbase
}

// VerifyHeader checks whether a header conforms to the consensus rules of a
// given engine. Verifying the seal may be done optionally here, or explicitly
// via the VerifySeal method.
func (ar *Algorand) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {
	log.Trace("VerifyHeader",
		"height", header.Number.Uint64(),
		"hash", header.Hash(),
		"seal", seal)

	// If we're running a full engine faking, accept any input as valid
	if false /*&& 0 == ModeFullFake*/ {
		return nil
	}
	// Short circuit if the header is known, or it's parent not
	number := header.Number.Uint64()
	if chain.GetHeader(header.Hash(), number) != nil {
		return nil
	}
	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	// Sanity checks passed, do a proper verification
	return ar.verifyHeader(chain, header, parent, false, seal)
}

func (ar *Algorand) verifyHeader(chain consensus.ChainReader, header, parent *types.Header, uncle bool, seal bool) error {
	// Verify Version
	if header.Version() != params.ConsensusVersion {
		return fmt.Errorf("version(nonce) must be %d", params.ConsensusVersion)
	}
	// Ensure difficulty is 1, so td is equal to number
	if header.Difficulty.Cmp(common.Big1) != 0 {
		return fmt.Errorf("difficulty must be 1")
	}
	// Ensure that the header's extra-data section is of a reasonable size
	if uint64(len(header.Extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("extra-data too long: %d > %d", len(header.Extra), params.MaximumExtraDataSize)
	}
	// Verify the header's timestamp
	if header.Time > uint64(time.Now().Add(allowedFutureBlockTime).Unix()) {
		return consensus.ErrFutureBlock
	}
	if header.Time <= parent.Time {
		return fmt.Errorf("timestamp equals to parent's")
	}

	// Verify that the gas limit is <= 2^63-1
	maxGasLimit := uint64(0x7fffffffffffffff)
	if header.GasLimit > maxGasLimit {
		return fmt.Errorf("invalid gasLimit: have %v, max %v", header.GasLimit, maxGasLimit)
	}
	// Verify that the gasUsed is <= gasLimit
	if header.GasUsed > header.GasLimit {
		return fmt.Errorf("invalid gasUsed: have %d, gasLimit %d", header.GasUsed, header.GasLimit)
	}

	// Verify that the gas limit remains within allowed bounds
	diff := int64(parent.GasLimit) - int64(header.GasLimit)
	if diff < 0 {
		diff *= -1
	}
	limit := parent.GasLimit / params.GasLimitBoundDivisor

	if uint64(diff) >= limit || header.GasLimit < params.MinGasLimit {
		return fmt.Errorf("invalid gas limit: have %d, want %d += %d", header.GasLimit, parent.GasLimit, limit)
	}
	// Verify that the block number is parent's +1
	if diff := new(big.Int).Sub(header.Number, parent.Number); diff.Cmp(big.NewInt(1)) != 0 {
		return consensus.ErrInvalidNumber
	}

	// Verify the engine specific seal securing the block
	if seal {
		if err := ar.VerifySeal(chain, header, parent); err != nil {
			return err
		}
	}

	// all checks passed
	return nil
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
// concurrently. The method returns a quit channel to abort the operations and
// a results channel to retrieve the async verifications (the order is that of
// the input slice).
func (ar *Algorand) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	if false { // fake one
		abort, results := make(chan struct{}), make(chan error, len(headers))
		for i := 0; i < len(headers); i++ {
			results <- nil
		}
		return abort, results
	}

	// Spawn as many workers as allowed threads
	workers := runtime.GOMAXPROCS(0)
	if len(headers) < workers {
		workers = len(headers)
	}

	// Create a task channel and spawn the verifiers
	var (
		inputs = make(chan int)
		done   = make(chan int, workers)
		errs   = make([]error, len(headers))
	)
	for i := 0; i < workers; i++ {
		go func() {
			for index := range inputs {
				var err error

				if chain.GetHeader(headers[index].Hash(), headers[index].Number.Uint64()) != nil {
					err = nil // known block
				} else {
					var parent *types.Header
					if index == 0 {
						parent = chain.GetHeader(headers[0].ParentHash, headers[0].Number.Uint64()-1)
					} else if headers[index-1].Hash() == headers[index].ParentHash {
						parent = headers[index-1]
					}
					if parent == nil {
						err = consensus.ErrUnknownAncestor
					} else {
						err = ar.verifyHeader(chain, headers[index], parent, false, seals[index])
					}
				}

				errs[index] = err
				done <- index
			}
		}()
	}

	var (
		abort     = make(chan struct{})
		errorsOut = make(chan error, len(headers))
	)
	go func() {
		defer close(inputs)
		var (
			in, out = 0, 0
			checked = make([]bool, len(headers))
			inputs  = inputs
		)
		for {
			select {
			case inputs <- in:
				if in++; in == len(headers) {
					// Reached end of headers. Stop sending to workers.
					inputs = nil
				}
			case index := <-done:
				for checked[index] = true; checked[out]; out++ {
					errorsOut <- errs[out]
					if out == len(headers)-1 {
						return
					}
				}
			case <-abort:
				return
			}
		}
	}()
	return abort, errorsOut
}

// VerifyUncles verifies that the given block's certification
func (ar *Algorand) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	// Verify no uncles included in this block
	const maxUncles = 0
	if len(block.Uncles()) > maxUncles {
		log.Warn("Unexpected!!! block has uncles",
			"height", block.NumberU64(), "Hash", block.Hash(), "uncles", len(block.Uncles()))
		return fmt.Errorf("block has uncles")
	}

	return nil
}

// VerifySeal checks whether the crypto seal on a header is valid according to
// the consensus rules of the given engine.
func (ar *Algorand) VerifySeal(chain consensus.ChainReader, header, parent *types.Header) error {
	// check proof
	height := header.Number.Uint64()
	certificate := header.Certificate
	addrSet := make(map[string]struct{})
	var stateDb *state.StateDB

	if len(certificate.TrieProof) > 0 { // for blockchain sync
		err := core2.VerifyProof(ar.config.Algorand, parent.Root, height, header.Certificate.Proposer(), certificate.CertVoteSet, certificate.TrieProof)
		if err != nil {
			return err
		}

		db := ethdb.NewMemDatabase()
		certificate.TrieProof.Store(db)
		database := state.NewDatabase(db)
		stateDb, err = state.New(parent.Root, database)
	} else { // for lightchain sync
		var err error
		stateDb, err = chain.StateAtHeader(parent)
		if err != nil {
			log.Warn("Unexpected!!! statedb error",
				"height", header.Number.Uint64(), "err", err)
			return err
		}
	}

	weightSum := uint64(0)

	for _, certVote := range certificate.CertVoteSet {
		vote := core.NewVoteDataFromCertVoteStorage(certVote, height, certificate.Round, certificate.Value)

		mv := core.GetMinerVerifier(ar.config.Algorand, stateDb, vote.Address, vote.Height)

		err := core.VerifySignatureAndCredential(mv, vote.SignBytes(), vote.ESignValue, &vote.Credential, stateDb, parent.Seed(), parent.TotalBalanceOfMiners)
		if err != nil {
			return err
		}

		weightSum += vote.Weight

		addrSet[certVote.Credential.Address.Str()] = struct{}{}
	}

	if numAddrs, numVotes := len(addrSet), len(certificate.CertVoteSet); numAddrs != numVotes {
		return fmt.Errorf("duplicate Address(%d) in certificate", numVotes-numAddrs)
	}

	threshold, _ := core.GetCommitteeNumber(height, types.RoundStep3Certifying)
	if weightSum < threshold {
		return fmt.Errorf("weightSum(%d) not reach threshold(%d)",
			weightSum, threshold)
	}

	proposerVerifier := core.GetMinerVerifier(ar.config.Algorand, stateDb, header.Certificate.Proposer(), height)

	// verify the credential of proposer
	hash := header.Hash()
	proposalLeader := core.NewProposalLeaderDataFromStorage(hash, height, &certificate.Proposal)
	err := core.VerifySignatureAndCredential(proposerVerifier, proposalLeader.SignBytes(), proposalLeader.ESignValue, &proposalLeader.Credential, stateDb, parent.Seed(), parent.TotalBalanceOfMiners)
	if err != nil {
		return fmt.Errorf("VerifyProposalLeader error, vote:%v, err:%s", certificate.Proposal, err.Error())
	}

	// Verify signature of header.ParentSeed
	err = proposerVerifier.VerifySeed(height, parent.Seed(), parent.Hash(), header.Seed(), header.Certificate.SeedProof())
	if err != nil {
		return fmt.Errorf("verify sigParentSeed error: %s", err)
	}

	return nil
}

// Prepare initializes the consensus fields of a block header according to the
// rules of a particular engine. The changes are executed inline.
func (ar *Algorand) Prepare(chain consensus.ChainReader, header *types.Header) error {
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}

	header.Difficulty = ar.CalcDifficulty(chain, header.Time, parent)

	return nil
}

// Finalize runs any post-transaction state modifications (e.g. block rewards)
// and assembles the final block.
// Note: The block header and state database might be updated to reflect any
// consensus rules that happen at finalization (e.g. block rewards).
func (ar *Algorand) Finalize(chain consensus.ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction,
	uncles []*types.Header, receipts []*types.Receipt) (*types.Block, error) {
	// Accumulate any block and uncle rewards and commit the final state root
	accumulateRewards(chain.Config(), state, header, uncles)

	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	header.TotalBalanceOfMiners = state.TotalBalanceOfMiners() // must invoke after IntermediateRoot

	// Header seems complete, assemble into a block and return
	return types.NewBlock(header, txs, uncles, receipts), nil
}

// Seal generates a new block for the given input block with the local miner's
// seal place on top.
func (ar *Algorand) Seal(chain consensus.ChainReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
	return errors.New("do NOT call Algorand.Seal to mine new block")
}

// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficulty
// that a new block should have.
func (ar *Algorand) CalcDifficulty(chain consensus.ChainReader, time uint64, parent *types.Header) *big.Int {
	return common.Big1
}

// APIs returns the RPC APIs this consensus engine provides.
func (ar *Algorand) APIs(chain consensus.ChainReader) []rpc.API {
	return nil
}

// Close terminates any background threads maintained by the consensus engine
func (ar *Algorand) Close() error {
	return nil
}

func (ar *Algorand) SealHash(header *types.Header) common.Hash {
	return header.Hash()
}

var (
	eidosFirstYearBlockReward  = new(big.Int).Mul(big.NewInt(40), common.BigEther)
	eidosSecondYearBlockReward = new(big.Int).Mul(big.NewInt(20), common.BigEther)
	eidosSteadyBlockReward     = new(big.Int).Mul(big.NewInt(10), common.BigEther)
)

func EidosBlockReward(num *big.Int, config *params.ChainConfig) *big.Int {
	height := num.Uint64()
	if height <= config.BlockYear {
		return eidosFirstYearBlockReward
	} else if height <= 2*config.BlockYear {
		return eidosSecondYearBlockReward
	} else {
		return eidosSteadyBlockReward
	}
}

// AccumulateRewards credits the coinbase of the given block with the mining
// reward. The total reward consists of the static block reward and rewards for
// included uncles. The coinbase of each uncle block is also rewarded.
func accumulateRewards(config *params.ChainConfig, state *state.StateDB, header *types.Header, _ []*types.Header) {
	// Select the correct block reward based on chain progression
	blockReward := EidosBlockReward(header.Number, config)
	// Accumulate the rewards for the miner and any included uncles
	state.AddBalance(header.Coinbase, blockReward)
}
