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
	"errors"
	"fmt"
	"math/big"

	"github.com/kaleidochain/kaleido/consensus/algorand/core/sortition"

	"github.com/kaleidochain/kaleido/core/state"

	"github.com/kaleidochain/kaleido/common/hexutil"

	"github.com/kaleidochain/kaleido/accounts/abi"
	"github.com/kaleidochain/kaleido/contracts"

	"github.com/kaleidochain/kaleido/params"

	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/crypto/ed25519"
)

type MinerVerifier struct {
	config *params.AlgorandConfig `rlp:"-"`

	miner        common.Address
	coinbase     common.Address
	start        uint64
	end          uint64
	lifespan     uint32
	vrfVerifier  ed25519.VrfPublicKey
	voteVerifier ed25519.ForwardSecurePublicKey
}

func (mv *MinerVerifier) Coinbase() common.Address {
	return mv.coinbase
}

func (mv *MinerVerifier) VerifySeed(height uint64, parentSeed ed25519.VrfOutput256, parentHash common.Hash, seed ed25519.VrfOutput256, proof ed25519.VrfProof) error {
	if !mv.Validate(height) {
		return errors.New("miner is not registered")
	}

	data := seedData{
		mv.miner, height, parentSeed, parentHash,
	}

	ok, seedCheck := ed25519.VrfVerify256(&mv.vrfVerifier, &proof, data.Bytes())

	if !ok {
		return errors.New("bad proof")
	}

	if seed != seedCheck {
		return errors.New("bad seed")
	}

	return nil
}

func (mv *MinerVerifier) VerifySortition(height uint64, round, step uint32, proof ed25519.VrfProof, parentSeed ed25519.VrfOutput256, parentState *state.StateDB, totalBalanceOfMiners *big.Int) (err error, j uint64) {
	if !mv.Validate(height) {
		err = errors.New("miner is not registered")
		return
	}

	data := sortitionData{
		mv.miner, height, round, step, parentSeed,
	}

	ok, hash := ed25519.VrfVerify256(&mv.vrfVerifier, &proof, data.Bytes())
	if !ok {
		err = errors.New("invalid vrf proof")
		return
	}

	ownWeight, totalWeight := GetWeight(mv.config, mv.miner, parentState, totalBalanceOfMiners, hash)

	threshold, size := GetCommitteeNumber(height, step)
	j = sortition.Choose(hash, ownWeight, threshold, size, totalWeight)

	if j == 0 {
		err = errors.New("sortition weight is 0")
	}

	return
}

func (mv *MinerVerifier) VerifySignature(height uint64, data []byte, sig ed25519.ForwardSecureSignature) error {
	if !mv.Validate(height) {
		return errors.New("miner is not registered")
	}

	offset := height / uint64(mv.lifespan)

	ok := ed25519.ForwardSecureVerify(&mv.voteVerifier, offset, data, sig)

	if !ok {
		return errors.New("bad signature")
	}
	return nil
}

func (mv *MinerVerifier) AbiString() string {
	minerAbi, err := abi.JSON(bytes.NewReader([]byte(contracts.MinerAbi)))
	if err != nil {
		panic("bad abi string")
	}

	key, err := minerAbi.Pack("set", mv.start, mv.lifespan, mv.coinbase,
		mv.vrfVerifier, mv.voteVerifier)
	if err != nil {
		panic(fmt.Sprintf("abi pack failed, err:%s", err))
	}

	return hexutil.Encode(key)
}

func (mv *MinerVerifier) String() string {
	return fmt.Sprintf("miner = %s, coinbase = %s, start = %d, end = %d, lifespan = %d, vrfVerifier = 0x%x, voteVerfier = 0x%x",
		mv.miner.String(), mv.coinbase.String(), mv.start, mv.end, mv.lifespan,
		mv.vrfVerifier,
		mv.voteVerifier)
}

func (mv *MinerVerifier) GenesisString() string {
	minerContract := state.NewMinerContract(mv.config)

	offset := minerContract.MakeMinerInfoKey(mv.start, mv.miner).Big()
	first256 := common.BytesToHash(offset.Bytes())
	offset = offset.Add(offset, common.Big1)
	second256 := common.BytesToHash(offset.Bytes())
	offset = offset.Add(offset, common.Big1)
	third256 := common.BytesToHash(offset.Bytes())

	return fmt.Sprintf("0x%x = 0x%x, 0x%x = 0x%x, 0x%x = 0x%x",
		first256, state.MakeStartLifespanCoinbase(mv.start, mv.lifespan, mv.coinbase),
		second256, mv.vrfVerifier,
		third256, mv.voteVerifier)
}

func (mv *MinerVerifier) Validate(height uint64) bool {
	return mv.start != 0 && mv.start <= height && height < mv.end
}
