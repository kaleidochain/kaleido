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
	"time"

	"github.com/kaleidochain/kaleido/params"

	"github.com/ethereum/go-ethereum/log"
	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/core"
	"github.com/kaleidochain/kaleido/core/vm"

	"github.com/kaleidochain/kaleido/core/types"
)

func (ctx *Context) verifyBlockSync(block *types.Block) error {
	key := block.Hash().Str()
	ctx.mutexVerifiedBlockState.RLock()
	result, hasResult := ctx.verifiedBlockState[key]
	ctx.mutexVerifiedBlockState.RUnlock()
	if hasResult {
		return result.err
	}

	resultCh := ctx.verifyBlockAsync(block)
	result = <-resultCh

	ctx.saveVerifiedBlockState(block.Hash(), result)

	if result.err != nil {
		log.Warn("VerifyBlock fail, drop it", "err", result.err, "Block", block)
		ctx.dropProposalBlock(block)
	}

	return result.err
}

func (ctx *Context) verifyBlockAsync(block *types.Block) chan *blockState {
	key := block.Hash().Str()

	ctx.mutexVerifiedBlockState.RLock()
	result, hasResult := ctx.verifiedBlockState[key]
	resultCh, hasRunning := ctx.verifingBlockState[key]
	ctx.mutexVerifiedBlockState.RUnlock()

	if !hasResult && !hasRunning {
		ctx.mutexVerifiedBlockState.Lock()

		// double check
		result, hasResult = ctx.verifiedBlockState[key]
		resultCh, hasRunning = ctx.verifingBlockState[key]
		if !hasResult && !hasRunning {
			resultCh = make(chan *blockState, 1)
			ctx.verifingBlockState[key] = resultCh
		}
		ctx.mutexVerifiedBlockState.Unlock()
	}

	if hasResult {
		resultCh = make(chan *blockState, 1)
		resultCh <- result
		return resultCh
	}

	if hasRunning {
		return resultCh
	}

	go verifyBlockWorker(ctx.eth.BlockChain(), ctx.config, block, resultCh)
	return resultCh
}

func verifyBlockWorker(bc *core.BlockChain, config *params.ChainConfig, block *types.Block, resultCh chan *blockState) {
	result := &blockState{}

	timeStart := time.Now()
	timeProcessStart, timeValidateStateStart := timeStart, timeStart

	for {
		err := bc.Validator().ValidateBody(block)
		if err == core.ErrKnownBlock {
			log.Warn("verifyBlock:block already known, continue")
		} else if err != nil {
			log.Error("verifyBlock:Failed to ValidateBody", "err", err)
			result.err = err
			break
		}

		parent := bc.GetBlock(block.ParentHash(), block.NumberU64()-1)
		statedb, err := bc.StateAt(parent.Root())
		if err != nil {
			log.Error("verifyBlock:Failed to create verifyBlock context", "err", err)
			result.err = err
			break
		}

		statedb.EnableMinerTracker(config.Algorand, block.NumberU64(), parent.TotalBalanceOfMiners())

		timeProcessStart = time.Now()
		timeValidateStateStart = timeProcessStart
		receipts, _, usedGas, mortgageContractCounter, err := bc.Processor().Process(block, statedb, vm.Config{})
		if err != nil {
			log.Error("verifyBlock:Failed to process tx", "err", err)
			result.err = err
			break
		}

		timeValidateStateStart = time.Now()
		// Validate the state using the default validator
		err = bc.Validator().ValidateState(block, parent, statedb, receipts, usedGas)
		if err != nil {
			log.Error("verifyBlock: Failed to ValidateState", "err", err)
			result.err = err
			break
		}

		result.statedb = statedb
		result.receipts = receipts
		result.counter = mortgageContractCounter
		break
	}

	usedTotal := time.Since(timeStart)
	usedValidateBody := timeProcessStart.Sub(timeStart)
	usedProcessor := timeValidateStateStart.Sub(timeProcessStart)
	usedValidateState := time.Since(timeValidateStateStart)

	log.Debug("VerifyBlock Done", "err", result.err, "number", block.NumberU64(), "hash", block.Hash(),
		"txs", len(block.Transactions()), "gas", block.GasUsed(), "stateRoot", block.Root(),
		"elapsed", common.PrettyDuration(usedTotal),
		"validateBodyUsed", common.PrettyDuration(usedValidateBody),
		"processorUsed", common.PrettyDuration(usedProcessor),
		"validateStateUsed", common.PrettyDuration(usedValidateState))

	resultCh <- result
	return
}
