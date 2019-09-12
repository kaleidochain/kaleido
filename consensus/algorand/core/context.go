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
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kaleidochain/kaleido/crypto/ed25519"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/kaleidochain/kaleido/core"
	"github.com/kaleidochain/kaleido/core/state"
	"github.com/kaleidochain/kaleido/core/vm"
	"github.com/kaleidochain/kaleido/event"

	"github.com/kaleidochain/kaleido/params"

	"github.com/kaleidochain/kaleido/core/types"

	"github.com/kaleidochain/kaleido/common"

	"github.com/ethereum/go-ethereum/log"
	"github.com/kaleidochain/kaleido/consensus"
)

const (
	// txChanSize is the size of channel listening to TxPreEvent.
	// The number is referenced from the size of tx pool.
	txChanSize = 4096
	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
	chainHeadChanSize = 10
	// chainSideChanSize is the size of channel listening to ChainSideEvent.
	chainSideChanSize = 10
	startMiningFlag   = 1
	stopMiningFlag    = 0
	startSnappingFlag = 1
	stopSnappingFlag  = 0
	startRunningFlag  = 1
	stopRunningFlag   = 0
)

var errStopped = errors.New("mining stopped")

// Backend wraps all methods required for mining.
type Backend interface {
	BlockChain() *core.BlockChain
	TxPool() *core.TxPool
	GossipInterval() time.Duration
}

type Broadcaster interface {
	Broadcast(code uint64, data interface{})

	Publish(chanId []byte, msgCode uint64, data interface{})
	Sub(chanId []byte, timeout time.Duration)
	UnSub(chanId []byte)
	Request(chanId []byte, value common.Hash)
}

const msgChanSize = 4096
const fromMe = "me"

type message struct {
	code uint64
	data interface{}
	from string
}

type getBlockReq struct {
	parentHash   common.Hash
	wantedHeight uint64
	coinbase     common.Address

	resp chan *snapshotBlockState
}

type snapshotBlockState struct {
	header  *types.Header
	statedb *state.StateDB

	txs      types.Transactions
	receipts types.Receipts
}

func (s *snapshotBlockState) Copy() *snapshotBlockState {
	to := &snapshotBlockState{}
	to.header = types.CopyHeader(s.header)
	to.statedb = s.statedb.Copy()
	if len(s.txs) > 0 {
		to.txs = make(types.Transactions, len(s.txs))
		copy(to.txs, s.txs)
	}
	if len(s.receipts) > 0 {
		to.receipts = make(types.Receipts, len(s.receipts))
		copy(to.receipts, s.receipts)
	}

	return to
}

type Snapshot struct {
	miner          common.Address
	chainHeadBlock *types.Block

	snapshotBlockState
}

func (s *Snapshot) setMiner(miner common.Address) {
	s.miner = miner
}

type blockState struct {
	err      error
	statedb  *state.StateDB
	receipts types.Receipts
	counter  core.MortgageContractCounter
}

type HeightState struct {
	startTime time.Time

	parent        *types.Block
	parentStatedb *state.StateDB

	parentRoundVoteSet       *RoundVoteSet
	parentCertVoteRound      uint32
	mutexParentRoundVoteSet  sync.RWMutex
	parentProposalBlockData  *ProposalBlockData
	mutexParentProposalBlock sync.RWMutex

	header *types.Header

	Height uint64 // readonly, do not need mutex
	Round  uint32 // modify by inner-thread，the other threads only read, do not need mutex
	Step   uint32 // only using in inner-thread, do not need mutex

	EmptyValue    common.Hash
	StartingValue common.Hash

	counter *HeightVoteSet

	ProposalLeader          map[uint32]*ProposalLeaderData // round => leader's proposal value message
	mutexLeaderValue        sync.RWMutex
	ReceivedProposalBlock   map[string]*ProposalBlockData // Value => all received proposal data
	mutexReceivedBlock      sync.RWMutex
	verifingBlockState      map[string]chan *blockState
	verifiedBlockState      map[string]*blockState
	mutexVerifiedBlockState sync.RWMutex

	WaitingBlockForValue common.Hash
	WaitingBlockForRound uint32

	statedb                 *state.StateDB
	receipts                []*types.Receipt
	mortgageContractCounter core.MortgageContractCounter
	newBlock                *types.Block
}

type Context struct {
	eth         Backend
	config      *params.ChainConfig
	gasFloor    uint64
	gasCeil     uint64
	mux         *event.TypeMux
	engine      consensus.Engine
	broadcaster Broadcaster

	broadcastMsgCh chan message
	msgChan        chan message

	canonicalBlock      *types.Block
	stateMachineStartCh chan struct{}
	timeoutTicker       TimeoutTicker
	quit                chan struct{}
	wg                  sync.WaitGroup
	running             int32
	mining              int32
	snapping            int32
	snappingQuit        chan struct{}

	extra        []byte
	dataDir      string
	mkm          *MinerKeyManager
	currentMiner common.Address
	nextMiner    common.Address

	state   State
	hsMutex sync.Mutex
	HeightState
	snapshotMutex sync.RWMutex
	snapshot      Snapshot

	snappingChainHead chan core.ChainHeadEvent
	getBlockReqCh     chan *getBlockReq

	journal *VoteJournal
	recover bool
}

func NewContext(eth Backend, broadcaster Broadcaster, config *params.ChainConfig, mux *event.TypeMux, engine consensus.Engine, algorandDataDir string, gasFloor, gasCeil uint64) *Context {
	context := &Context{
		eth:         eth,
		config:      config,
		gasFloor:    gasFloor,
		gasCeil:     gasCeil,
		mux:         mux,
		engine:      engine,
		broadcaster: broadcaster,

		broadcastMsgCh: make(chan message, msgChanSize),
		msgChan:        make(chan message, msgChanSize),

		dataDir:           algorandDataDir,
		mkm:               NewMinerKeyManager(config.Algorand, algorandDataDir),
		snappingChainHead: make(chan core.ChainHeadEvent),
		getBlockReqCh:     make(chan *getBlockReq),

		recover: true,
	}

	return context
}

func (ctx *Context) startStateMachine() {
	if ctx.state != nil {
		panic("Context.state should be nil on start")
	}

	ctx.state = &StateNewHeight{chainHeadBlock: ctx.canonicalBlock}
	ctx.state.SetContext(ctx)
	ctx.state.OnEnter()
}

func (ctx *Context) gotoState(newState State) {
	if ctx.state == nil { // state machine not started
		panic("StateMachine not started")
	}

	ctx.state.OnExit()
	ctx.state = newState
	ctx.state.SetContext(ctx)
	ctx.state.OnEnter()
}

func (ctx *Context) HRS() string {
	return fmt.Sprintf("%d/%d/%d", ctx.Height, ctx.Round, ctx.Step)
}

func (ctx *Context) HR() (uint64, uint32) {
	ctx.hsMutex.Lock()
	defer ctx.hsMutex.Unlock()

	return ctx.Height, ctx.Round
}

func (ctx *Context) IsMining() bool {
	return atomic.LoadInt32(&ctx.mining) == startMiningFlag
}

func (ctx *Context) startSnapping() {
	swapped := atomic.CompareAndSwapInt32(&ctx.snapping, stopSnappingFlag, startSnappingFlag)
	if !swapped {
		log.Error("Context snapshotLoop already snapping")
		return
	}

	ctx.snapshotMutex.Lock()
	defer ctx.snapshotMutex.Unlock()

	ctx.snappingQuit = make(chan struct{})
	ctx.snapshot.chainHeadBlock = ctx.canonicalBlock
	ctx.updateSnapshotBlockState(nil)
	go ctx.snapshotLoop()
}

func (ctx *Context) stopSnapping() {
	swapped := atomic.CompareAndSwapInt32(&ctx.snapping, startSnappingFlag, stopSnappingFlag)
	if !swapped {
		log.Error("Context snapshotLoop already stoped")
		return
	}
	close(ctx.snappingQuit)
}

func commitTransactions(quit chan struct{}, config *params.ChainConfig, mux *event.TypeMux, eth Backend, txs *types.TransactionsByPriceAndNonce,
	statedb *state.StateDB, header *types.Header, signer types.Signer) ([]*types.Transaction, []*types.Receipt, core.MortgageContractCounter) {
	validatedTxs := make([]*types.Transaction, 0)
	validatedReceipts := make([]*types.Receipt, 0)
	mortgageContractCounter := make(core.MortgageContractCounter)

	gp := new(core.GasPool).AddGas(header.GasLimit)

	var coalescedLogs []*types.Log
	txcount := 0

	for {
		select {
		case <-quit:
			log.Debug("Snapping stopped, break commitTransactions", "committed", txcount)
			return validatedTxs, validatedReceipts, mortgageContractCounter
		default: //do nothing
		}

		// If we don't have enough gas for any further transactions then we're done
		if gp.Gas() < params.TxGas {
			log.Trace("Not enough gas for further transactions", "gp", gp)
			break
		}
		// Retrieve the next transaction and abort if all done
		tx := txs.Peek()
		if tx == nil {
			break
		}
		// Error may be ignored here. The error has already been checked
		// during transaction acceptance is the transaction pool.
		//
		// We use the eip155 signer regardless of the current hf.
		from, _ := types.Sender(signer, tx)
		// Check whether the tx is replay protected. If we're not in the EIP155 hf
		// phase, start ignoring the sender until we do.
		if tx.Protected() && !config.IsEIP155(header.Number) {
			log.Trace("Ignoring reply protected transaction", "hash", tx.Hash(), "eip155", config.EIP155Block)

			txs.Pop()
			continue
		}
		// Start executing the transaction
		statedb.Prepare(tx.Hash(), common.Hash{}, txcount)

		core.ProcessTx(mortgageContractCounter, statedb, tx)

		err, receipt := commitTransaction(config, tx, eth.BlockChain(), statedb, header, header.Coinbase, gp)
		switch err {
		case core.ErrGasLimitReached:
			// Pop the current out-of-gas transaction without shifting in the next from the account
			log.Trace("Gas limit exceeded for current block", "sender", from)
			txs.Pop()

		case core.ErrNonceTooLow:
			// New head notification data race between the transaction pool and miner, shift
			log.Trace("Skipping transaction with low nonce", "sender", from, "nonce", tx.Nonce())
			txs.Shift()

		case core.ErrNonceTooHigh:
			// Reorg notification data race between the transaction pool and miner, skip account =
			log.Trace("Skipping account with high nonce", "sender", from, "nonce", tx.Nonce())
			txs.Pop()

		case nil:
			// Everything ok, collect the logs and shift in the next transaction from the same account
			validatedTxs = append(validatedTxs, tx)
			validatedReceipts = append(validatedReceipts, receipt)

			coalescedLogs = append(coalescedLogs, receipt.Logs...)
			txcount++
			txs.Shift()

		default:
			// Strange error, discard the transaction and get the next in line (note, the
			// nonce-too-high clause will prevent us from executing in vain).
			log.Debug("Transaction failed, account skipped", "hash", tx.Hash(), "err", err)
			txs.Shift()
		}
	}

	if len(coalescedLogs) > 0 {
		// make a copy, the state caches the logs and these logs get "upgraded" from pending to mined
		// logs by filling in the block hash when the block was mined by the local miner. This can
		// cause a race condition if a log was "upgraded" before the PendingLogsEvent is processed.
		cpy := make([]*types.Log, len(coalescedLogs))
		for i, l := range coalescedLogs {
			cpy[i] = new(types.Log)
			*cpy[i] = *l
		}
		go mux.Post(core.PendingLogsEvent{Logs: cpy})
	}

	return validatedTxs, validatedReceipts, mortgageContractCounter
}

func commitTransaction(config *params.ChainConfig, tx *types.Transaction,
	bc *core.BlockChain, statedb *state.StateDB, header *types.Header, coinbase common.Address, gp *core.GasPool) (error, *types.Receipt) {
	snap := statedb.Snapshot()

	receipt, _, err := core.ApplyTransaction(config, bc, &coinbase, gp, statedb, header, tx, &header.GasUsed, vm.Config{})
	if err != nil {
		statedb.RevertToSnapshot(snap)
		return err, nil
	}

	return nil, receipt
}

func (ctx *Context) updateSnapshotBlockState(author *common.Address) error {
	blockchain := ctx.eth.BlockChain()
	parent := ctx.snapshot.chainHeadBlock

	// make header for new block
	start := time.Now()
	timestamp := start.Unix()
	if parent.Time() >= uint64(timestamp) {
		timestamp = int64(parent.Time() + 1)
	}
	// this will ensure we're not going off too far in the future
	if now := time.Now().Unix(); timestamp > now+1 {
		wait := time.Duration(timestamp-now) * time.Second
		log.Info("Mining too far in the future", "wait", common.PrettyDuration(wait))
		time.Sleep(wait)
	}

	// Could potentially happen if starting to mine in an odd statedb.
	statedb, err := blockchain.StateAt(parent.Root())
	if err != nil {
		log.Error("Failed to create block", "err", err)
		return err
	}

	num := parent.Number()
	height := num.Add(num, common.Big1)
	header := &types.Header{
		ParentHash:  parent.Hash(),
		Number:      height,
		GasLimit:    core.CalcGasLimit(parent, ctx.gasFloor, ctx.gasCeil),
		Time:        uint64(timestamp),
		Certificate: new(types.Certificate), // must set it to empty value
	}
	if author == nil {
		header.Coinbase = ctx.snapshot.miner
	} else {
		header.Coinbase = *author
	}

	if err := ctx.engine.Prepare(blockchain, header); err != nil {
		log.Error("Failed to prepare header for snapping", "err", err)
		return err
	}

	statedb.EnableMinerTracker(ctx.config.Algorand, header.Number.Uint64(), parent.TotalBalanceOfMiners())
	signer := types.NewEIP155Signer(ctx.config.ChainID)

	pendingTxs, err := ctx.eth.TxPool().Pending()
	if err != nil {
		log.Error("Failed to fetch pending transactions", "err", err)
		return err
	}

	txs := types.NewTransactionsByPriceAndNonce(signer, pendingTxs)
	validatedTxs, validatedReceipts, _ := commitTransactions(ctx.snappingQuit, ctx.config, ctx.mux, ctx.eth, txs, statedb, header, signer)

	log.Trace("updateSnapshotBlockState",
		"number", header.Number.Uint64(),
		"HRS", ctx.HRS())

	ctx.snapshot.header = header
	ctx.snapshot.statedb = statedb
	ctx.snapshot.txs = validatedTxs
	ctx.snapshot.receipts = validatedReceipts

	return nil
}

func (ctx *Context) processChainHeadEventForSnapshotLoop(e *core.ChainHeadEvent) {
	ctx.snapshotMutex.Lock()
	defer ctx.snapshotMutex.Unlock()

	ctx.snapshot.chainHeadBlock = e.Block
	ctx.updateSnapshotBlockState(nil)
}

func (ctx *Context) processGetBlockReqForSnapshotLoop(req *getBlockReq) {
	ctx.snapshotMutex.Lock()
	defer ctx.snapshotMutex.Unlock()

	if req.parentHash != ctx.snapshot.header.ParentHash {
		log.Error("State NewBlock parent unequal",
			"wantednumber", req.wantedHeight,
			"pendingnumber", ctx.snapshot.header.Number.Uint64(),
			"HRS", ctx.HRS())

		panic("State NewBlock parent unequal")
	} else if req.coinbase != ctx.snapshot.header.Coinbase {
		log.Trace("State NewBlock coinbase unequal",
			"wantednumber", req.wantedHeight,
			"pendingnumber", ctx.snapshot.header.Number.Uint64(),
			"wantedcoinbaes", req.coinbase,
			"pendingcoinbase", ctx.snapshot.header.Coinbase,
			"HRS", ctx.HRS())

		ctx.updateSnapshotBlockState(&req.coinbase)
	}

	if req.parentHash != ctx.snapshot.header.ParentHash || req.coinbase != ctx.snapshot.header.Coinbase {
		log.Error("Get Block from snapping error",
			"wantednumber", req.wantedHeight,
			"pendingnumber", ctx.snapshot.header.Number.Uint64(),
			"HRS", ctx.HRS())

		panic("Get Block from snapping error")
	}

	cache := ctx.snapshot.snapshotBlockState.Copy()

	log.Trace("Get Block State from snapping ok",
		"wantednumber", req.wantedHeight,
		"number", cache.header.Number.Uint64())

	select {
	case req.resp <- cache:
	case <-ctx.snappingQuit:
	}

}

func (ctx *Context) processTxPreventFromSnapshotLoop(txs types.Transactions) {
	ctx.snapshotMutex.Lock()
	defer ctx.snapshotMutex.Unlock()

	signer := types.NewEIP155Signer(ctx.config.ChainID)
	txsMap := make(map[common.Address]types.Transactions)
	for _, tx := range txs {
		acc, _ := types.Sender(signer, tx)
		txsMap[acc] = append(txsMap[acc], tx)
	}

	txset := types.NewTransactionsByPriceAndNonce(signer, txsMap)
	validatedTxs, validatedReceipts, _ := commitTransactions(ctx.snappingQuit, ctx.config, ctx.mux, ctx.eth, txset,
		ctx.snapshot.statedb, ctx.snapshot.header, signer)
	if len(validatedTxs) > 0 {
		ctx.snapshot.txs = append(ctx.snapshot.txs, validatedTxs...)
		ctx.snapshot.receipts = append(ctx.snapshot.receipts, validatedReceipts...)
	}

	log.Trace("Updated pending snapshot block", "txs", txs.Len(),
		"number", ctx.snapshot.header.Number, "txCount", len(ctx.snapshot.txs),
		"gasLimit", ctx.snapshot.header.GasLimit, "gasUsed", ctx.snapshot.header.GasUsed)
}

func (ctx *Context) snapshotLoop() {
	txCh := make(chan core.NewTxsEvent, txChanSize)
	chainSideCh := make(chan core.ChainSideEvent, chainSideChanSize)

	// Subscribe TxPreEvent for tx pool
	txSub := ctx.eth.TxPool().SubscribeNewTxsEvent(txCh)
	// Subscribe events for blockchain
	chainSideSub := ctx.eth.BlockChain().SubscribeChainSideEvent(chainSideCh)

	defer txSub.Unsubscribe()
	defer chainSideSub.Unsubscribe()

	for {
		// A real event arrived, process interesting content
		select {
		case <-ctx.snappingQuit:
			log.Trace("snapshotLoop exit")
			return

		case e := <-ctx.snappingChainHead:
			ctx.processChainHeadEventForSnapshotLoop(&e)

		case req := <-ctx.getBlockReqCh:
			ctx.processGetBlockReqForSnapshotLoop(req)

		// Handle ChainSideEvent
		case <-chainSideCh:

		// Handle TxPreEvent
		case e := <-txCh:
			// Apply transaction to the snapping state
			ctx.processTxPreventFromSnapshotLoop(e.Txs)

		// System stopped
		case err := <-txSub.Err():
			log.Debug("snapshotLoop txSub error", "err", err)
			return
		case err := <-chainSideSub.Err():
			log.Debug("snapshotLoop chainHeadSub error", "err", err)
			return
		}
	}

}

func (ctx *Context) startRunning() {
	swapped := atomic.CompareAndSwapInt32(&ctx.running, stopRunningFlag, startRunningFlag)
	if !swapped {
		log.Error("Context already started")
		return
	}

	ctx.hsMutex.Lock()
	defer ctx.hsMutex.Unlock()

	ctx.stateMachineStartCh = make(chan struct{})

	ctx.timeoutTicker = NewTimeoutTicker()
	_ = ctx.timeoutTicker.Start()

	ctx.quit = make(chan struct{})
	go ctx.broadcastLoop()
	go ctx.receiveRoutine()
}

func (ctx *Context) stopRunning() {
	swapped := atomic.CompareAndSwapInt32(&ctx.running, startRunningFlag, stopRunningFlag)
	if !swapped {
		log.Error("Context already stopped")
		return
	}

	ctx.hsMutex.Lock()
	defer ctx.hsMutex.Unlock()

	close(ctx.quit)
	ctx.wg.Wait()

	ctx.timeoutTicker.Stop()
	ctx.timeoutTicker = nil

	// 中止新peer对我的gossip消息
	ctx.Height = 0
	ctx.Round = types.BadRound

	ctx.state = nil

	if ctx.journal != nil {
		_ = ctx.journal.CloseFile()
	}
}

func (ctx *Context) setCanonicalBlock() {
	ctx.canonicalBlock = ctx.eth.BlockChain().CurrentBlock()
}

func (ctx *Context) Start() {
	ctx.setCanonicalBlock()
	ctx.startSnapping()
	ctx.startRunning()
}

func (ctx *Context) Stop() {
	ctx.stopRunning()
	ctx.stopSnapping()
}

func (ctx *Context) StartMining() {
	swapped := atomic.CompareAndSwapInt32(&ctx.mining, stopMiningFlag, startMiningFlag)
	if !swapped {
		log.Error("Mining already started")
		return
	}
}

func (ctx *Context) StopMining() {
	swapped := atomic.CompareAndSwapInt32(&ctx.mining, startMiningFlag, stopMiningFlag)
	if !swapped {
		log.Error("Mining already stopped")
		return
	}
}

func (ctx *Context) broadcastLoop() {
	ctx.wg.Add(1)
	defer ctx.wg.Done()

	for {
		select {
		case <-ctx.quit:
			log.Trace("StateMachine context broadcast loop exit", "HRS", ctx.HRS())
			return

		case msg := <-ctx.broadcastMsgCh:
			ctx.broadcaster.Broadcast(msg.code, msg.data)
		}
	}
}

func (ctx *Context) OnReceive(code uint64, data interface{}, from string) {
	sendToMessageChan(ctx.msgChan, message{code, data, from})
}

func (ctx *Context) prepareRecover() {
	parentHeight := ctx.canonicalBlock.NumberU64()
	dir := fmt.Sprintf("%s/msg", ctx.dataDir)
	if !common.FileExist(dir) {
		err := os.Mkdir(dir, os.ModePerm)
		if err != nil {
			log.Error("Context recover mkdir error", "dir", dir, "err", err)
			return
		}
		ctx.recover = false
	}
	recoverFile := fmt.Sprintf("%s/%d", dir, parentHeight+1)
	if !common.FileExist(recoverFile) {
		log.Info("Context recover can not recover, recover file is not exist", "file", recoverFile)
		ctx.recover = false
	}
	ctx.journal = NewVoteJournal(recoverFile)
}

func (ctx *Context) loadRecoverEntry() {
	err := ctx.journal.Read(func(ts, code uint64, data interface{}, from string) error {
		from = "recover-" + from

		switch code {
		case ProposalLeaderMsg:
			nb := bytes.NewReader(data.([]byte))
			var leader ProposalLeaderData
			stream := rlp.NewStream(nb, 0)
			if err := stream.Decode(&leader); err != nil {
				log.Error("recover decode failed", "code", code, "from", from)
				return err
			}

			ctx.handleMsg(code, &leader, from)
		case ProposalBlockMsg:
			nb := bytes.NewReader(data.([]byte))
			var proposalBlock ProposalBlockData
			stream := rlp.NewStream(nb, 0)
			if err := stream.Decode(&proposalBlock); err != nil {
				log.Error("recover decode failed", "code", code, "from", from)
				return err
			}

			ctx.handleMsg(code, &proposalBlock, from)
		case VoteMsg:
			nb := bytes.NewReader(data.([]byte))
			var vote VoteData
			stream := rlp.NewStream(nb, 0)
			if err := stream.Decode(&vote); err != nil {
				log.Error("recover decode failed", "code", code, "from", from)
				return err
			}

			ctx.handleMsg(code, &vote, from)
		case TimeoutMsg:
			nb := bytes.NewReader(data.([]byte))
			var ti TimeoutInfo
			stream := rlp.NewStream(nb, 0)
			if err := stream.Decode(&ti); err != nil {
				log.Error("recover decode failed", "code", code, "from", from)
				return err
			}

			ctx.handleTimeout(ti)
		}

		return nil
	})

	if err != nil {
		log.Error("Context recover error", "err", err)
	}
}

func PrintVoteJournalEntry(ts, code uint64, data interface{}, from string) error {
	var entry string

	switch code {
	case ProposalLeaderMsg:
		nb := bytes.NewReader(data.([]byte))
		var leader ProposalLeaderData
		stream := rlp.NewStream(nb, 0)
		if err := stream.Decode(&leader); err != nil {
			log.Error("recover decode failed", "code", code, "from", from)
			return err
		}

		entry = leader.String()
	case ProposalBlockMsg:
		nb := bytes.NewReader(data.([]byte))
		var proposalBlock ProposalBlockData
		stream := rlp.NewStream(nb, 0)
		if err := stream.Decode(&proposalBlock); err != nil {
			log.Error("recover decode failed", "code", code, "from", from)
			return err
		}

		entry = proposalBlock.String()
	case VoteMsg:
		nb := bytes.NewReader(data.([]byte))
		var vote VoteData
		stream := rlp.NewStream(nb, 0)
		if err := stream.Decode(&vote); err != nil {
			log.Error("recover decode failed", "code", code, "from", from)
			return err
		}

		entry = vote.String()
	case TimeoutMsg:
		nb := bytes.NewReader(data.([]byte))
		var ti TimeoutInfo
		stream := rlp.NewStream(nb, 0)
		if err := stream.Decode(&ti); err != nil {
			log.Error("recover decode failed", "code", code, "from", from)
			return err
		}

		entry = ti.String()
	}

	fmt.Printf("%s %s %20s %s\n", from, time.Unix(0, int64(ts)).Format("2006-01-02 15:04:05.000"), CodeToString[code], entry)

	return nil
}

func (ctx *Context) readyRecover() {
	log.Trace("Context recover", "recover", ctx.recover)
	if ctx.recover {
		ctx.prepareRecover()
	}

	ctx.startStateMachine()

	if ctx.recover {
		log.Trace("Context recover begin", "recover", ctx.recover, "HRS", ctx.HRS())

		ctx.loadRecoverEntry()
	}
	log.Trace("Context recover end", "recover", ctx.recover, "HRS", ctx.HRS())

	ctx.recover = false
}

func (ctx *Context) resetAndOpenRecoverFile(height uint64) {
	recoverFile := fmt.Sprintf("%s/msg/%d", ctx.dataDir, height)

	if ctx.journal != nil {
		_ = ctx.journal.CloseFile()
	}
	ctx.journal = NewVoteJournal(recoverFile)
	_ = ctx.journal.OpenFile()
}

func (ctx *Context) receiveRoutine() {
	ctx.wg.Add(1)
	defer ctx.wg.Done()

	chainHeadCh := make(chan core.ChainHeadEvent, chainHeadChanSize)
	chainHeadSub := ctx.eth.BlockChain().SubscribeChainHeadEvent(chainHeadCh)
	defer chainHeadSub.Unsubscribe()

	ctx.readyRecover()

	for {
		select {
		case <-ctx.quit:
			log.Trace("StateMachine context receive routine exit", "HRS", ctx.HRS())
			return

		case <-chainHeadCh:
			currentBlock := ctx.eth.BlockChain().CurrentBlock()
			if ctx.parent.Hash() != currentBlock.Hash() {
				log.Debug("chain head is changed, go to new height",
					"hash", currentBlock.Hash(),
					"number", currentBlock.NumberU64(),
					"context.parent.hash", ctx.parent.Hash(),
					"HRS", ctx.HRS())

				select {
				case ctx.snappingChainHead <- core.ChainHeadEvent{Block: currentBlock}:
				case <-ctx.snappingQuit:
					log.Trace("StateMachine context receive routine snapping exit", "HRS", ctx.HRS())
					return
				case <-ctx.quit:
					log.Trace("StateMachine context receive routine snapping exit", "HRS", ctx.HRS())
					return
				}

				parentHeight := currentBlock.NumberU64()
				ctx.resetAndOpenRecoverFile(parentHeight + 1)

				// TODO: Minerkey should be deleted by notification from here.

				vote := ctx.makeStampingVote(currentBlock)
				ctx.eth.BlockChain().PostChainEvents([]interface{}{core.ChainStampingEvent{Vote: vote}}, nil)

				ctx.gotoState(&StateNewHeight{chainHeadBlock: currentBlock})
			}

		case <-ctx.stateMachineStartCh:
			ctx.startStateMachine()

		case msg := <-ctx.msgChan:
			if currentHeight := ctx.eth.BlockChain().CurrentBlock().NumberU64(); currentHeight < ctx.Height {
				if err := ctx.journal.Write(msg.code, msg.data, msg.from); err != nil {
					log.Error("recover: write msg journal err", "HRS", ctx.HRS(), "err", err)
				}
				ctx.handleMsg(msg.code, msg.data, msg.from)
			}

		case ti := <-ctx.timeoutTicker.Chan():
			if currentHeight := ctx.eth.BlockChain().CurrentBlock().NumberU64(); currentHeight < ctx.Height {
				if err := ctx.journal.Write(TimeoutMsg, ti, "ti"); err != nil {
					log.Error("recover: write timeout journal err", "HRS", ctx.HRS(), "err", err)
				}
				ctx.handleTimeout(ti)
			}

		case err := <-chainHeadSub.Err():
			log.Error("receiveRoutine chainHeadSub error", "err", err)
			return
		}
	}
}

func (ctx *Context) handleMsg(code uint64, data interface{}, from string) {
	log.Trace("handleMessage", "code", CodeToString[code],
		"HRS", ctx.HRS(), "startingValue", ctx.StartingValue, "from", from)

	if ctx.state == nil {
		log.Error("handleMsg StateMachine not started")
		return
	}

	switch code {
	case ProposalLeaderMsg:
		proposalValue, ok := data.(*ProposalLeaderData)
		if !ok {
			log.Error("Bad ProposalLeaderMsg", "data", data)
			return
		}
		if !proposalValue.Valid(ctx.Height) {
			log.Debug("invalid msg", "msg", proposalValue)
			return
		}
		err := ctx.handleProposalLeader(proposalValue, from)
		if err != nil {
			log.Debug("handle ProposalValue failed", "proposalValue", proposalValue, "err", err.Error())
		}
	case ProposalBlockMsg:
		proposalBlock, ok := data.(*ProposalBlockData)
		if !ok {
			log.Error("Bad ProposalBlockMsg", "data", data)
			return
		}
		if !proposalBlock.Valid(ctx.Height) {
			log.Debug("invalid msg", "msg", proposalBlock)
			return
		}
		err := ctx.handleProposalBlock(proposalBlock, from)
		if err != nil {
			log.Debug("handle ProposalBlock failed", "proposalBlock", proposalBlock, "err", err.Error())
		}
	case VoteMsg:
		vote, ok := data.(*VoteData)
		if !ok {
			log.Warn("Bad VoteMsg", "data", data)
			return
		}
		if !vote.Valid(ctx.Height, ctx.EmptyValue) {
			log.Debug("invalid msg", "msg", vote)
			return
		}
		err := ctx.handleVote(vote, from)
		if err != nil {
			log.Debug("handle vote failed", "vote", vote, "err", err)
		}
	}
}

func (ctx *Context) handleProposalLeader(data *ProposalLeaderData, from string) error {
	log.Trace("handleProposalLeader", "data", data, "HRS", ctx.HRS(), "from", from)

	if leader := ctx.getProposalLeader(data.Round); leader != nil && leader.LessThan(data) {
		return fmt.Errorf("ProposalLeader is bigger")
	}

	mv := GetMinerVerifier(ctx.config.Algorand, ctx.parentStatedb, data.Address, data.Height)

	err := VerifySignatureAndCredential(mv, data.SignBytes(), data.ESignValue, &data.Credential, ctx.parentStatedb, ctx.parent.Seed(), ctx.parent.TotalBalanceOfMiners())
	if err != nil {
		return err
	}

	ctx.broadcastMsg(HasProposalLeaderMsg, data.ToHasProposalData())

	ctx.state.OnProposalLeaderReceived(data)
	return nil
}

func (ctx *Context) handleProposalBlock(block *ProposalBlockData, from string) error {
	log.Trace("handleProposalBlock", "msg", block, "HRS", ctx.HRS(), "from", from)

	if has := ctx.getProposalBlock(block.Block.Hash()); has != nil {
		return fmt.Errorf("ProposalBlock %s exists", block.Block.Hash().String())
	}

	mv := GetMinerVerifier(ctx.config.Algorand, ctx.parentStatedb, block.Address, block.Height)

	err := VerifySignatureAndCredential(mv, block.SignBytes(), block.ESignValue, &block.Credential, ctx.parentStatedb, ctx.parent.Seed(), ctx.parent.TotalBalanceOfMiners())
	if err != nil {
		return err
	}

	err = mv.VerifySeed(block.Height, ctx.parent.Seed(), ctx.parent.Hash(), block.Block.Seed(), block.Block.SeedProof())
	if err != nil {
		log.Error("verify seed failed", "err", err, "msg", block)
		return err
	}

	if block.Block.Coinbase() != mv.coinbase {
		return fmt.Errorf("verify coinbase failed, header(%d) coinbase:%s, proposalBlock:%s, expected coinbase:%s",
			block.Block.NumberU64(), block.Block.Coinbase(), block, mv.coinbase)
	}

	ctx.broadcastMsg(HasProposalBlockMsg, block.ToHasProposalData())

	ctx.state.OnProposalLeaderReceived(block.NewProposalLeaderData())

	// Verify block.Block header
	err = ctx.engine.VerifyHeader(ctx.eth.BlockChain(), block.Block.Header(), false)
	if err != nil {
		log.Error("handleProposalBlock verify header fail", "ProposalBlock", block, "err", err)
		return err
	}

	if from == fromMe {
		cache := &blockState{statedb: ctx.statedb, receipts: ctx.receipts, counter: ctx.mortgageContractCounter}
		ctx.saveVerifiedBlockState(block.Block.Hash(), cache)
	}

	ctx.SaveProposalBlock(block)

	if ctx.StartingValue == block.Block.Hash() || ctx.counter.IsPotentialValue(block.Round, block.Block.Hash()) {
		// this proposal will be the final block in high probability
		ctx.verifyBlockAsync(block.Block)
	}

	ctx.state.OnProposalDataComplete(block)

	return nil
}

func (ctx *Context) handleVote(vote *VoteData, from string) error {
	log.Trace("handleVote", "vote", vote, "HRS", ctx.HRS(), "from", from)

	if ctx.counter.HasVote(vote.Round, vote.Step, vote.Address) {
		return errors.New("duplicate vote")
	}

	mv := GetMinerVerifier(ctx.config.Algorand, ctx.parentStatedb, vote.Address, vote.Height)

	err := VerifySignatureAndCredential(mv, vote.SignBytes(), vote.ESignValue, &vote.Credential, ctx.parentStatedb, ctx.parent.Seed(), ctx.parent.TotalBalanceOfMiners())
	if err != nil {
		return err
	}

	ctx.broadcastMsg(HasVoteMsg, ToHasVote(vote))

	threshold, _ := GetCommitteeNumber(vote.Height, vote.Step)
	added, newPotential, enough, err := ctx.counter.AddVoteAndCount(vote, threshold)
	if err != nil {
		return err
	}

	if newPotential {
		ctx.state.OnPotentialVoteEnough(vote.Value, vote.Round, vote.Step)
	}

	if added && enough { // is firstly enough for this vote
		switch t := types.VoteTypeOfStep(vote.Step); t {
		case types.VoteTypeSoft:
			ctx.state.OnSoftVoteEnough(vote.Value, vote.Round)
		case types.VoteTypeCert:
			ctx.state.OnCertVoteEnough(vote.Value, vote.Round)
		case types.VoteTypeNext:
			ctx.state.OnNextVoteEnough(vote.Value, vote.Round, vote.Step)
		}
	}

	return nil
}

func sendToMessageChan(ch chan<- message, msg message) {
	select {
	case ch <- msg:
		return
	default:
		log.Error("message chan full", "size", len(ch))
		return
	}
}

func (ctx *Context) broadcastMsg(code uint64, data interface{}) {
	if ctx.recover {
		return
	}

	sendToMessageChan(ctx.broadcastMsgCh, message{code, data, fromMe})
}

const (
	increaseFromRound = 3
	increaseFromStep  = 8
)

func (ctx *Context) startTimer() {
	factor := uint64(1)
	if ctx.Round >= increaseFromRound {
		r := math.Pow(2, float64(ctx.Round-increaseFromRound+1))
		if r > 30 {
			r = 30
		}

		factor *= uint64(r)
	}
	if ctx.Step >= increaseFromStep {
		s := uint64((ctx.Step-increaseFromStep)/2 + 1)

		if s > 10 {
			s = 10
		}

		factor *= uint64(s)
	}

	ti := TimeoutInfo{
		uint64(ctx.config.Algorand.TimeoutTwoLambdaDuration()) * factor,
		ctx.Height,
		ctx.Round,
		ctx.Step,
	}

	switch ctx.state.(type) {
	case *StateProposal:
		chanId := makeChanId(ctx.Height, ctx.Round)
		ctx.broadcaster.Sub(chanId, time.Duration(ti.Duration*2))
	}

	ctx.timeoutTicker.ScheduleTimeout(ti)
}

func (ctx *Context) clearTimer() {
	switch ctx.state.(type) {
	case *StateCertifying:
		chanId := makeChanId(ctx.Height, ctx.Round)
		ctx.broadcaster.UnSub(chanId)
	}
}

func makeChanId(height uint64, round uint32) []byte {
	chanId := make([]byte, 8+4)
	binary.BigEndian.PutUint64(chanId[0:8], height)
	binary.BigEndian.PutUint32(chanId[8:8+4], round)
	return chanId
}

func (ctx *Context) handleTimeout(ti TimeoutInfo) {
	if ctx.state == nil {
		return
	}

	if ti.Height != ctx.Height || ti.Round != ctx.Round || ti.Step != ctx.Step {
		log.Debug("Ignoring timeout because we're ahead", "HRS", ctx.HRS(), "timeoutInfo", ti)
		return
	}

	ctx.state.OnTimeout()
}

func (ctx *Context) isNextVoteEnoughForEmpty(round uint32) bool {
	return ctx.counter.IsNextVoteEnoughForEmptyAtAnyStep(round, ctx.EmptyValue)
}

func (ctx *Context) resetReceivedProposalBlock() {
	ctx.mutexReceivedBlock.Lock()
	defer ctx.mutexReceivedBlock.Unlock()

	ctx.ReceivedProposalBlock = make(map[string]*ProposalBlockData)
}

// GetProposalLeader returns a READONLY ProposalLeaderData of the leader
// return nil if not exists
func (ctx *Context) GetProposalLeader(height uint64, round uint32) *ProposalLeaderData {

	ctx.mutexLeaderValue.RLock()
	defer ctx.mutexLeaderValue.RUnlock()

	if ctx.Height != height || ctx.ProposalLeader == nil || round == types.BadRound {
		return nil
	}

	return ctx.ProposalLeader[round]
}

func (ctx *Context) getProposalLeader(round uint32) *ProposalLeaderData {
	ctx.mutexLeaderValue.RLock()
	defer ctx.mutexLeaderValue.RUnlock()

	if ctx.ProposalLeader == nil || round == types.BadRound {
		return nil
	}

	return ctx.ProposalLeader[round]
}

// UpdateProposalLeader stores a copy of ProposalLeaderData of the leader
func (ctx *Context) UpdateProposalLeader(value *ProposalLeaderData) {
	ctx.mutexLeaderValue.Lock()
	defer ctx.mutexLeaderValue.Unlock()
	ctx.ProposalLeader[value.Round] = value
}

// GetProposalBlock returns a READONLY proposalBlock of the value
// return nil if not exists
func (ctx *Context) GetProposalBlock(height uint64, value common.Hash) *ProposalBlockData {
	ctx.mutexReceivedBlock.RLock()
	defer ctx.mutexReceivedBlock.RUnlock()

	if ctx.Height != height {
		return nil
	}

	return ctx.ReceivedProposalBlock[value.Str()]
}

func (ctx *Context) getProposalBlock(value common.Hash) *ProposalBlockData {
	ctx.mutexReceivedBlock.RLock()
	defer ctx.mutexReceivedBlock.RUnlock()

	return ctx.ReceivedProposalBlock[value.Str()]
}

// SaveProposalBlock stores a copy of proposalBlock of the value
func (ctx *Context) SaveProposalBlock(data *ProposalBlockData) {
	ctx.mutexReceivedBlock.Lock()
	defer ctx.mutexReceivedBlock.Unlock()

	ctx.ReceivedProposalBlock[data.Block.Hash().Str()] = data
}

func (ctx *Context) dropProposalBlock(block *types.Block) {
	ctx.mutexReceivedBlock.Lock()
	defer ctx.mutexReceivedBlock.Unlock()

	delete(ctx.ReceivedProposalBlock, block.Hash().Str())
}

func (ctx *Context) resetProposalLeader() {
	ctx.mutexLeaderValue.Lock()
	defer ctx.mutexLeaderValue.Unlock()

	ctx.ProposalLeader = make(map[uint32]*ProposalLeaderData)
}

func (ctx *Context) resetParentRoundVoteSet() {
	ctx.mutexParentRoundVoteSet.Lock()
	defer ctx.mutexParentRoundVoteSet.Unlock()

	if ctx.parent.NumberU64() == 0 {
		return
	}

	certificate := ctx.parent.Certificate()
	votes := make([]*VoteData, len(certificate.CertVoteSet))
	for i, certVote := range certificate.CertVoteSet {
		if certVote == nil {
			continue
		}

		votes[i] = NewVoteDataFromCertVoteStorage(certVote, ctx.parent.NumberU64(), certificate.Round, certificate.Value)
	}

	threshold, _ := GetCommitteeNumber(ctx.parent.NumberU64(), types.RoundStep3Certifying)
	ctx.parentRoundVoteSet = NewRoundVoteSetFromCertificates(votes, threshold)
	ctx.parentCertVoteRound = certificate.Round
}

func (ctx *Context) GetParentRoundVoteSet(height uint64) (*RoundVoteSet, uint32) {
	ctx.mutexParentRoundVoteSet.RLock()
	defer ctx.mutexParentRoundVoteSet.RUnlock()

	if ctx.Height-1 != height {
		return nil, types.BadRound
	}

	return ctx.parentRoundVoteSet, ctx.parentCertVoteRound
}

func (ctx *Context) resetParentProposalBlockData() {
	ctx.mutexParentProposalBlock.Lock()
	defer ctx.mutexParentProposalBlock.Unlock()

	if ctx.parent.NumberU64() == 0 {
		return
	}

	headerNoCert := ctx.parent.Header()
	headerNoCert.Certificate = new(types.Certificate)

	certificate := ctx.parent.Certificate()
	ctx.parentProposalBlockData = NewProposalBlockDataFromProposalStorage(&certificate.Proposal, ctx.parent.WithSeal(headerNoCert))
}

func (ctx *Context) GetParentProposalBlockData(height uint64) *ProposalBlockData {
	ctx.mutexParentProposalBlock.RLock()
	defer ctx.mutexParentProposalBlock.RUnlock()

	if ctx.Height-1 != height {
		return nil
	}

	return ctx.parentProposalBlockData
}

func (ctx *Context) sortition() (hash ed25519.VrfOutput256, proof ed25519.VrfProof, j uint64) {
	if !ctx.IsMining() {
		return
	}

	mk, err := ctx.mkm.GetMinerKey(ctx.currentMiner, ctx.Height)
	if err != nil {
		log.Warn("GetMinerKey failed", "err", err, "HRS", ctx.HRS(), "miner", ctx.currentMiner)
		return
	}

	hash, proof, j, err = mk.Sortition(ctx.Height, ctx.Round, ctx.Step, ctx.parent.Seed(), ctx.parentStatedb, ctx.parent.TotalBalanceOfMiners())
	if err != nil {
		log.Error("Sortition failed", "err", err, "HRS", ctx.HRS(), "miner", ctx.currentMiner)
		return
	}

	return
}

// make a new block to proposal if value is common.Hash{}
// otherwise, get the block of value to proposal
// value should never be EmptyValue Ve
//
// 这里有个问题，如果value对应的block找不到怎么办？是不发呢，还是只发ProposalValue
// 这种情况在前一轮的block没收有可能收齐该block的next-vote，从而以这个值进入下一轮时会出现
// 也许应该作废，使用自己的proposalBlock来发
func (ctx *Context) sendProposal(value common.Hash, sortHash ed25519.VrfOutput256, proof ed25519.VrfProof) {
	if ctx.recover {
		log.Info("recover: sendProposal return", "HRS", ctx.HRS())
		return
	}

	proposer := ctx.currentMiner

	mk, err := ctx.mkm.GetMinerKey(proposer, ctx.Height)
	if err != nil {
		log.Warn("GetMinerKey failed", "err", err, "HRS", ctx.HRS(), "miner", proposer)
		return
	}

	proposalLeader := &ProposalLeaderData{
		Value: value,
		Credential: Credential{
			Address: proposer,
			Height:  ctx.Height,
			Round:   ctx.Round,
			Step:    types.RoundStep1Proposal,
			Proof:   proof,
		},
	}

	sig, err := mk.Sign(proposalLeader.Height, proposalLeader.SignBytes())
	if err != nil {
		log.Error("sendProposal: Error signing proposalLeader", "HRS", ctx.HRS(), "err", err)
		return
	}
	proposalLeader.ESignValue = sig

	sendToMessageChan(ctx.msgChan, message{ProposalLeaderMsg, proposalLeader, fromMe})
	ctx.broadcastMsg(ProposalLeaderMsg, proposalLeader) // broadcast ProposalLeaderMsg Now

	// 只在发的是自己的提案时，才发送ProposalBlock
	if ctx.newBlock != nil && value == ctx.newBlock.Hash() {
		proposalBlock := &ProposalBlockData{
			Block: ctx.newBlock,

			ESignValue: proposalLeader.ESignValue,
			Credential: proposalLeader.Credential,
		}
		sendToMessageChan(ctx.msgChan, message{ProposalBlockMsg, proposalBlock, fromMe})

		chanId := makeChanId(proposalLeader.Height, proposalLeader.Round)
		ctx.broadcaster.Publish(chanId, ProposalBlockMsg, proposalBlock)
	}

	log.Debug("sendProposal done", "HRS", ctx.HRS(), "proposal", proposalLeader)
}

// only sent to self, broadcast by gossip
func (ctx *Context) sendVote(voteType uint32, value common.Hash, sortHash ed25519.VrfOutput256, proof ed25519.VrfProof, j uint64) {
	if ctx.recover {
		log.Info("recover: sendVote return", "HRS", ctx.HRS(), "voteType", voteType, "value", value.TerminalString())
		return
	}

	if voteType != types.VoteTypeOfStep(ctx.Step) {
		panic(fmt.Sprintf("voteType not matches current step, voteType=%d HRS=%s", voteType, ctx.HRS()))
	}

	mk, err := ctx.mkm.GetMinerKey(ctx.currentMiner, ctx.Height)
	if err != nil {
		log.Warn("GetMinerKey failed", "err", err, "HRS", ctx.HRS(), "miner", ctx.currentMiner)
		return
	}

	vote := &VoteData{
		Value: value,
		Credential: Credential{
			Address: ctx.currentMiner,
			Height:  ctx.Height,
			Round:   ctx.Round,
			Step:    ctx.Step,
			Proof:   proof,
			Weight:  j,
		},
	}
	sig, err := mk.Sign(vote.Height, vote.SignBytes())
	if err != nil {
		log.Error("sendVote: Error signing vote", "HRS", ctx.HRS(), "err", err)
		return
	}
	vote.ESignValue = sig

	sendToMessageChan(ctx.msgChan, message{VoteMsg, vote, fromMe})
	// broadcast vote Now
	ctx.broadcastMsg(VoteMsg, vote)

	log.Trace("Signed and send vote message", "vote", vote)

}

func (ctx *Context) NewBlock() error {
	blockchain := ctx.eth.BlockChain()

	ctx.hsMutex.Lock()
	defer ctx.hsMutex.Unlock()

	// init for new block
	ctx.statedb = nil
	ctx.receipts = nil
	ctx.mortgageContractCounter = nil
	ctx.newBlock = nil

	// make header for new block
	start := time.Now()
	timestamp := start.Unix()
	if ctx.parent.Time() >= uint64(timestamp) {
		timestamp = int64(ctx.parent.Time() + 1)
	}
	// this will ensure we're not going off too far in the future
	if now := time.Now().Unix(); timestamp > now+1 {
		wait := time.Duration(timestamp-now) * time.Second
		log.Info("Mining too far in the future", "wait", common.PrettyDuration(wait))
		time.Sleep(wait)
	}

	statedb, err := blockchain.StateAt(ctx.parent.Root())
	if err != nil {
		log.Error("Failed to create mining context", "err", err)
		return err
	}

	num := ctx.parent.Number()
	height := num.Add(num, common.Big1)

	mv := GetMinerVerifier(ctx.config.Algorand, ctx.parentStatedb, ctx.currentMiner, height.Uint64())

	ctx.header = &types.Header{
		ParentHash:  ctx.parent.Hash(),
		Number:      height,
		GasLimit:    core.CalcGasLimit(ctx.parent, ctx.gasFloor, ctx.gasCeil),
		Extra:       ctx.extra,
		Time:        uint64(timestamp),
		Coinbase:    mv.Coinbase(),
		Certificate: new(types.Certificate), // must set it to empty value
	}
	ctx.header.SetVersion(params.ConsensusVersion)

	mk, err := ctx.mkm.GetMinerKey(ctx.currentMiner, height.Uint64())
	if err != nil {
		log.Warn("GetMinerKey failed", "err", err, "HRS", ctx.HRS(), "miner", ctx.currentMiner)
		return err
	}

	seed, proof, err := mk.Seed(height.Uint64(), ctx.parent.Seed(), ctx.parent.Hash())
	if err != nil {
		log.Error("Failed to calc Seed Hash", "err", err)
		return err
	}
	ctx.header.SetSeed(seed)
	ctx.header.Certificate.SetSeedProof(proof[:])
	ctx.header.Certificate.SetProposer(ctx.currentMiner)

	if err := ctx.engine.Prepare(blockchain, ctx.header); err != nil {
		log.Error("Failed to prepare header for mining", "err", err)
		return err
	}

	ctx.statedb = statedb
	ctx.statedb.EnableMinerTracker(ctx.config.Algorand, ctx.header.Number.Uint64(), ctx.parent.TotalBalanceOfMiners())

	prepareTime := time.Since(start)
	begin := time.Now()

	pending, err := ctx.eth.TxPool().Pending()
	if err != nil {
		log.Error("Failed to fetch pending transactions", "err", err)
		return err
	}
	pendingTime := time.Since(begin)
	begin = time.Now()

	signer := types.NewEIP155Signer(ctx.config.ChainID)
	txs := types.NewTransactionsByPriceAndNonce(signer, pending)
	validatedTxs, validatedReceipts, mortgageContractCounter := commitTransactions(ctx.quit, ctx.config, ctx.mux, ctx.eth, txs, ctx.statedb, ctx.header, signer)
	procTime := time.Since(begin)
	begin = time.Now()

	if !ctx.IsMining() {
		log.Debug("Context stopped, break NewBlock", "txs", len(validatedTxs))
		return errStopped
	}

	// Create the new block to seal with the consensus engine
	block, err := ctx.engine.Finalize(blockchain, ctx.header, ctx.statedb, validatedTxs, nil, validatedReceipts)
	if err != nil {
		log.Error("Failed to finalize block for sealing", "err", err)
		return err
	}
	finalizeTime := time.Since(begin)

	ctx.newBlock = block
	ctx.receipts = validatedReceipts
	ctx.mortgageContractCounter = mortgageContractCounter

	log.Info("Created new block", "number", ctx.newBlock.Number(), "hash", ctx.newBlock.Hash(),
		"txs", len(ctx.newBlock.Transactions()), "gasLimit", ctx.newBlock.GasLimit(), "gas", ctx.newBlock.GasUsed(),
		"stateRoot", ctx.newBlock.Root(), "elapsed", common.PrettyDuration(time.Since(start)),
		"prepare", common.PrettyDuration(prepareTime), "pending", common.PrettyDuration(pendingTime),
		"proc", common.PrettyDuration(procTime), "finalize", common.PrettyDuration(finalizeTime))

	return nil
}

func (ctx *Context) insertIntoBlockChain(block *types.Block) error {
	blockchain := ctx.eth.BlockChain()

	sealedValue := block.Hash()
	sealedCertifyingBlockCache := ctx.getVerifiedBlockState(sealedValue)

	if sealedCertifyingBlockCache == nil {
		// 对这里区块认为等同于sync过来的，要经过最全面的校验再写入
		// 这样实现比较符合eth的思路
		_, err := blockchain.InsertChain(types.Blocks{block})
		if err == nil {
			// Broadcast the block and announce chain insertion event
			_ = ctx.mux.Post(core.NewMinedBlockEvent{Block: block})
		}
		return err
	}

	if sealedCertifyingBlockCache.err != nil {
		log.Error("insert invalid block", "block", block, "err", sealedCertifyingBlockCache.err)
		return sealedCertifyingBlockCache.err
	}

	start := time.Now()

	// Update the block hash in all logs since it is now available and not when the
	// receipt/log of individual transactions were created.
	statedb := sealedCertifyingBlockCache.statedb
	receipts := sealedCertifyingBlockCache.receipts
	mortgageContractCounter := sealedCertifyingBlockCache.counter

	ctx.hsMutex.Lock()
	for _, r := range receipts {
		for _, l := range r.Logs {
			l.BlockHash = block.Hash()
		}
	}
	for _, l := range statedb.Logs() {
		l.BlockHash = block.Hash()
	}
	ctx.hsMutex.Unlock()

	stat, err := blockchain.WriteBlockWithStateIfNotExists(block, receipts, statedb)
	if err != nil {
		return err
	}

	log.Debug("Inserted new block", "number", block.Number(), "hash", block.Hash(),
		"txs", len(block.Transactions()), "gas", block.GasUsed(), "stateRoot", block.Root(),
		"elapsed", common.PrettyDuration(time.Since(start)))

	_ = ctx.mux.Post(core.NewMinedBlockEvent{Block: block})

	// Announce chain insertion event
	var (
		events []interface{}
		logs   = statedb.Logs()
	)
	events = append(events, core.ChainEvent{Block: block, Hash: block.Hash(), Logs: logs})
	if stat == core.CanonStatTy {
		events = append(events, core.ChainHeadEvent{Block: block})

		mortgageContractCounters := make([]core.HeightMortgageContractCounter, 1)
		mortgageContractCounters[0] = core.HeightMortgageContractCounter{
			Height:  block.NumberU64(),
			Counter: mortgageContractCounter,
		}
		blockchain.UpdateMortgage(mortgageContractCounters)
	} else {
		log.Warn("Sealed block is not canonical block", "height", block.NumberU64(), "hash", block.Hash(), "HRS", ctx.HRS())
	}
	blockchain.PostChainEvents(events, logs)

	return nil
}

func (ctx *Context) SetExtra(extra []byte) {
	// only used in init of HeightState, so we hold hsMutex
	ctx.hsMutex.Lock()
	defer ctx.hsMutex.Unlock()

	ctx.extra = extra
}

func (ctx *Context) SetMiner(address common.Address) {
	// only used in init of HeightState, so we hold hsMutex
	ctx.hsMutex.Lock()
	ctx.nextMiner = address
	if !ctx.IsMining() {
		ctx.currentMiner = address
	}
	ctx.hsMutex.Unlock()

	ctx.snapshotMutex.Lock()
	ctx.snapshot.setMiner(address)
	ctx.snapshotMutex.Unlock()
}

func (ctx *Context) getPendingFromSnapshot() (*types.Block, *state.StateDB) {
	ctx.snapshotMutex.RLock()
	defer ctx.snapshotMutex.RUnlock()

	// 为了与ethash返回pending block时coinbase为空保持一致
	header := types.CopyHeader(ctx.snapshot.header)
	header.Coinbase = common.Address{}
	block := types.NewBlock(header, ctx.snapshot.txs, nil, ctx.snapshot.receipts)
	return block, ctx.snapshot.statedb.Copy()
}

func (ctx *Context) saveVerifiedBlockState(value common.Hash, cache *blockState) {
	ctx.mutexVerifiedBlockState.Lock()
	defer ctx.mutexVerifiedBlockState.Unlock()

	ctx.verifiedBlockState[value.Str()] = cache
}

func (ctx *Context) getVerifiedBlockState(value common.Hash) *blockState {
	ctx.mutexVerifiedBlockState.RLock()
	defer ctx.mutexVerifiedBlockState.RUnlock()

	return ctx.verifiedBlockState[value.Str()]
}

func (ctx *Context) Pending() (*types.Block, *state.StateDB) {
	return ctx.getPendingFromSnapshot()
}

func (ctx *Context) RoundVoteSet(height uint64, round uint32) *RoundVoteSet {
	ctx.hsMutex.Lock()
	defer ctx.hsMutex.Unlock()

	if ctx.Height != height {
		return nil
	}
	return ctx.counter.RoundVoteSet(round)
}

func (ctx *Context) isSoftVoteEnoughForNonEmptyValueWhoseBlockIsReceivedAndValid(round uint32) (bool, common.Hash) {
	if exists, nonEmptyValue := ctx.counter.IsSoftVoteEnoughForNonEmpty(ctx.EmptyValue, round); exists {
		if block := ctx.getProposalBlock(nonEmptyValue); block != nil {
			if err := ctx.verifyBlockSync(block.Block); err == nil {
				return true, nonEmptyValue
			}
		}
	}
	return false, common.Hash{}
}

func (ctx *Context) Stake() uint64 {
	if !ctx.IsMining() {
		return 0
	}

	// must get latest block, not the ctx.parent which maybe old if context is locked by downloader
	currentBlock := ctx.eth.BlockChain().CurrentBlock()
	stateDb, err := ctx.eth.BlockChain().StateAt(currentBlock.Root())
	if err != nil {
		log.Error("Failed to get statedb", "err", err)
		return 0
	}

	minerContract := state.NewMinerContract(ctx.config.Algorand)
	isMiner := minerContract.IsMinerOf(stateDb, currentBlock.NumberU64(), ctx.currentMiner)
	if !isMiner {
		return 0
	}

	var hashHalf ed25519.VrfOutput256
	hashHalf[0] = 0x80

	weight, _ := GetWeight(ctx.config.Algorand, ctx.currentMiner, stateDb, currentBlock.TotalBalanceOfMiners(), hashHalf)

	return weight
}

func (ctx *Context) makeStampingVote(block *types.Block) *StampingVote {
	height := block.NumberU64()

	sortitionHash, sortitionProof, sortitionWeight := ctx.stampingSortition(height)
	if sortitionWeight == 0 {
		log.Trace("MakeStampingVote sortitionWeight == 0",
			"Height", height)
		return nil
	}
	_ = sortitionHash

	mk, err := ctx.mkm.GetMinerKey(ctx.currentMiner, height)
	if err != nil {
		log.Warn("GetMinerKey failed", "err", err, "Height", height, "miner", ctx.currentMiner)
		return nil
	}

	vote := &StampingVote{
		Value: block.Hash(),
		Credential: Credential{
			Address: ctx.currentMiner,
			Height:  block.NumberU64(),
			Round:   0,
			Step:    0,
			Proof:   sortitionProof,
			Weight:  sortitionWeight,
		},
	}
	sig, err := mk.Sign(vote.Height, vote.SignBytes())
	if err != nil {
		log.Error("sendVote: Error signing vote", "Height", height, "err", err)
		return nil
	}
	vote.ESignValue = sig

	return vote
}

func (ctx *Context) stampingSortition(height uint64) (hash ed25519.VrfOutput256, proof ed25519.VrfProof, j uint64) {
	if height <= ctx.config.Stamping.B {
		log.Warn("Height < B", "Height", height, "config.B", ctx.config.Stamping.B)
		return
	}

	mk, err := ctx.mkm.GetMinerKey(ctx.currentMiner, height)
	if err != nil {
		log.Warn("GetMinerKey failed", "err", err, "Height", height, "miner", ctx.currentMiner)
		return
	}

	parentHeight := height - ctx.config.Stamping.B
	parent := ctx.eth.BlockChain().GetHeaderByNumber(parentHeight)
	parentStatedb, err := ctx.eth.BlockChain().StateAtHeader(parent)
	if err != nil {
		log.Warn("StateAtHeader failed(stamping)", "err", err, "Height", parentHeight, "miner", ctx.currentMiner)
		return
	}

	hash, proof, j, err = mk.StampingSortition(height, parent.Seed(), parentStatedb, ctx.parent.TotalBalanceOfMiners())
	if err != nil {
		log.Error("Sortition failed", "err", err, "Height", height, "miner", ctx.currentMiner)
		return
	}

	return
}
