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
	"sync/atomic"
	"time"

	"github.com/kaleidochain/kaleido/consensus/algorand/core"
	"github.com/kaleidochain/kaleido/p2p"

	"github.com/ethereum/go-ethereum/log"
	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/consensus"
	"github.com/kaleidochain/kaleido/core/state"
	"github.com/kaleidochain/kaleido/core/types"
	"github.com/kaleidochain/kaleido/eth/downloader"
	"github.com/kaleidochain/kaleido/event"
	"github.com/kaleidochain/kaleido/params"
)

// Miner creates blocks and make consensus using Algorand and gossip p2p.
type Miner struct {
	mux *event.TypeMux

	coinbase common.Address
	mining   int32
	engine   consensus.Engine

	gossiper *ProtocolManager

	canStart    int32 // can start indicates whether we can start the mining operation
	shouldStart int32 // should start indicates whether we should start after sync
}

func NewMiner(eth core.Backend, config *params.ChainConfig, mux *event.TypeMux, engine consensus.Engine, ephemeralKeyDir string, gasFloor, gasCeil uint64) *Miner {
	miner := &Miner{
		mux:      mux,
		engine:   engine,
		gossiper: NewProtocolManager(eth, config, mux, engine, ephemeralKeyDir, gasFloor, gasCeil),
		canStart: 1,
	}

	go miner.update() // 是起什么作用，还不是很明白

	return miner
}

// update keeps track of the downloader events. Please be aware that this is a one shot type of update loop.
// It's entered once and as soon as `Done` or `Failed` has been broadcasted the events are unregistered and
// the loop is exited. This to prevent a major security vuln where external parties can DOS you with blocks
// and halt your mining operation for as long as the DOS continues.
func (m *Miner) update() {
	events := m.mux.Subscribe(downloader.StartEvent{}, downloader.DoneEvent{}, downloader.FailedEvent{})
out:
	for ev := range events.Chan() {
		switch ev.Data.(type) {
		case downloader.StartEvent:
			atomic.StoreInt32(&m.canStart, 0)
			if m.Mining() {
				log.Info("Abort mining due to sync")
				m.Stop()
				atomic.StoreInt32(&m.shouldStart, 1)
				log.Info("Mining aborted due to sync")
			}
		case downloader.DoneEvent, downloader.FailedEvent:
			shouldStart := atomic.LoadInt32(&m.shouldStart) == 1

			atomic.StoreInt32(&m.canStart, 1)
			atomic.StoreInt32(&m.shouldStart, 0)
			if shouldStart {
				log.Info("Start mining after sync")
				m.Start(m.coinbase)
				log.Info("Mining started after sync")
			}
			// unsubscribe. we're only interested in this event once
			events.Unsubscribe()
			// stop immediately and ignore all further pending events
			break out
		}
	}
}

func (m *Miner) StartService() {
	log.Info("Starting mine running operation")
	m.gossiper.Start()
}

func (m *Miner) StopService() {
	log.Info("Stopping mine running operation")
	m.gossiper.Stop()
	log.Info("Miner Stopped")
}

func (m *Miner) Start(coinbase common.Address) {
	m.SetEtherbase(coinbase)
	atomic.StoreInt32(&m.shouldStart, 1)

	if atomic.LoadInt32(&m.canStart) == 0 {
		log.Info("Network syncing, will start miner afterwards")
		return
	}
	atomic.StoreInt32(&m.mining, 1)

	log.Info("Starting mining operation")
	m.gossiper.setEtherbase(coinbase)
	m.gossiper.StartMining()
}

func (m *Miner) Stop() {
	log.Info("Stopping mining operation")
	m.gossiper.StopMining()
	atomic.StoreInt32(&m.mining, 0)
	atomic.StoreInt32(&m.shouldStart, 0)
}

func (m *Miner) Mining() bool {
	if atomic.LoadInt32(&m.mining) > 0 {
		return m.gossiper.Mining()
	}
	return false
}

func (m *Miner) HashRate() (tot uint64) {
	return m.gossiper.Stake()
}

func (m *Miner) SetExtra(extra []byte) error {
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		return fmt.Errorf("extra exceeds max length. %d > %v", len(extra), params.MaximumExtraDataSize)
	}
	m.gossiper.setExtra(extra)
	return nil
}

// Pending returns the currently pending block and associated state.
func (m *Miner) Pending() (*types.Block, *state.StateDB) {
	return m.gossiper.pending()
}

// PendingBlock returns the currently pending block.
//
// Note, to access both the pending block and the pending state
// simultaneously, please use Pending(), as the pending state can
// change between multiple method calls
func (m *Miner) PendingBlock() *types.Block {
	return m.gossiper.pendingBlock()
}

func (m *Miner) SetEtherbase(addr common.Address) {
	m.coinbase = addr
	m.gossiper.setEtherbase(addr)
}

func (m *Miner) Protocols() []p2p.Protocol {
	return m.gossiper.SubProtocols
}

func (m *Miner) SetRecommitInterval(interval time.Duration) {
}
