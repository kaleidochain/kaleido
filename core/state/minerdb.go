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

package state

import (
	"encoding/binary"
	"math/big"

	"github.com/kaleidochain/kaleido/contracts"

	"github.com/kaleidochain/kaleido/params"

	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/crypto"
)

var (
	zeroHash = common.Uint64ToHash(0)
	oneHash  = common.Uint64ToHash(1)
)

type MinerContract struct {
	config *params.AlgorandConfig
}

func NewMinerContract(config *params.AlgorandConfig) *MinerContract {
	return &MinerContract{
		config: config,
	}
}

func (minerdb *MinerContract) MakeMinerInfoKey(height uint64, miner common.Address) common.Hash {
	minerMapHash := zeroHash
	minerHash := miner.Hash()
	snHash := common.Uint64ToHash(minerdb.config.GetIntervalSn(height))
	//minerMap[sn][miner]
	key := crypto.Keccak256(append(minerHash.Bytes(),
		crypto.Keccak256(append(snHash.Bytes(), minerMapHash.Bytes()...))...))
	return common.BytesToHash(key)
}

func (minerdb *MinerContract) MakeNewMinersKey(height uint64) common.Hash {
	newMinersMapHash := oneHash
	heightHash := common.Uint64ToHash(height)
	return common.BytesToHash(crypto.Keccak256(append(heightHash.Bytes(), newMinersMapHash.Bytes()...)))
}

func (minerdb *MinerContract) Get(stateDb *StateDB, height uint64, miner common.Address) (uint64, uint32, common.Address, [32]byte, [32]byte) {
	minerInfoKey := minerdb.MakeMinerInfoKey(height, miner)

	first256 := stateDb.GetState(contracts.MinerAddress, minerInfoKey)
	start, lifespan, coinbase := parseStartLifespanCoinbase(&first256)

	if start == 0 {
		return 0, 0, common.Address{}, common.Hash{}, common.Hash{}
	}

	offset := minerInfoKey.Big()
	offset = offset.Add(offset, common.Big1)
	vrfVerifier := stateDb.GetState(contracts.MinerAddress, common.BigToHash(offset))

	offset = offset.Add(offset, common.Big1)
	voteVerifier := stateDb.GetState(contracts.MinerAddress, common.BigToHash(offset))

	return start, lifespan, coinbase, vrfVerifier, voteVerifier
}

func (minerdb *MinerContract) NewAddedMiners(stateDb *StateDB, height uint64) []common.Address {
	newMinersKey := minerdb.MakeNewMinersKey(height)
	newMinerNum := stateDb.GetState(contracts.MinerAddress, newMinersKey).Big()

	num := newMinerNum.Uint64()
	if num == 0 {
		return nil
	}

	miners := make([]common.Address, 0, num)

	pos := common.BytesToHash(crypto.Keccak256(newMinersKey.Bytes())).Big()
	end := new(big.Int).Add(pos, newMinerNum)
	for pos.Cmp(end) < 0 {
		miner := common.BytesToAddress(stateDb.GetState(contracts.MinerAddress, common.BigToHash(pos)).Bytes())
		miners = append(miners, miner)
		pos = pos.Add(pos, common.Big1)
	}

	return miners
}

// IsMinerOf returns if the miner participates mining of the block height
func (minerdb *MinerContract) IsMinerOf(stateDb *StateDB, height uint64, miner common.Address) bool {
	minerInfoKey := minerdb.MakeMinerInfoKey(height, miner)
	first256 := stateDb.GetState(contracts.MinerAddress, minerInfoKey)
	start, _, _ := parseStartLifespanCoinbase(&first256)

	return isMinerOf(height, start)
}

func isMinerOf(height, begin uint64) bool {
	return begin != 0 && height >= begin
}

func parseStartLifespanCoinbase(data *common.Hash) (uint64, uint32, common.Address) {
	start := binary.BigEndian.Uint64(data[common.HashLength-8:])
	lifespan := binary.BigEndian.Uint32(data[common.HashLength-8-4 : common.HashLength-8])
	coinbase := common.BytesToAddress(data[common.HashLength-8-4-20 : common.HashLength-8-4])

	return start, lifespan, coinbase
}

func MakeStartLifespanCoinbase(start uint64, lifespan uint32, coinbase common.Address) (data common.Hash) {
	binary.BigEndian.PutUint64(data[common.HashLength-8:], start)
	binary.BigEndian.PutUint32(data[common.HashLength-8-4:common.HashLength-8], lifespan)
	copy(data[common.HashLength-8-4-20:common.HashLength-8-4], coinbase[:])

	return
}
