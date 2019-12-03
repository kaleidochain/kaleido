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
	"path/filepath"
	"sync"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/ethereum/go-ethereum/log"

	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/params"
)

const (
	dirname         = "minerkeys"
	DefaultLifespan = 100
	MaxLifespan     = 100
	MaxMinerKeySize = 200 * 1024 * 1024 // for lifespan = 1 on interval size = 1,000,000
)

type minerKeyId struct {
	miner common.Address
	begin uint64
	end   uint64
}

func (id minerKeyId) Filename(root string) string {
	name := fmt.Sprintf("%s-%d-%d.bin", id.miner.String(), id.begin, id.end)
	return filepath.Join(root, dirname, name)
}

type MinerKeyManager struct {
	config  *params.AlgorandConfig
	dataDir string

	mutex sync.RWMutex
	keys  map[minerKeyId]*MinerKey

	quit chan struct{}
}

func NewMinerKeyManager(config *params.AlgorandConfig, dataDir string) *MinerKeyManager {
	mkm := &MinerKeyManager{
		config:  config,
		dataDir: dataDir,
		keys:    make(map[minerKeyId]*MinerKey),
		quit:    make(chan struct{}),
	}

	return mkm
}

func (mkm *MinerKeyManager) GetMinerKey(miner common.Address, height uint64) (*MinerKey, error) {
	sn := mkm.config.GetIntervalSn(height)
	begin, end := mkm.config.GetInterval(sn)
	id := minerKeyId{miner, begin, end}

	mkm.mutex.RLock()
	mk, has := mkm.keys[id] // todo: MinerKey in memory is never unloaded if it does not expire
	mkm.mutex.RUnlock()

	if !has {
		filename := id.Filename(mkm.dataDir)
		f, err := os.Open(filename)
		if err != nil {
			return nil, err
		}
		defer func(file *os.File) {
			_ = file.Close()
		}(f)

		stream := rlp.NewStream(f, MaxMinerKeySize)
		mk = NewEmptyMinerKey(mkm.config)
		err = mk.DecodeRLP(stream)
		if err != nil {
			return nil, err
		}

		mkm.mutex.Lock()
		mkm.keys[id] = mk
		mkm.mutex.Unlock()
	}

	return mk, nil
}

func (mkm *MinerKeyManager) Generate(miner, coinbase common.Address, start uint64, lifespan uint32) (mv *MinerVerifier, mk *MinerKey, err error) {
	if lifespan == 0 {
		lifespan = DefaultLifespan
	}
	if lifespan > MaxLifespan {
		err = fmt.Errorf("lifespan cannot greater than %d", MaxLifespan)
		return
	}

	sn := mkm.config.GetIntervalSn(start)
	begin, end := mkm.config.GetInterval(sn)
	id := minerKeyId{miner, begin, end}

	filename := id.Filename(mkm.dataDir)
	_, err = os.Stat(filename)
	if err == nil {
		err = fmt.Errorf("miner key already exists at %s", filename)
		return
	}

	err = os.MkdirAll(filepath.Dir(filename), 0700)
	if err != nil {
		return
	}

	var file *os.File
	file, err = os.Create(filename)
	if err != nil {
		return
	}

	mv, mk, err = generateMinerKey(mkm.config, miner, coinbase, start, lifespan)
	if err != nil {
		_ = file.Close()
		return
	}

	err = mk.EncodeRLP(file)
	if err != nil {
		_ = file.Close()
		return
	}

	err = file.Close()
	return
}

func (mkm *MinerKeyManager) StartUpdateRoutine(chainHeadCh chan uint64) {
	go mkm.updateRoutine(chainHeadCh)
}

func (mkm *MinerKeyManager) updateRoutine(chainHeadCh chan uint64) {
	for {
		select {
		case current := <-chainHeadCh:
			// unload what we don't need
			toUnload := make([]minerKeyId, 0)

			mkm.mutex.RLock()
			for id, v := range mkm.keys {
				if v.Validate(current) {
					deleted := v.Update(current)
					if deleted == 0 {
						continue
					}

					// write new data to file
					// todo: move this to another routine, and reduce write frequency in case lifespan is very small
					mkm.updateMinerKeyStore(v)
				} else {
					toUnload = append(toUnload, id)
				}
			}
			mkm.mutex.RUnlock()

			mkm.mutex.Lock()
			for _, id := range toUnload {
				delete(mkm.keys, id)
			}
			mkm.mutex.Unlock()

		case <-mkm.quit:
			log.Info("MinerKeyManager updateRoutine exited")
			return
		}
	}
}

func (mkm *MinerKeyManager) updateMinerKeyStore(mk *MinerKey) {
	filename := mk.id().Filename(mkm.dataDir)
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_TRUNC, 0)

	if err == nil {
		err = mk.EncodeRLP(file)
	}

	if err != nil {
		log.Warn("MinerKeyManager update routine failed", "filename", filename, "err", err)
	}

	_ = file.Close()
}

func (mkm *MinerKeyManager) Stop() {
	close(mkm.quit)
}
