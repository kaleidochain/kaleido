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

package core

import (
	"fmt"

	"github.com/kaleidochain/kaleido/crypto/ed25519"

	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/core/types"
)

// protocol message codes
const (
	HandshakeMsg         = 0x00
	StatusMsg            = 0x01
	ProposalLeaderMsg    = 0x02
	ProposalBlockMsg     = 0x03
	VoteMsg              = 0x04
	HasVoteMsg           = 0x05
	TimeoutMsg           = 0x06
	HasProposalLeaderMsg = 0x07
	HasProposalBlockMsg  = 0x08
)

var CodeToString = map[uint64]string{
	HandshakeMsg:         "HandshakeMsg",
	StatusMsg:            "StatusMsg",
	ProposalLeaderMsg:    "ProposalLeaderMsg",
	ProposalBlockMsg:     "ProposalBlockMsg",
	VoteMsg:              "VoteMsg",
	HasVoteMsg:           "HasVoteMsg",
	TimeoutMsg:           "TimeoutMsg",
	HasProposalLeaderMsg: "HasProposalLeaderMsg",
	HasProposalBlockMsg:  "HasProposalBlockMsg",
}

type HandshakeData struct {
	Version uint32
	Height  uint64
	Round   uint32
}

type StatusData struct {
	Height uint64
	Round  uint32
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

func NewCredentialFromCredentialStorage(storage *types.CredentialStorage, height uint64, round uint32, step uint32) Credential {
	return Credential{
		Address: storage.Address,
		Height:  height,
		Round:   round,
		Step:    step,
		Proof:   storage.Proof,
	}
}

func (c *Credential) LessThan(other *Credential) bool {
	return types.LessThanByProof(&c.Proof, &other.Proof)
}

func (c *Credential) String() string {
	return fmt.Sprintf("%d/%d/%d(%d) by %s, %x",
		c.Height, c.Round, c.Step, c.Weight, c.Address.String(), c.Proof[:3])
}

type ProposalLeaderData struct {
	Value common.Hash

	ESignValue ed25519.ForwardSecureSignature
	Credential
}

func NewProposalLeaderDataFromStorage(value common.Hash, height uint64, storage *types.ProposalStorage) *ProposalLeaderData {
	return &ProposalLeaderData{
		Value:      value,
		ESignValue: storage.ESignValue,
		Credential: NewCredentialFromCredentialStorage(&storage.Credential, height, storage.Round, types.RoundStep1Proposal),
	}
}

func (data *ProposalLeaderData) String() string {
	return fmt.Sprintf("ProposalLeader: %s %s",
		data.Value.TerminalString(), data.Credential.String())
}

func (data *ProposalLeaderData) SignBytes() []byte {
	return data.Value[:]
}

func (data *ProposalLeaderData) LessThan(other *ProposalLeaderData) bool {
	return data.Credential.LessThan(&other.Credential)
}

func (data *ProposalLeaderData) ToHasProposalData() *HasProposalData {
	return &HasProposalData{data.Height, data.Round, data.Credential.Proof, data.Value}
}

func (data *ProposalLeaderData) Valid(height uint64) bool {
	return data.Height == height && data.Round != types.BadRound && data.Step == types.RoundStep1Proposal
}

type ProposalBlockData struct {
	Block *types.Block

	ESignValue ed25519.ForwardSecureSignature
	Credential
}

func NewProposalBlockDataFromProposalStorage(leader *types.ProposalStorage, block *types.Block) *ProposalBlockData {
	credentialStorage := types.CredentialStorage{
		Address: block.Proposer(),
		Proof:   leader.Credential.Proof,
	}
	return &ProposalBlockData{
		Block:      block,
		ESignValue: leader.ESignValue,
		Credential: NewCredentialFromCredentialStorage(&credentialStorage, block.NumberU64(), leader.Round, types.RoundStep1Proposal),
	}
}

func (data *ProposalBlockData) String() string {
	return fmt.Sprintf("ProposalBlock: %s(%s) %s",
		data.Block.Hash().Hex(),
		data.Block.Coinbase().String(),
		data.Credential.String(),
	)
}

func (data *ProposalBlockData) NewProposalLeaderData() *ProposalLeaderData {
	return &ProposalLeaderData{
		Value:      data.Block.Hash(),
		ESignValue: data.ESignValue,
		Credential: data.Credential,
	}
}

func (data *ProposalBlockData) LessThan(other *ProposalBlockData) bool {
	return data.Credential.LessThan(&other.Credential)
}

func (data *ProposalBlockData) ToHasProposalData() *HasProposalData {
	return &HasProposalData{data.Height, data.Round, data.Credential.Proof, data.Block.Hash()}
}

func (data *ProposalBlockData) ToProposalStorage() types.ProposalStorage {
	return types.ProposalStorage{
		ESignValue: data.ESignValue,
		Round:      data.Round,
		Credential: types.CredentialStorage{
			Address: data.Credential.Address,
			Proof:   data.Credential.Proof,
		},
		SeedProof: data.Block.SeedProof(),
	}
}

func (data *ProposalBlockData) SignBytes() []byte {
	value := data.Block.Hash()
	return value[:]
}
func (data *ProposalBlockData) Valid(height uint64) bool {
	return data.Height == height && data.Round != types.BadRound && data.Step == types.RoundStep1Proposal
}

type HasVoteData struct {
	Address common.Address
	Height  uint64
	Round   uint32
	Step    uint32
}

func (data *HasVoteData) String() string {
	return fmt.Sprintf("HasVote: %s %s %d/%d/%d",
		data.Address.String(),
		types.VoteTypeString(types.VoteTypeOfStep(data.Step)),
		data.Height, data.Round, data.Step,
	)
}

func ToHasVote(data *VoteData) *HasVoteData {
	return &HasVoteData{
		data.Address,
		data.Height,
		data.Round,
		data.Step,
	}
}

// Vote represents votes produced by committee members
type VoteData struct {
	Value      common.Hash                    `json:"value" gencodec:"required"`
	ESignValue ed25519.ForwardSecureSignature `json:"eSignature" gencodec:"required"`

	Credential `json:"credential" gencodec:"required"`
}

func NewVoteDataFromCertVoteStorage(storage *types.CertVoteStorage, height uint64, round uint32, value common.Hash) *VoteData {
	return &VoteData{
		Value:      value,
		ESignValue: storage.ESignValue,
		Credential: NewCredentialFromCredentialStorage(&storage.Credential, height, round, types.RoundStep3Certifying),
	}
}

func (data *VoteData) SignBytes() []byte {
	return data.Value[:]
}

func (data *VoteData) Valid(height uint64, emptyValue common.Hash) bool {
	if data.Height != height || data.Round == types.BadRound || data.Step == types.BadStep || data.Step == types.RoundStep1Proposal || data.Step > types.MaxStep {
		return false
	}

	if data.Step == types.RoundStep2Filtering || data.Step == types.RoundStep3Certifying {
		if data.Value == emptyValue {
			return false
		}
	}

	return true
}

func (data *VoteData) String() string {
	return fmt.Sprintf("%s: %s %s",
		types.VoteTypeString(types.VoteTypeOfStep(data.Step)),
		data.Value.TerminalString(),
		data.Credential.String(),
	)
}

type HasProposalData struct {
	Height uint64
	Round  uint32
	Proof  ed25519.VrfProof
	Value  common.Hash
}

func (data *HasProposalData) String() string {
	return fmt.Sprintf("HasProposalData: %d/%d %x %s", data.Height, data.Round, data.Proof[:3], data.Value.TerminalString())
}
