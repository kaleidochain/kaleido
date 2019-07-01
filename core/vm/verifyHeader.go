package vm

import (
	"bytes"
	"encoding/binary"
	"math/big"

	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/consensus/ethash"
	"github.com/ethereum/go-ethereum/rlp"
	"golang.org/x/crypto/sha3"
)

var dataCache map[uint64][]uint32

const (
	epochLength = 30000 // Blocks per epoch
)

func init() {
	dataCache = make(map[uint64][]uint32)
}

var (
	expDiffPeriod = big.NewInt(100000)
	//big1      = big.NewInt(1)
	big2       = big.NewInt(2)
	big9       = big.NewInt(9)
	big10      = big.NewInt(10)
	bigMinus99 = big.NewInt(-99)
	big2999999 = big.NewInt(2999999)

	maxUint256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), big.NewInt(0))
)

type header struct {
	ParentHash  common.Hash ""
	UncleHash   common.Hash ""
	Coinbase    common.Address
	Root        common.Hash
	TxHash      common.Hash
	ReceiptHash common.Hash
	Bloom       []byte   "256"
	Difficulty  *big.Int ""
	Number      *big.Int ""
	GasLimit    uint64
	GasUsed     uint64
	Time        *big.Int ""
	Extra       []byte
	MixDigest   common.Hash
	Nonce       []byte "8"
}

func (h *header) hashNoNonce() (hash common.Hash) {
	hasher := sha3.NewLegacyKeccak256()
	rlp.Encode(hasher, []interface{}{
		h.ParentHash,
		h.UncleHash,
		h.Coinbase,
		h.Root,
		h.TxHash,
		h.ReceiptHash,
		h.Bloom,
		h.Difficulty,
		h.Number,
		h.GasLimit,
		h.GasUsed,
		h.Time,
		h.Extra,
	})
	hasher.Sum(hash[:0])
	return hash
}

func verify(data []byte) bool {
	var head header
	rlp.DecodeBytes(data, &head)
	blockNumber := ethash.DatasetSize(head.Number.Uint64())
	epoch := head.Number.Uint64() / epochLength
	size := ethash.CacheSize(epoch*epochLength + 1)
	seed := ethash.SeedHash(epoch*epochLength + 1)
	var cache []uint32
	if _, ok := dataCache[epoch]; ok {
		cache = dataCache[epoch]
		if _, ok := dataCache[epoch-1]; ok {
			delete(dataCache, epoch-1)
		}
	} else {
		cache = make([]uint32, size/4)
		ethash.GenerateCache(cache, epoch, seed)
		dataCache[epoch] = cache
	}

	digest, result := ethash.HashimotoLight(blockNumber, cache, head.hashNoNonce().Bytes(), binary.BigEndian.Uint64(head.Nonce[:]))
	if !bytes.Equal(head.MixDigest[:], digest) {
		return false
	}

	target := new(big.Int).Div(maxUint256, head.Difficulty)
	if new(big.Int).SetBytes(result).Cmp(target) > 0 {
		return false
	}
	return true
}
