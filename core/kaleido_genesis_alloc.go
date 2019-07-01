package core

import (
	"math/big"

	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/contracts"
)

var (
	kaleidoMainnetGenesisBalance = new(big.Int).Mul(new(big.Int).SetUint64(630720000), common.BigEther)
	kaleidoTestnetGenesisBalance = new(big.Int).Mul(new(big.Int).SetUint64(100000000), common.BigEther)
)

var kaleidoMainnetAllocData = map[common.Address]GenesisAccount{
	common.HexToAddress("0x45Ec182EDC6774c9A2926172F1Fd996e59b58CED"): {
		Balance: kaleidoMainnetGenesisBalance,
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
		Storage: map[common.Hash]common.Hash{
			common.HexToHash("0x06ff3c55f357d4545a14dcc167670bf1dcc8bb45dcd90fa4a085a02a39da3a8a"): common.HexToHash("0x45ec182edc6774c9a2926172f1fd996e59b58ced000000640000000000000001"),
			common.HexToHash("0x06ff3c55f357d4545a14dcc167670bf1dcc8bb45dcd90fa4a085a02a39da3a8b"): common.HexToHash("0xf88a8d844c217531a38d6019ea671652340fe0d899996250bccce13af99933de"),
			common.HexToHash("0x06ff3c55f357d4545a14dcc167670bf1dcc8bb45dcd90fa4a085a02a39da3a8c"): common.HexToHash("0x6e8f4a7c7651766722dd7fb9d7a97cd28678a1cefb12631580a7ffe90a910b8f"),
		},
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

var kaleidoTestnetAllocData = map[common.Address]GenesisAccount{
	common.HexToAddress("0x0e09768B2B2e7aa534243f8bf9AFdC145DdA8EDa"): {
		Balance: kaleidoTestnetGenesisBalance,
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
		Storage: map[common.Hash]common.Hash{
			common.HexToHash("0x82f1831a162a7f3e29811d2195e3e69849199225cd98cdaac26cbc717f24fcaf"): common.HexToHash("0x0e09768b2b2e7aa534243f8bf9afdc145dda8eda000000640000000000000001"),
			common.HexToHash("0x82f1831a162a7f3e29811d2195e3e69849199225cd98cdaac26cbc717f24fcb0"): common.HexToHash("0x5acfc834316080bb6158cc3f2ba4abc2b5dfe14865dfaa979aa6d8937bbe21b7"),
			common.HexToHash("0x82f1831a162a7f3e29811d2195e3e69849199225cd98cdaac26cbc717f24fcb1"): common.HexToHash("0xcc126d7ac641ea94a756ad4ddd4bcf92b683521bc92f7a261c21e116e62a9083"),
		},
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
