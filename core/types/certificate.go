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

package types

import (
	"bytes"
	"crypto/sha512"
	"fmt"
	"unsafe"

	"github.com/kaleidochain/kaleido/crypto/ed25519"

	"github.com/kaleidochain/kaleido/common/math"

	"github.com/kaleidochain/kaleido/common"
)

const (
	BadRound = uint32(0)
	BadStep  = uint32(0)
	MaxStep  = math.MaxUint32
)

// Round Step Value
const (
	RoundStep1Proposal        = 0x01
	RoundStep2Filtering       = 0x02
	RoundStep3Certifying      = 0x03
	RoundStep4FirstFinishing  = 0x04
	RoundStep5SecondFinishing = 0x05
)

// Vote Type Value
const (
	VoteTypeSoft = uint32(0x01)
	VoteTypeCert = uint32(0x02)
	VoteTypeNext = uint32(0x03)
)

// VoteTypeString returns a string
func VoteTypeString(voteType uint32) string {
	switch voteType {
	case VoteTypeSoft:
		return "SoftVote"
	case VoteTypeCert:
		return "CertVote"
	case VoteTypeNext:
		return "NextVote"
	default:
		return "BadVote"
	}
}

// VoteTypeOfStep returns vote type according to Step
func VoteTypeOfStep(step uint32) uint32 {
	if step == RoundStep2Filtering {
		return VoteTypeSoft
	} else if step == RoundStep3Certifying {
		return VoteTypeCert
	} else if step >= RoundStep4FirstFinishing {
		return VoteTypeNext
	}
	return BadStep
}

func LessThanByProof(a, b *ed25519.VrfProof) bool {
	hashA, okA := ed25519.VrfProofToHash256(a)
	hashB, okB := ed25519.VrfProofToHash256(b)
	if !okA || !okB {
		panic(fmt.Sprintf("bad vrf proof: %x, %x", a, b))
	}

	randA := sha512.Sum512_256(hashA[:])
	randB := sha512.Sum512_256(hashB[:])

	return bytes.Compare(randA[:], randB[:]) < 0
}

func EqualToByProof(a, b *ed25519.VrfProof) bool {
	hashA, okA := ed25519.VrfProofToHash256(a)
	hashB, okB := ed25519.VrfProofToHash256(b)
	if !okA || !okB {
		panic(fmt.Sprintf("bad vrf proof: %x, %x", a, b))
	}

	return hashA == hashB
}

// For minimal storage
type CredentialStorage struct {
	Address common.Address   `json:"address" gencodec:"required"`
	Proof   ed25519.VrfProof `json:"proof" gencodec:"required"`
}

func (c CredentialStorage) EqualTo(other *CredentialStorage) bool {
	return EqualToByProof(&c.Proof, &other.Proof)
}

func (c *CredentialStorage) LessThan(other *CredentialStorage) bool {
	return LessThanByProof(&c.Proof, &other.Proof)
}

func (c *CredentialStorage) String() string {
	return fmt.Sprintf("%s %x", c.Address, c.Proof[:3])
}

// ------

type ProposalStorage struct {
	ESignValue ed25519.ForwardSecureSignature `json:"eSignValue" gencodec:"required"`
	Round      uint32                         `json:"round" gencodec:"required"`
	Credential CredentialStorage              `json:"Credential" gencodec:"required"`

	SeedProof ed25519.VrfProof `json:"seedProof" gencodec:"required"`
}

func (p *ProposalStorage) String() string {
	return fmt.Sprintf("%x %d %s %x", p.ESignValue.Sig[:3], p.Round, p.Credential.String(), p.SeedProof[:3])
}

// ------

// For minimal storage
type CertVoteStorage struct {
	ESignValue ed25519.ForwardSecureSignature `json:"eSignValue" gencodec:"required"`
	Credential CredentialStorage              `json:"Credential" gencodec:"required"`
}

func (c *CertVoteStorage) String() string {
	return fmt.Sprintf("%x %s", c.ESignValue.Sig[:3], c.Credential.String())
}

// -----

type Certificate struct {
	Height      uint64             `json:"height" gencodec:"required"`
	Round       uint32             `json:"round" gencodec:"required"`
	Value       common.Hash        `json:"value" gencodec:"required"`
	Proposal    ProposalStorage    `json:"proposal" gencodec:"required"`
	CertVoteSet []*CertVoteStorage `json:"certVoteSet" gencodec:"required"`
	TrieProof   NodeList           `json:"trieProof" gencodec:"required"`
}

func (c *Certificate) Proposer() common.Address {
	return c.Proposal.Credential.Address
}

func (c *Certificate) SetProposer(addr common.Address) {
	c.Proposal.Credential.Address = addr
}

func (c *Certificate) SeedProof() ed25519.VrfProof {
	return c.Proposal.SeedProof
}

func (c *Certificate) SeedProofBytes() []byte {
	return c.Proposal.SeedProof[:]
}

func (c *Certificate) SetSeedProof(proof []byte) {
	copy(c.Proposal.SeedProof[:], proof[:])
}

func (c *Certificate) SignBytes() []byte {
	return c.Value[:]
}

func (c *Certificate) Copy() *Certificate {
	cpy := new(Certificate)
	*cpy = *c

	if c.CertVoteSet != nil {
		cpy.CertVoteSet = make([]*CertVoteStorage, len(c.CertVoteSet))
		for i := 0; i < len(c.CertVoteSet); i++ {
			cpy.CertVoteSet[i] = &CertVoteStorage{}
			*cpy.CertVoteSet[i] = *c.CertVoteSet[i]
		}
	}
	if c.TrieProof != nil {
		cpy.TrieProof = make(NodeList, len(c.TrieProof))
		for i := 0; i < len(c.TrieProof); i++ {
			cpy.TrieProof[i] = make([]byte, len(c.TrieProof[i]))
			copy(cpy.TrieProof[i], c.TrieProof[i])
		}
	}

	return cpy
}

func (c *Certificate) Size() common.StorageSize {
	size := common.StorageSize(unsafe.Sizeof(*c))

	if len(c.CertVoteSet) > 0 {
		size += common.StorageSize(len(c.CertVoteSet) * int(unsafe.Sizeof(c.CertVoteSet[0])))
	}
	if len(c.TrieProof) > 0 {
		size += common.StorageSize(len(c.TrieProof) * len(c.TrieProof[0]))
	}

	return size
}

type CertVoteStorageSlice []*CertVoteStorage

func (s CertVoteStorageSlice) Len() int      { return len(s) }
func (s CertVoteStorageSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s CertVoteStorageSlice) Less(i, j int) bool {
	return bytes.Compare(s[i].Credential.Address[:], s[j].Credential.Address[:]) < 0
}
