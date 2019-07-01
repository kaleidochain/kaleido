package vm

import (
	"crypto/sha512"
	"encoding/binary"
	"math/big"

	"github.com/kaleidochain/kaleido/crypto/ed25519"

	"github.com/kaleidochain/kaleido/params"

	"github.com/kaleidochain/kaleido/common"
)

type getSeed struct {
}

func (c *getSeed) RequiredGas(input []byte) uint64 {
	return GasExtStep
}

func (c *getSeed) Run(evm *EVM, caller common.Address, input []byte) ([]byte, error) {
	height := big.NewInt(0).SetBytes(input).Uint64()
	seed, _ := evm.GetSeed(height)
	return seed[:], nil
}

type getRand struct {
}

func (c *getRand) RequiredGas(input []byte) uint64 {
	total := uint64(4 + common.HashLength + ed25519.VrfOutputSize256 + len(input))
	return (total+31)/32*params.Sha256PerWordGas + params.Sha256BaseGas
}

func (c *getRand) Run(evm *EVM, caller common.Address, input []byte) ([]byte, error) {
	seed, seq := evm.GetSeed(evm.BlockNumber.Uint64())

	buffer := make([]byte, 4+common.HashLength+ed25519.VrfOutputSize256+len(input))
	binary.BigEndian.PutUint32(buffer[0:], uint32(seq))
	copy(buffer[4:], evm.TxHash.Bytes())
	copy(buffer[4+common.HashLength:], seed[:])
	copy(buffer[4+common.HashLength+ed25519.VrfOutputSize256:], input)

	rand := sha512.Sum512_256(buffer)
	return rand[:], nil
}
