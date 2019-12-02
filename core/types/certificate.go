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
	"encoding/binary"
	"fmt"
	"unsafe"

	"github.com/kaleidochain/kaleido/params"

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

func GetCommitteeNumber(height uint64, step uint32) (uint64, uint64) {
	const proposerThreshold = 1
	if step == RoundStep1Proposal {
		return proposerThreshold, params.CommitteeConfigv1.NumProposer
	}

	if step == RoundStep2Filtering {
		return params.CommitteeConfigv1.SoftCommitteeThreshold, params.CommitteeConfigv1.SoftCommitteeSize
	}

	if step == RoundStep3Certifying {
		return params.CommitteeConfigv1.CertCommitteeThreshold, params.CommitteeConfigv1.CertCommitteeSize
	}

	return params.CommitteeConfigv1.NextCommitteeThreshold, params.CommitteeConfigv1.NextCommitteeSize
}

func LessThanByProof(proofA, proofB *ed25519.VrfProof, jA, jB uint64) bool {
	return LessThanByProofInt(proofA, proofB, jA, jB) < 0
}

func LessThanByProofInt(proofA, proofB *ed25519.VrfProof, jA, jB uint64) int {
	if jA == 0 {
		return 1
	} else if jB == 0 {
		return -1
	}

	hashA, okA := ed25519.VrfProofToHash256(proofA)
	hashB, okB := ed25519.VrfProofToHash256(proofB)
	if !okA || !okB {
		panic(fmt.Sprintf("bad vrf proof: %x, %x", proofA, proofB))
	}

	minA := minRandHash(hashA, jA)
	minB := minRandHash(hashB, jB)

	return bytes.Compare(minA, minB)
}

func minRandHash(hash ed25519.VrfOutput256, j uint64) []byte {
	min := sha512.Sum512_256(hash[:])
	for i := uint64(2); i <= j; i++ {
		b := new(bytes.Buffer)
		b.Write(hash[:])
		binary.Write(b, binary.BigEndian, i)
		hash512 := sha512.Sum512_256(b.Bytes())

		if bytes.Compare(hash512[:], min[:]) < 0 {
			min = hash512
		}
	}
	return min[:]
}

type Credential struct {
	Address common.Address   `json:"address" gencodec:"required"`
	Height  uint64           `json:"height" gencodec:"required"`
	Round   uint32           `json:"round" gencodec:"required"`
	Step    uint32           `json:"step" gencodec:"required"`
	Proof   ed25519.VrfProof `json:"proof" gencodec:"required"`

	// cached
	Weight uint64 `json:"weight" rlp:"-"`
}

func NewCredentialFromCredentialStorage(storage *CredentialStorage, height uint64, round uint32, step uint32) Credential {
	return Credential{
		Address: storage.Address,
		Height:  height,
		Round:   round,
		Step:    step,
		Proof:   storage.Proof,
	}
}

func (c *Credential) LessThan(other *Credential) bool {
	return LessThanByProof(&c.Proof, &other.Proof)
}

func (c *Credential) String() string {
	return fmt.Sprintf("%d/%d/%d(%d) by %s, %x",
		c.Height, c.Round, c.Step, c.Weight, c.Address.String(), c.Proof[:3])
}

// For minimal storage
type CredentialStorage struct {
	Address common.Address   `json:"address" gencodec:"required"`
	Proof   ed25519.VrfProof `json:"proof" gencodec:"required"`
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

func (c *Certificate) Proof() ed25519.VrfProof {
	return c.Proposal.Credential.Proof
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
