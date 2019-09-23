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

package main

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/kaleidochain/kaleido/accounts/abi/bind"

	"github.com/kaleidochain/kaleido/contracts"
	"github.com/kaleidochain/kaleido/contracts/contractgo"

	"github.com/kaleidochain/kaleido/ethclient"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/ethereum/go-ethereum/log"

	"github.com/kaleidochain/kaleido/accounts/keystore"

	"github.com/kaleidochain/kaleido/cmd/utils"
	"github.com/kaleidochain/kaleido/common"
	algorandcore "github.com/kaleidochain/kaleido/consensus/algorand/core"
	"github.com/kaleidochain/kaleido/consensus/ethash"
	"github.com/kaleidochain/kaleido/eth"
	"github.com/kaleidochain/kaleido/params"
	"gopkg.in/urfave/cli.v1"
)

var (
	makecacheCommand = cli.Command{
		Action:    utils.MigrateFlags(makecache),
		Name:      "makecache",
		Usage:     "Generate ethash verification cache (for testing)",
		ArgsUsage: "<blockNum> <outputDir>",
		Category:  "MISCELLANEOUS COMMANDS",
		Description: `
The makecache command generates an ethash cache in <outputDir>.

This command exists to support the system testing project.
Regular users do not need to execute it.
`,
	}
	makedagCommand = cli.Command{
		Action:    utils.MigrateFlags(makedag),
		Name:      "makedag",
		Usage:     "Generate ethash mining DAG (for testing)",
		ArgsUsage: "<blockNum> <outputDir>",
		Category:  "MISCELLANEOUS COMMANDS",
		Description: `
The makedag command generates an ethash DAG in <outputDir>.

This command exists to support the system testing project.
Regular users do not need to execute it.
`,
	}
	versionCommand = cli.Command{
		Action:    utils.MigrateFlags(version),
		Name:      "version",
		Usage:     "Print version numbers",
		ArgsUsage: " ",
		Category:  "MISCELLANEOUS COMMANDS",
		Description: `
The output of this command is supposed to be machine-readable.
`,
	}
	licenseCommand = cli.Command{
		Action:    utils.MigrateFlags(license),
		Name:      "license",
		Usage:     "Display license information",
		ArgsUsage: " ",
		Category:  "MISCELLANEOUS COMMANDS",
	}

	makeMinerKeyCommand = cli.Command{
		Action:    utils.MigrateFlags(makeMinerKey),
		Name:      "makeminerkey",
		Usage:     "Generates a miner key for mining",
		ArgsUsage: " ",
		Flags: []cli.Flag{
			utils.DataDirFlag,
			utils.MinerStakeOwnerFlag,
			utils.MinerKeyCoinbaseFlag,
			utils.MinerKeyStartFlag,
			utils.MinerKeyLifespanFlag,
		},
		Category: "MINER COMMANDS",
		Description: `
The makeMinerKey command generates a miner key for mining.
If the key already exists, just return it.
`,
	}

	dumpVoteCommand = cli.Command{
		Action:    utils.MigrateFlags(dumpVote),
		Name:      "dumpvote",
		Usage:     "Print height vote messages",
		ArgsUsage: " ",
		Flags: []cli.Flag{
			utils.DataDirFlag,
			utils.VoteMsgHeightFlag,
		},
		Category:    "DEBUG COMMANDS",
		Description: "The dumpVote command prints height vote message for algorand-consensus.",
	}
	minerkeyReaderCommand = cli.Command{
		Action:    utils.MigrateFlags(minerKeyReader),
		Name:      "minerkeyreader",
		Usage:     "Print minerkey file",
		ArgsUsage: " ",
		Flags: []cli.Flag{
			utils.DataDirFlag,
			utils.MinerStakeOwnerFlag,
			utils.MinerKeyStartFlag,
		},
		Category:    "DEBUG COMMANDS",
		Description: "The minerkeyreader command prints 'minerkey' in order to check minerkey file.",
	}
	minerDbReaderCommand = cli.Command{
		Action:    utils.MigrateFlags(minerDbReader),
		Name:      "minerdbreader",
		Usage:     "Print minerdb.get",
		ArgsUsage: " ",
		Flags: []cli.Flag{
			utils.DataDirFlag,
			utils.MinerStakeOwnerFlag,
			utils.MinerKeyStartFlag,
			utils.RPCEndpointFlag,
		},
		Category:    "DEBUG COMMANDS",
		Description: "The minerdbreader command prints MinerDB.get.",
	}
)

// makecache generates an ethash verification cache into the provided folder.
func makecache(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) != 2 {
		utils.Fatalf(`Usage: kalgo makecache <block number> <outputdir>`)
	}
	block, err := strconv.ParseUint(args[0], 0, 64)
	if err != nil {
		utils.Fatalf("Invalid block number: %v", err)
	}
	ethash.MakeCache(block, args[1])

	return nil
}

// makedag generates an ethash mining DAG into the provided folder.
func makedag(ctx *cli.Context) error {
	args := ctx.Args()
	if len(args) != 2 {
		utils.Fatalf(`Usage: kalgo makedag <block number> <outputdir>`)
	}
	block, err := strconv.ParseUint(args[0], 0, 64)
	if err != nil {
		utils.Fatalf("Invalid block number: %v", err)
	}
	ethash.MakeDataset(block, args[1])

	return nil
}

func version(ctx *cli.Context) error {
	fmt.Println(strings.Title(clientIdentifier))
	fmt.Println("Version:", params.KalgoVersion)
	if gitCommit != "" {
		fmt.Println("Git Commit:", gitCommit)
	}
	fmt.Println("Geth Version:", params.Version)
	fmt.Println("Architecture:", runtime.GOARCH)
	fmt.Println("Protocol Versions:", eth.ProtocolVersions)
	fmt.Println("Network Id:", eth.DefaultConfig.NetworkId)
	fmt.Println("Go Version:", runtime.Version())
	fmt.Println("Operating System:", runtime.GOOS)
	fmt.Printf("GOPATH=%s\n", os.Getenv("GOPATH"))
	fmt.Printf("GOROOT=%s\n", runtime.GOROOT())
	return nil
}

func license(_ *cli.Context) error {
	fmt.Println(`Kalgo is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

Kalgo is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with kalgo. If not, see <http://www.gnu.org/licenses/>.`)
	return nil
}

func makeMinerKey(ctx *cli.Context) error {
	start := ctx.GlobalUint64(utils.MinerKeyStartFlag.Name)
	if start == 0 {
		utils.Fatalf("start must be greater than 0")
	}

	config := params.KaleidoMainnetChainConfig.Algorand
	if ctx.GlobalBool(utils.TestnetFlag.Name) {
		config = params.KaleidoTestnetChainConfig.Algorand
	} else if ctx.GlobalBool(utils.DeveloperFlag.Name) {
		utils.Fatalf("not support devnet")
		// todo: support devnet
	}

	stack := makeFullNode(ctx)

	var miner, coinbase common.Address

	var minerStr string
	if ctx.GlobalIsSet(utils.MinerStakeOwnerFlag.Name) {
		minerStr = ctx.GlobalString(utils.MinerStakeOwnerFlag.Name)
	}
	if minerStr != "" {
		ks := stack.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
		account, err := utils.MakeAddress(ks, minerStr)
		if err != nil {
			utils.Fatalf("Invalid miner etherbase: %v", err)
		}

		miner = account.Address
	}

	if ctx.GlobalIsSet(utils.MinerKeyCoinbaseFlag.Name) {
		str := ctx.GlobalString(utils.MinerKeyCoinbaseFlag.Name)
		if !common.IsHexAddress(str) {
			utils.Fatalf("%s address invalid: %s", utils.MinerKeyCoinbaseFlag.Name, str)
		}
		coinbase = common.HexToAddress(str)
	} else { // default is miner
		coinbase = miner
	}

	lifespan := ctx.GlobalUint(utils.MinerKeyLifespanFlag.Name)

	dataDir := stack.ResolvePath("")
	mkm := algorandcore.NewMinerKeyManager(config, dataDir)

	var mv *algorandcore.MinerVerifier
	if mk, err := mkm.GetMinerKey(miner, start); err == nil {
		fmt.Println("MinerKey already exists!")
		mv = mk.ToVerifier()
		log.Debug("mv", "mv", mv)
	} else {
		mv, _, err = mkm.Generate(miner, coinbase, start, uint32(lifespan))

		if err != nil {
			utils.Fatalf("failed to make miner key: %v", err)
		}
	}

	details := strings.ReplaceAll(mv.String(), ", ", "\n\t")
	fmt.Printf("MinerKey: %s\nDetails:\n\t%s\n", mv.AbiString(), details)

	if start == 1 {
		genesisStorage := strings.ReplaceAll(mv.GenesisString(), ", ", "\n\t")
		fmt.Printf("GenesisStorage:\n\t%s\n", genesisStorage)
	}

	return nil
}

func dumpVote(ctx *cli.Context) error {
	height := ctx.GlobalUint64(utils.VoteMsgHeightFlag.Name)
	if height == 0 {
		utils.Fatalf("Invalid height, height SHOULD BE greater than 0")
	}

	dataDir := utils.MakeDataDir(ctx)
	recoverFile := fmt.Sprintf("%s/kalgo/msg/%d", dataDir, height)
	journal := algorandcore.NewVoteJournal(recoverFile)
	if err := journal.OpenFile(); err != nil {
		utils.Fatalf("Failed to open vote message file for height(%d): %v", err, height)
	}

	err := journal.Read(algorandcore.PrintVoteJournalEntry)

	if err != nil {
		utils.Fatalf("Failed to read vote message file for height(%d): %v", err, height)
	}
	journal.CloseFile()

	return nil
}

func minerKeyReader(ctx *cli.Context) error {
	stakeOwner := ctx.GlobalString(utils.MinerStakeOwnerFlag.Name)
	if len(stakeOwner) == 0 {
		utils.Fatalf("Invalid StakeOwner")
	}
	start := ctx.GlobalUint64(utils.MinerKeyStartFlag.Name)
	if start == 0 {
		utils.Fatalf("start must be greater than 0")
	}

	config := params.KaleidoMainnetChainConfig.Algorand
	if ctx.GlobalBool(utils.TestnetFlag.Name) {
		config = params.KaleidoTestnetChainConfig.Algorand
	} else if ctx.GlobalBool(utils.DeveloperFlag.Name) {
		utils.Fatalf("not support devnet")
		// todo: support devnet
	}

	sn := config.GetIntervalSn(start)
	begin, end := config.GetInterval(sn)
	filename := fmt.Sprintf("%s-%d-%d.bin", stakeOwner, begin, end)

	dataDir := utils.MakeDataDir(ctx)
	path := fmt.Sprintf("%s/kalgo/minerkeys/%s", dataDir, filename)

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(f)

	stream := rlp.NewStream(f, algorandcore.MaxMinerKeySize)
	mk := algorandcore.NewEmptyMinerKey(config)
	err = mk.DecodeRLP(stream)
	if err != nil {
		return err
	}

	mv := mk.ToVerifier()
	details := strings.ReplaceAll(mv.String(), ", ", "\n\t")
	fmt.Printf("MinerKey: %s\nDetails:\n\t%s\n", mv.AbiString(), details)

	return nil
}

func minerDbReader(ctx *cli.Context) error {
	stakeOwner := ctx.GlobalString(utils.MinerStakeOwnerFlag.Name)
	if len(stakeOwner) == 0 {
		utils.Fatalf("Invalid StakeOwner")
	}
	start := ctx.GlobalUint64(utils.MinerKeyStartFlag.Name)
	if start == 0 {
		utils.Fatalf("start must be greater than 0")
	}
	rpc := ctx.GlobalString(utils.RPCEndpointFlag.Name)
	if len(rpc) == 0 {
		utils.Fatalf("Invalid rpc")
	}

	config := params.KaleidoMainnetChainConfig.Algorand
	if ctx.GlobalBool(utils.TestnetFlag.Name) {
		config = params.KaleidoTestnetChainConfig.Algorand
	} else if ctx.GlobalBool(utils.DeveloperFlag.Name) {
		utils.Fatalf("not support devnet")
		// todo: support devnet
	}

	sn := config.GetIntervalSn(start)
	begin, end := config.GetInterval(sn)

	client, err := ethclient.Dial(rpc)
	if err != nil {
		utils.Fatalf("cannt connect rpc:%s", rpc)
	}

	miner, err := contractgo.NewMiner(contracts.MinerAddress, client)
	if err != nil {
		utils.Fatalf("NewMiner error, err:%s", err)
	}

	stakeOwnerAddress := common.HexToAddress(stakeOwner)
	opts := &bind.CallOpts{
		Pending: false,
		From:    stakeOwnerAddress,
		Context: context.Background(),
	}
	startUint, lifespan, coinbase, vrfVerifier, voteVerifier, err := miner.Get(opts, new(big.Int).SetUint64(begin), stakeOwnerAddress)
	if err != nil {
		utils.Fatalf("miner.get error, err:%s", err)
	}
	details := fmt.Sprintf("miner = %s, coinbase = %s, start = %d, end = %d, lifespan = %d, vrfVerifier = 0x%x, voteVerfier = 0x%x",
		stakeOwner, coinbase.String(), startUint, end, lifespan, vrfVerifier, voteVerifier[:])

	details = strings.ReplaceAll(details, ", ", "\n\t")
	fmt.Printf("MinerDB:\n\t%s\n", details)

	return nil
}
