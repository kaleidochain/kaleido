package vm

import (
	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/contracts"
	"github.com/kaleidochain/kaleido/crypto"
)

func CreatorSlotKey(contractAddr common.Address) common.Hash {
	return common.BytesToHash(crypto.Keccak256(append(contractAddr.Hash().Bytes(), common.HexToHash("0x0").Bytes()...)))
}

func getCreatorOrSelf(statedb StateDB, addr *common.Address) *common.Address {
	creator := GetContractCreator(statedb, addr)

	var empty common.Address
	if creator == empty {
		return addr
	}

	return &creator
}

//setContractCreator 设置合约创建者
func setContractCreator(statedb StateDB, contractAddr common.Address, creator common.Address) {
	realCreator := getCreatorOrSelf(statedb, &creator)
	key := CreatorSlotKey(contractAddr)
	statedb.SetState(contracts.CreatorAddress, key, realCreator.Hash())
}

//GetContractCreator 取合约创建者
func GetContractCreator(statedb StateDB, contractAddr *common.Address) common.Address {
	key := CreatorSlotKey(*contractAddr)
	h := statedb.GetState(contracts.CreatorAddress, key)
	return common.BytesToAddress(h.Bytes())
}

func deleteFromCreator(statedb StateDB, contractAddr common.Address) {
	setContractCreator(statedb, contractAddr, common.Address{})
}
