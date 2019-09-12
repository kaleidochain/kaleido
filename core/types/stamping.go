package types

import (
	"fmt"

	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/crypto/ed25519"
)

type StampingVote struct {
	Value      common.Hash                    `json:"value" gencodec:"required"`
	ESignValue ed25519.ForwardSecureSignature `json:"eSignature" gencodec:"required"`

	Credential `json:"credential" gencodec:"required"`
}

func (vote *StampingVote) SignBytes() []byte {
	return vote.Value[:]
}

func (vote *StampingVote) String() string {
	return fmt.Sprintf("%s: %s %s",
		"Stamping",
		vote.Value.TerminalString(),
		vote.Credential.String(),
	)
}
