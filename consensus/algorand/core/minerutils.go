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
	"math/big"

	"github.com/kaleidochain/kaleido/common/hexutil"

	"github.com/ethereum/go-ethereum/log"
	"github.com/kaleidochain/kaleido/core/state"
	"github.com/kaleidochain/kaleido/core/types"

	"github.com/ethereum/go-ethereum/rlp"
	"github.com/kaleidochain/kaleido/common"
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

// GetWeight returns weight of a miner. It's caller's responsibility to check that addr is a miner of current block
func GetWeight(config *params.AlgorandConfig, miner common.Address, stateDb *state.StateDB, totalBalanceOfMiners *big.Int, hash ed25519.VrfOutput256) (ownWeight, totalWeight uint64) {
	ownBalance := state.GetBalanceWithFund(stateDb, miner)

	var hashHalf ed25519.VrfOutput256
	hashHalf[0] = 0x80

	// ownWeight = TotalWeight * ownBalance / TotalBalanceOfMiners ... reminder
	ownWeightBig := new(big.Int).Mul(config.TotalWeight, ownBalance)
	reminder := new(big.Int)
	ownWeightBig.QuoRem(ownWeightBig, totalBalanceOfMiners, reminder)

	ownWeight = ownWeightBig.Uint64()

	var delta uint64
	if reminder.Uint64() != 0 {
		r := bytes.Compare(hash[:], hashHalf[:])

		if r < 0 {
			delta = 0
		} else if r > 0 {
			delta = 1
		} else {
			if ownWeight%2 == 0 {
				delta = 0
			} else {
				delta = 1
			}
		}
	}
	ownWeight += delta
	totalWeight = config.TotalWeight.Uint64()

	log.Trace("GetWeight", "address", miner, "totalBalance", totalBalanceOfMiners, "totalWeight", totalWeight,
		"ownBalance", ownBalance, "ownWeight", ownWeight, "reminder", reminder, "hash", hexutil.Encode(hash[:3]))

	return
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

func GetCommitteeNumber(height uint64, step uint32) (uint64, uint64) {
	const proposerThreshold = 1
	if step == types.RoundStep1Proposal {
		return proposerThreshold, params.CommitteeConfigv1.NumProposer
	}

	if step == types.RoundStep2Filtering {
		return params.CommitteeConfigv1.SoftCommitteeThreshold, params.CommitteeConfigv1.SoftCommitteeSize
	}

	if step == types.RoundStep3Certifying {
		return params.CommitteeConfigv1.CertCommitteeThreshold, params.CommitteeConfigv1.CertCommitteeSize
	}

	return params.CommitteeConfigv1.NextCommitteeThreshold, params.CommitteeConfigv1.NextCommitteeSize
}
