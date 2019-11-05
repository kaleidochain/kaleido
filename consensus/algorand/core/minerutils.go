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
	"math/big"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/core/state"
	"github.com/kaleidochain/kaleido/crypto/ed25519"
	"github.com/kaleidochain/kaleido/params"
)

func generateMinerKey(config *params.AlgorandConfig, miner, coinbase common.Address, start uint64, lifespan uint32) (*MinerVerifier, *MinerKey, error) {
	sn := config.GetIntervalSn(start)
	_, end := config.GetInterval(sn)

	offset := start / uint64(lifespan)
	limit := end / uint64(lifespan)
	if offset >= limit {
		return nil, nil, errors.New("invalid start and lifespan")
	}

	count := limit - offset

	vrfVerifier, vrfKey := ed25519.VrfGenerateKey()
	voteVerifier, voteKey := ed25519.GenerateForwardSecureKey(offset, count)

	mv := &MinerVerifier{
		config:       config,
		miner:        miner,
		coinbase:     coinbase,
		start:        start,
		end:          end,
		lifespan:     lifespan,
		vrfVerifier:  vrfVerifier,
		voteVerifier: voteVerifier,
	}

	mk := &MinerKey{
		config: config,
		mkdata: mkdata{
			Miner:    miner,
			Coinbase: coinbase,
			Start:    start,
			End:      end,
			Lifespan: lifespan,
			VrfKey:   vrfKey,
			VoteKey:  voteKey,
		},
	}

	return mv, mk, nil
}

type seedData struct {
	Miner      common.Address
	Height     uint64
	ParentSeed ed25519.VrfOutput256
	ParentHash common.Hash
}

func (d *seedData) Bytes() []byte {
	data, _ := rlp.EncodeToBytes(d)
	return data
}

type sortitionData struct {
	Miner      common.Address
	Height     uint64
	Round      uint32
	Step       uint32
	ParentSeed ed25519.VrfOutput256
}

func (d *sortitionData) Bytes() []byte {
	data, _ := rlp.EncodeToBytes(d)
	return data
}

func GetMinerVerifier(config *params.AlgorandConfig, statedb *state.StateDB, miner common.Address, height uint64) *MinerVerifier {
	minerdb := state.NewMinerContract(config)
	start, lifespan, coinbase, vrfVerifier, voteVerifier := minerdb.Get(statedb, height, miner)

	_, end := config.GetInterval(config.GetIntervalSn(start))

	return &MinerVerifier{
		config: config,

		miner:        miner,
		coinbase:     coinbase,
		start:        start,
		end:          end,
		lifespan:     lifespan,
		vrfVerifier:  vrfVerifier,
		voteVerifier: voteVerifier,
	}
}

func VerifySignatureAndCredential(mv *MinerVerifier, signBytes []byte, signature ed25519.ForwardSecureSignature, credential *Credential, stateDb *state.StateDB, parentSeed ed25519.VrfOutput256, totalBalanceOfMiners *big.Int) error {
	err := mv.VerifySignature(credential.Height, signBytes, signature)
	if err != nil {
		log.Warn("verify signature failed", "err", err)
		return err
	}

	err, choosedWeight := mv.VerifySortition(credential.Height, credential.Round, credential.Step, credential.Proof, parentSeed, stateDb, totalBalanceOfMiners)
	if err != nil {
		log.Warn("verify credential failed", "err", err, "credential", credential)
		return err
	}

	credential.Weight = choosedWeight
	return nil
}
