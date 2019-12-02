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
	"errors"
	"io"
	"math/big"
	"sync"

	"github.com/kaleidochain/kaleido/core"
	"github.com/kaleidochain/kaleido/sortition"

	"github.com/kaleidochain/kaleido/core/state"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/crypto/ed25519"
	"github.com/kaleidochain/kaleido/params"
)

type mkdata struct {
	Miner    common.Address                  `json:"miner" gencodec:"required"`
	Coinbase common.Address                  `json:"coinbase" gencodec:"required"`
	Start    uint64                          `json:"start" gencodec:"required"`
	End      uint64                          `json:"end" gencodec:"required"`
	Lifespan uint32                          `json:"lifespan" gencodec:"required"`
	VrfKey   ed25519.VrfPrivateKey           `json:"vrfkey" gencodec:"required"`
	VoteKey  ed25519.ForwardSecurePrivateKey `json:"votekey" gencodec:"required"`
}

type MinerKey struct {
	config *params.AlgorandConfig `rlp:"-"`
	mutex  sync.RWMutex           `rlp:"-"`

	mkdata
}

func NewEmptyMinerKey(config *params.AlgorandConfig) *MinerKey {
	return &MinerKey{
		config: config,
	}
}

func (mk *MinerKey) Seed(height uint64, parentSeed ed25519.VrfOutput256, parentHash common.Hash) (seed ed25519.VrfOutput256, proof ed25519.VrfProof, err error) {
	mk.mutex.RLock()
	defer mk.mutex.RUnlock()

	if !mk.Validate(height) {
		err = errors.New("miner is not registered")
		return
	}

	data := seedData{
		mk.Miner, height, parentSeed, parentHash,
	}
	var ok bool

	proof, ok = ed25519.VrfProve(&mk.VrfKey, data.Bytes())
	if !ok {
		err = errors.New("vrf prove failed")
		return
	}

	seed, ok = ed25519.VrfProofToHash256(&proof)
	if !ok {
		err = errors.New("vrf prove to hash256 failed")
		return
	}

	return
}

func (mk *MinerKey) Sortition(height uint64, round, step uint32, parentSeed ed25519.VrfOutput256, parentState *state.StateDB, totalBalanceOfMiners *big.Int) (hash ed25519.VrfOutput256, proof ed25519.VrfProof, j uint64, err error) {
	data := sortitionData{
		mk.Miner, height, round, step, parentSeed,
	}
	threshold, totalNumber := GetCommitteeNumber(height, step)

	return mk.sortition(height, data.Bytes(), parentSeed, parentState, totalBalanceOfMiners, threshold, totalNumber)
}

func (mk *MinerKey) StampingSortition(height uint64, parentSeed ed25519.VrfOutput256, parentState *state.StateDB, totalBalanceOfMiners *big.Int) (hash ed25519.VrfOutput256, proof ed25519.VrfProof, j uint64, err error) {
	data := stampingSortitionData{
		mk.Miner, height, parentSeed,
	}
	threshold, totalNumber := GetStampingCommitteeNumber(height)

	return mk.sortition(height, data.Bytes(), parentSeed, parentState, totalBalanceOfMiners, threshold, totalNumber)
}

func (mk *MinerKey) sortition(height uint64, sortitionData []byte, parentSeed ed25519.VrfOutput256, parentState *state.StateDB,
	totalBalanceOfMiners *big.Int, threshold, totalNumber uint64) (hash ed25519.VrfOutput256, proof ed25519.VrfProof, j uint64, err error) {
	mk.mutex.RLock()
	defer mk.mutex.RUnlock()

	if !mk.Validate(height) {
		err = errors.New("miner is not registered")
		return
	}

	var ok bool
	proof, ok = ed25519.VrfProve(&mk.VrfKey, sortitionData)
	if !ok {
		err = errors.New("invalid vrf key")
		return
	}

	hash, ok = ed25519.VrfProofToHash256(&proof)
	if !ok {
		err = errors.New("invalid proof")
		return
	}

	ownWeight, totalWeight := core.GetWeight(mk.config, mk.Miner, parentState, totalBalanceOfMiners, hash)
	j = sortition.Choose(hash, ownWeight, threshold, totalNumber, totalWeight)
	return
}

func (mk *MinerKey) Sign(height uint64, data []byte) (sig ed25519.ForwardSecureSignature, err error) {
	mk.mutex.RLock()
	defer mk.mutex.RUnlock()

	if !mk.Validate(height) {
		err = errors.New("invalid height")
		return
	}

	offset := height / uint64(mk.Lifespan)
	if !mk.VoteKey.ValidOffset(offset) {
		err = errors.New("invalid offset")
		return
	}

	sig = ed25519.ForwardSecureSign(&mk.VoteKey, offset, data)
	return
}

func (mk *MinerKey) Validate(height uint64) bool {
	return mk.Start != 0 && mk.Start <= height && height < mk.End
}

func (mk *MinerKey) Update(height uint64) uint64 {
	mk.mutex.Lock()
	defer mk.mutex.Unlock()

	offset := height / uint64(mk.Lifespan)
	return ed25519.ForwardSecureUpdate(&mk.VoteKey, offset)
}

func (mk *MinerKey) EncodeRLP(writer io.Writer) error {
	mk.mutex.RLock()
	defer mk.mutex.RUnlock()

	return rlp.Encode(writer, &mk.mkdata)
}

func (mk *MinerKey) DecodeRLP(stream *rlp.Stream) error {
	mk.mutex.Lock()
	defer mk.mutex.Unlock()

	return stream.Decode(&mk.mkdata)
}

func (mk *MinerKey) ToVerifier() *MinerVerifier {
	mv := &MinerVerifier{
		config: mk.config,

		miner:        mk.Miner,
		coinbase:     mk.Coinbase,
		start:        mk.Start,
		end:          mk.End,
		lifespan:     mk.Lifespan,
		vrfVerifier:  ed25519.VrfPrivateKeyToPublicKey(&mk.VrfKey),
		voteVerifier: ed25519.ForwardSecurePrivateKeyToPublicKey(&mk.VoteKey),
	}

	return mv
}

func (mk *MinerKey) id() minerKeyId {
	sn := mk.config.GetIntervalSn(mk.Start)
	begin, _ := mk.config.GetInterval(sn)
	return minerKeyId{mk.Miner, begin, mk.End}
}
