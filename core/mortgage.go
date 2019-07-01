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
	"math/big"
	"sync"

	"github.com/kaleidochain/kaleido/core/syscon"

	"github.com/kaleidochain/kaleido/core/vm"

	lru "github.com/hashicorp/golang-lru"

	"github.com/ethereum/go-ethereum/log"
	"github.com/kaleidochain/kaleido/core/state"

	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/core/types"
)

const (
	slotCount = 24
	step      = 1200
	maxHeight = 28800

	entriesCacheLimit = 100 * 1000
)

var cPrice = new(big.Int).SetUint64(2 * 1e6)

type MortgageContractCounter map[common.Address]uint64
type HeightMortgageContractCounter struct {
	Height  uint64
	Counter MortgageContractCounter
}

type circleQueue struct {
	curSlotStartHeight uint64
	total              uint64
	slots              [slotCount]uint64
	pos                uint64
}

func (q *circleQueue) Add(height uint64, count uint64) {
	if height < q.curSlotStartHeight {
		return
	}
	if height >= q.curSlotStartHeight+maxHeight {
		for i := 0; i < slotCount; i++ {
			q.slots[i] = 0
		}

		q.curSlotStartHeight = (height / step) * step
		q.pos = (height % maxHeight) / step
		q.slots[q.pos] = count
		q.total = count
		return
	}

	curPos := (height % maxHeight) / step
	if curPos == q.pos {
		q.slots[q.pos] += count

		q.total += count

		return
	}

	gap := (slotCount + curPos - q.pos) % slotCount
	for i := uint64(1); i <= gap; i++ {
		index := (i + q.pos) % slotCount
		q.total -= q.slots[index]
		q.slots[index] = 0
		q.curSlotStartHeight += step
	}

	q.pos = curPos
	q.slots[q.pos] = count
	q.total += count
}

type MortgageEntry struct {
	counter circleQueue
}

type MortgageWorker struct {
	entries  *lru.Cache
	quotas   map[common.Address]*big.Int
	pendings map[common.Address]uint64

	height uint64
	mutex  sync.RWMutex
}

func NewMortgage() *MortgageWorker {
	entriesCache, err := lru.New(entriesCacheLimit)
	if err != nil {
		log.Error("LRU new", "err", err)
		panic("LRU new failed")
	}

	worker := &MortgageWorker{
		entries: entriesCache,

		height: 0,
	}
	worker.quotas = make(map[common.Address]*big.Int)
	worker.pendings = make(map[common.Address]uint64)

	return worker
}

func (l *MortgageWorker) resetAllEntry() {
	l.entries.Purge()
}

func isSystemContract(addr common.Address) bool {
	systemContractPrefix := [common.AddressLength - 1]byte{}
	systemContractPrefix[0] = 0x10
	if bytes.Compare(addr[:common.AddressLength-1], systemContractPrefix[:]) == 0 {
		return true
	}

	return false
}

func ProcessTx(mortgageContractCounter MortgageContractCounter, statedb *state.StateDB, tx *types.Transaction) {
	if isCallOrdinaryContract(statedb, tx) {
		creator := vm.GetContractCreator(statedb, tx.To())
		mortgageContractCounter[creator]++
	}
}

func isCallOrdinaryContract(statedb *state.StateDB, tx *types.Transaction) bool {
	return syscon.IsCallContract(statedb, tx) && !isSystemContract(*tx.To())
}

// 获取合约创建者对应的配额
// total = balance(creator) + GetTotalReceived(creator)
func (l *MortgageWorker) getQuota(statedb *state.StateDB, contract common.Address, creator common.Address, gasPrice *big.Int) *big.Int {
	if quota, ok := l.quotas[creator]; ok {
		return quota
	}

	value := new(big.Int).SetUint64(0)

	value.Add(value, statedb.GetBalance(creator))
	value.Add(value, state.GetTotalReceived(statedb, creator))

	value.Div(value, gasPrice)
	value.Div(value, cPrice)

	l.quotas[creator] = value

	return value
}

func (l *MortgageWorker) Updates(counters []HeightMortgageContractCounter) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	for _, m := range counters {
		for creator, c := range m.Counter {
			entry := l.getEntry(creator)
			entry.counter.Add(m.Height, c)
		}
	}

	l.quotas = make(map[common.Address]*big.Int)
	l.pendings = make(map[common.Address]uint64)
}

func (l *MortgageWorker) getEntry(creator common.Address) *MortgageEntry {
	var entry *MortgageEntry = nil
	if entryObj, exist := l.entries.Get(creator); exist {
		entry = entryObj.(*MortgageEntry)
	} else {
		entry = new(MortgageEntry)
		l.entries.Add(creator, entry)
	}

	return entry
}

func (l *MortgageWorker) Passable(statedb *state.StateDB, tx *types.Transaction, gasPrice *big.Int) (bool, *common.Address) {
	if !isCallOrdinaryContract(statedb, tx) {
		return true, nil
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	creator := vm.GetContractCreator(statedb, tx.To())

	entry := l.getEntry(creator)
	quota := l.getQuota(statedb, *tx.To(), creator, gasPrice)
	pending := l.pendings[creator]
	if quota.Cmp(new(big.Int).SetUint64(entry.counter.total+pending)) > 0 {
		return true, &creator
	}

	return false, nil
}

func (l *MortgageWorker) AddPending(creator *common.Address) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.pendings[*creator]++
}
