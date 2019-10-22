package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/kaleidochain/kaleido/node"

	core2 "github.com/kaleidochain/kaleido/consensus/algorand/core"

	"github.com/kaleidochain/kaleido/contracts"
	"github.com/kaleidochain/kaleido/core"

	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/common/hexutil"
	"github.com/kaleidochain/kaleido/params"
)

var (
	dataDir          = flag.String("datadir", node.DefaultDataDir(), "Data directory for the databases and keystore")
	account          = flag.String("account", "0x0e09768B2B2e7aa534243f8bf9AFdC145DdA8EDa", "The address of genesis account")
	chainID          = flag.Int64("chainid", 1001, "The chainID of new block chain")
	blockYear        = flag.Uint64("blockyear", 1000, "The year in block numbers")
	timeoutTwoLambda = flag.Uint64("twolambda", 1000, "two lambda in milli-seconds")
	intervalSize     = flag.Uint("interval", 1000*1000, "interval size of miner key expiring")
)

func customGenesisBlock(dataDir string, customGenesisAccount common.Address, chainID int64,
	blockYear uint64, timeoutTwoLambda uint64, intervalSize uint32) (*core.Genesis, error) {
	var customGenesisBalance = new(big.Int).Mul(new(big.Int).SetUint64(100000000), common.BigEther)

	var customChainConfig = &params.ChainConfig{
		ChainID: big.NewInt(chainID),

		HomesteadBlock:      big.NewInt(1),
		DAOForkBlock:        nil,
		DAOForkSupport:      false,
		EIP150Block:         big.NewInt(2),
		EIP150Hash:          common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"),
		EIP155Block:         big.NewInt(3),
		EIP158Block:         big.NewInt(3),
		ByzantiumBlock:      big.NewInt(4),
		ConstantinopleBlock: nil,

		BlockYear: blockYear,

		Algorand: &params.AlgorandConfig{
			TimeoutTwoLambda: timeoutTwoLambda,
			TotalWeight:      big.NewInt(10000000000),
			IntervalSize:     intervalSize,
		},
	}

	mkm := core2.NewMinerKeyManager(customChainConfig.Algorand, dataDir)
	mk, err := mkm.GetMinerKey(customGenesisAccount, 0)
	if err != nil {
		return nil, err
	}

	var customAllocData = map[common.Address]core.GenesisAccount{
		customGenesisAccount: {
			Balance: customGenesisBalance,
		},

		contracts.CreatorAddress: {
			Balance: common.Big0,
			Code:    common.FromHex(contracts.CreatorBinRuntime),
			Storage: map[common.Hash]common.Hash{
				common.HexToHash("0x532520e789c75268c7f44ddbbd852a8d4c26633cdd67cb7db15d222b279978da"): common.HexToHash("0x0000000000000000000000002000000000000000000000000000000000000001"),
				common.HexToHash("0x62595a6df946f92490f48861b47d6d970ef92efb6d6f1408453ce1744b0a608b"): common.HexToHash("0x0000000000000000000000002000000000000000000000000000000000000001"),
				common.HexToHash("0x629362be76a9f739e057377744216110b291b004b675e54828bbba825d6e5cb9"): common.HexToHash("0x0000000000000000000000002000000000000000000000000000000000000001"),
				common.HexToHash("0x994bb5a7050cfae00119e5fba64dd81c63fe25678097d07c93f634ca4e137a15"): common.HexToHash("0x0000000000000000000000002000000000000000000000000000000000000001"),
				common.HexToHash("0xb0ff1679fd47264f4031693afe33f0db97c9922f0c27b95ff96eee0f357b325e"): common.HexToHash("0x0000000000000000000000002000000000000000000000000000000000000001"),
				common.HexToHash("0xb850de402b07a44eafd99ce6da7338338c422c8bb4fa49bb2641292468db191d"): common.HexToHash("0x0000000000000000000000002000000000000000000000000000000000000001"),
				common.HexToHash("0xe6f18b3f6d2cdeb50fb82c61f7a7a249abf7b534575880ddcfde84bba07ce81d"): common.HexToHash("0x0000000000000000000000002000000000000000000000000000000000000001"),
				common.HexToHash("0xf83f5a1b22d116430e08ff8febbb7c335eee7f0e949b3b06b1e55817e7bba435"): common.HexToHash("0x0000000000000000000000002000000000000000000000000000000000000001"),
				common.HexToHash("0xfb750de6f7d0583f749efc558ce6626b24fed04efd7219dc3f4294c408699e8c"): common.HexToHash("0x0000000000000000000000002000000000000000000000000000000000000001"),
			},
		},
		contracts.MinerAddress: {
			Balance: common.Big0,
			Code:    common.FromHex(contracts.MinerBinRuntime),
			Storage: makeMinerKeyStorage(mk.ToVerifier()),
		},
		contracts.AuthorityAddress: {
			Balance: common.Big0,
			Code:    common.FromHex(contracts.AuthorityBinRuntime),
		},
		contracts.DelegationAddress: {
			Balance: common.Big0,
			Code:    common.FromHex(contracts.DelegationBinRuntime),
		},
	}

	genesis := &core.Genesis{
		Config:               customChainConfig,
		Nonce:                params.ConsensusVersion,
		Timestamp:            uint64(time.Now().Unix()),
		ExtraData:            hexutil.MustDecode("0x6b616c6569646f"),
		GasLimit:             63000000,
		Difficulty:           big.NewInt(0),
		TotalBalanceOfMiners: customGenesisBalance,
		Seed:                 hexutil.MustDecode("0x5affa368fc5b2488445a5bdce3c32f6a655190eb144199c43b76081c93854ee5"),
		Alloc:                customAllocData,
	}

	return genesis, nil
}

func main() {
	flag.Parse()

	genesisAccount := common.HexToAddress(*account)
	genesis, err := customGenesisBlock(filepath.Join(*dataDir, "kalgo"), genesisAccount, *chainID, *blockYear, *timeoutTwoLambda, uint32(*intervalSize))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "make genesis error: %s\n", err)
		_, _ = fmt.Fprintf(os.Stderr, "Hint: You must make a minerkey before making genesis.\n")

		return
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "    ")
	err = encoder.Encode(genesis)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "marshal genesis error: %s\n", err)
		return
	}
}

func makeMinerKeyStorage(mv *core2.MinerVerifier) map[common.Hash]common.Hash {
	kvs := mv.KeyValueStorage()
	storage := make(map[common.Hash]common.Hash)
	storage[kvs[0]] = kvs[1]
	storage[kvs[2]] = kvs[3]
	storage[kvs[4]] = kvs[5]
	return storage
}
