package core

import (
	"bytes"
	"math/big"

	"github.com/kaleidochain/kaleido/common/hexutil"

	"github.com/kaleidochain/kaleido/core/types"

	"github.com/ethereum/go-ethereum/log"
	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/core/state"
	"github.com/kaleidochain/kaleido/crypto/ed25519"
	"github.com/kaleidochain/kaleido/params"
	"github.com/kaleidochain/kaleido/sortition"
)

func GetSortitionWeight(config *params.AlgorandConfig, bc *BlockChain, height uint64, proof ed25519.VrfProof, miner common.Address) (j uint64) {
	parentHeader := bc.GetHeaderByNumber(height - 1)
	stateDb, err := bc.StateAt(parentHeader.Root)
	if err != nil {
		log.Error("Failed to get stateDb", "err", err)
		return
	}

	j = getSortitionWeight(config, stateDb, height, proof, miner, parentHeader.TotalBalanceOfMiners)

	return
}

func GetSortitionWeightByState(config *params.AlgorandConfig, parentStateDb *state.StateDB, parent, header *types.Header) (j uint64) {
	return getSortitionWeight(config, parentStateDb, header.Number.Uint64(), header.Proof(), header.Proposer(), parent.TotalBalanceOfMiners)
}

func getSortitionWeight(config *params.AlgorandConfig, parentStateDb *state.StateDB, height uint64, proof ed25519.VrfProof, miner common.Address, totalBalanceOfMiners *big.Int) (j uint64) {
	hash, ok := ed25519.VrfProofToHash256(&proof)
	if !ok {
		return 0
	}

	ownWeight, totalWeight := GetWeight(config, miner, parentStateDb, totalBalanceOfMiners, hash)

	threshold, size := types.GetCommitteeNumber(height, types.RoundStep1Proposal)
	j = sortition.Choose(hash, ownWeight, threshold, size, totalWeight)

	return
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
