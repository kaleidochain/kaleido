package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/big"

	"github.com/kaleidochain/kaleido/contracts"

	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/core"
	"github.com/kaleidochain/kaleido/core/vm"
)

var (
	contract = flag.String("contract", "", "Contract Address")
	creator  = flag.String("creator", "", "Contract's Creator(Address)")
)

var (
	systemContracts = []struct{ contract, creator string }{
		{"0x1000000000000000000000000000000000000001", "0x2000000000000000000000000000000000000001"},
		{"0x1000000000000000000000000000000000000002", "0x2000000000000000000000000000000000000001"},
		{"0x1000000000000000000000000000000000000003", "0x2000000000000000000000000000000000000001"},
		{"0x1000000000000000000000000000000000000004", "0x2000000000000000000000000000000000000001"},
		{"0x1000000000000000000000000000000000000005", "0x2000000000000000000000000000000000000001"},
		{"0x1000000000000000000000000000000000000006", "0x2000000000000000000000000000000000000001"},
		{"0x1000000000000000000000000000000000000007", "0x2000000000000000000000000000000000000001"},
		{"0x1000000000000000000000000000000000000008", "0x2000000000000000000000000000000000000001"},
		{"0x1000000000000000000000000000000000000009", "0x2000000000000000000000000000000000000001"},
	}
)

func main() {
	flag.Parse()

	var account core.GenesisAccount
	account.Balance = big.NewInt(0)
	account.Storage = make(map[common.Hash]common.Hash)

	if len(*contract) > 0 && len(*creator) > 0 {
		contractAddr := common.HexToAddress(*contract)
		slotKey := vm.CreatorSlotKey(&contractAddr)

		account.Storage[slotKey] = common.HexToHash(*creator)
	} else {
		for _, item := range systemContracts {
			contractAddr := common.HexToAddress(item.contract)
			slotKey := vm.CreatorSlotKey(&contractAddr)

			account.Storage[slotKey] = common.HexToHash(item.creator)
		}
	}

	alloc := make(core.GenesisAlloc)
	alloc[contracts.CreatorAddress] = account

	allocStr, err := json.MarshalIndent(alloc, "", "    ")
	if err != nil {
		fmt.Printf("bad alloc string, err:%s\n", err)
		return
	}

	fmt.Printf("alloc key: %s\n", string(allocStr))
}
