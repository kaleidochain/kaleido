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

package syscon

import (
	"math/big"

	"errors"

	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/contracts"
	"github.com/kaleidochain/kaleido/crypto"
)

//StateDB 从状态db获取状态
type StateDB interface {
	GetState(common.Address, common.Hash) common.Hash
	SetState(addr common.Address, key, value common.Hash)
	HasCode(addr common.Address) bool
}

// Message represents a message sent to a contract.
type Message interface {
	To() *common.Address
	Gas() uint64
	GasPrice() *big.Int
	DataLen() int
}

func IsCallContract(stateDb StateDB, msg Message) bool {
	return msg.DataLen() > 0 && msg.To() != nil && stateDb.HasCode(*msg.To())
}

func GetRealGas(stateDb StateDB, msg Message) uint64 {
	if IsCallContract(stateDb, msg) && msg.Gas() == 0 {
		return GetGasLimit(stateDb, msg.To())
	}

	return msg.Gas()
}

func GetMaxGasPrice(stateDb StateDB, toAddress *common.Address) (price *big.Int) {
	if toAddress == nil {
		return
	}
	price = getMaxGasPrice(stateDb, toAddress)
	if price.Cmp(big.NewInt(0)) > 0 {
		return
	}
	creator := CreatorOrSelf(stateDb, toAddress)
	if creator == *toAddress {
		return
	}
	price = getMaxGasPrice(stateDb, &creator)
	return
}

func GetGasLimit(stateDb StateDB, toAddress *common.Address) (gasLmt uint64) {
	if toAddress == nil {
		return

	}
	gasLmt = gasLimit(stateDb, toAddress)
	if gasLmt > 0 {
		return
	}
	creator := CreatorOrSelf(stateDb, toAddress)
	if creator == *toAddress {
		return
	}
	gasLmt = gasLimit(stateDb, &creator)
	return
}

func GetModel(stateDb StateDB, toAddress *common.Address) uint64 {
	return model(stateDb, toAddress)
}

func CreatorOrSelf(stateDb StateDB, toAddress *common.Address) common.Address {
	creatorkey := common.BytesToHash(crypto.Keccak256(toAddress.Hash().Bytes(), common.HexToHash("0x0").Bytes()))
	creator := stateDb.GetState(contracts.CreatorAddress, creatorkey)
	if common.EmptyHash(creator) {
		return *toAddress
	}
	return common.BytesToAddress(creator.Bytes())
}

func getMaxGasPrice(stateDb StateDB, creatorAddress *common.Address) *big.Int {
	key := common.BytesToHash(crypto.Keccak256(append(creatorAddress.Hash().Bytes(), common.HexToHash("0x0").Bytes()...)))
	return stateDb.GetState(contracts.AuthorityAddress, key).Big()
}
func gasLimit(stateDb StateDB, creatorAddress *common.Address) uint64 {
	key := common.BytesToHash(crypto.Keccak256(append(creatorAddress.Hash().Bytes(), common.HexToHash("0x1").Bytes()...)))
	return stateDb.GetState(contracts.AuthorityAddress, key).Big().Uint64()
}

func model(stateDb StateDB, creatorAddress *common.Address) uint64 {
	key := common.BytesToHash(crypto.Keccak256(append(creatorAddress.Hash().Bytes(), common.HexToHash("0x2").Bytes()...)))
	return stateDb.GetState(contracts.AuthorityAddress, key).Big().Uint64()
}
func isWhite(stateDb StateDB, fromAddress common.Address, creatorAddress *common.Address) bool {
	key1 := crypto.Keccak256(append(creatorAddress.Hash().Bytes(), common.HexToHash("0x3").Bytes()...))
	key2 := common.BytesToHash(crypto.Keccak256(append(fromAddress.Hash().Bytes(), key1...)))
	return stateDb.GetState(contracts.AuthorityAddress, key2).Big().Cmp(big.NewInt(0)) > 0
}

func isBlack(stateDb StateDB, fromAddress common.Address, creatorAddress *common.Address) bool {
	key1 := crypto.Keccak256(append(creatorAddress.Hash().Bytes(), common.HexToHash("0x4").Bytes()...))
	key2 := common.BytesToHash(crypto.Keccak256(append(fromAddress.Hash().Bytes(), key1...)))
	return stateDb.GetState(contracts.AuthorityAddress, key2).Big().Cmp(big.NewInt(0)) > 0
}

func ValidatePayByContract(stateDb StateDB, fromAddress common.Address, msg Message) (err error, maxPrice *big.Int, limit uint64) {
	if !IsCallContract(stateDb, msg) {
		err = errors.New("tx.Gas should not be 0")
		return
	}

	toAddress := msg.To()
	gasPrice := msg.GasPrice()

	creator := CreatorOrSelf(stateDb, toAddress)
	if creator == *toAddress {
		err = errors.New("tx.to should be a contract while payByContract(tx.gasLimit=0)")
		return
	}

	maxPrice = GetMaxGasPrice(stateDb, toAddress)
	limit = GetGasLimit(stateDb, toAddress)

	if maxPrice.Cmp(big.NewInt(0)) == 0 || limit == 0 {
		err = errors.New("payByContract(tx.gasLimit=0) not enabled by creator of Tx.to")
		return
	}

	if gasPrice.Cmp(maxPrice) > 0 {
		err = errors.New("tx.gasPrice exceeded contract's max price while payByContract(tx.gasLimit=0)")
		return
	}
	model := GetModel(stateDb, toAddress)
	var allowed bool
	if model == 0 { //白名单模式
		allowed = isWhite(stateDb, fromAddress, &creator)
	} else { //黑名单模式
		allowed = !isBlack(stateDb, fromAddress, &creator)
	}

	if !allowed {
		err = errors.New("tx.from not allowed by Tx.to while payByContract(tx.gasLimit=0)")
	}
	return
}
