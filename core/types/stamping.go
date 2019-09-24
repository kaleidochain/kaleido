package types

import (
	"fmt"

	"github.com/kaleidochain/kaleido/params"

	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/crypto/ed25519"
)

type StampingStatus struct {
	Height    uint64
	Candidate uint64
	Proof     uint64
	Fz        uint64
}

func (s *StampingStatus) String() string {
	return fmt.Sprintf("%d/%d/%d/%d", s.Fz, s.Proof, s.Candidate, s.Height)
}

type StampingVote struct {
	Value      common.Hash                    `json:"value" gencodec:"required"`
	ESignValue ed25519.ForwardSecureSignature `json:"eSignature" gencodec:"required"`
	Credential `json:"credential" gencodec:"required"`
	TrieProof  NodeList `json:"trieProof" gencodec:"required"`
}

func (vote *StampingVote) SignBytes() []byte {
	return vote.Value[:]
}

func (vote *StampingVote) String() string {
	return fmt.Sprintf("%s: %s %s",
		"Stamping",
		vote.Value.TerminalString(),
		fmt.Sprintf("%d(%d) by %s, %x", vote.Height, vote.Weight, vote.Address.String(), vote.Proof[:3]),
	)
}

type StampingCertificate struct {
	Height uint64
	Hash   common.Hash // TODO: need verify
	Votes  []*StampingVote
}

func NewStampingCertificate(header *Header, votes []*StampingVote) *StampingCertificate {
	return &StampingCertificate{
		Height: header.NumberU64(),
		Hash:   header.Hash(),
		Votes:  votes,
	}
}

func (sc *StampingCertificate) Verify(config *params.ChainConfig, header, proofHeader *Header) error {
	if sc.Height <= config.Stamping.HeightB() {
		return fmt.Errorf("sc(%d) <= B", sc.Height)
	}

	if sc.Height != header.NumberU64() && sc.Height != proofHeader.NumberU64()+config.Stamping.B {
		return fmt.Errorf("sc height error, sc: %d, header: %d, proof: %d", sc.Height, header.NumberU64(), proofHeader.NumberU64())
	}

	if sc.Hash != header.Hash() {
		return fmt.Errorf("sc hash error, sc.Hash: %s, header.Hash: %s", sc.Hash.TerminalString(), header.Hash().TerminalString())
	}

	for _, vote := range sc.Votes {
		if vote.Height != sc.Height {
			return fmt.Errorf("vote height isnot equal sc.height, sc.Height: %d, vote: %s", sc.Height, vote.String())
		}

		if vote.Value != sc.Hash {
			return fmt.Errorf("vote hash isnot equal sc.height, sc.Hash: %s, vote: %s", sc.Hash.TerminalString(), vote.String())
		}
	}

	return nil
}

func (sc *StampingCertificate) AddVote(vote *StampingVote) {
	sc.Votes = append(sc.Votes, vote)
}

func (sc *StampingCertificate) ToStampingVoteStorage() *StampingCertificateStorage {
	vs := make([]*StampingVoteStorage, 0, len(sc.Votes))
	for _, vote := range sc.Votes {
		vs = append(vs, &StampingVoteStorage{
			ESignValue: vote.ESignValue,
			Credential: CredentialStorage{
				Address: vote.Address,
				Proof:   vote.Proof,
			},
			TrieProof: vote.TrieProof,
		})
	}

	return &StampingCertificateStorage{
		Height:  sc.Height,
		Value:   sc.Hash,
		VoteSet: vs,
	}
}

type StampingVoteStorage struct {
	ESignValue ed25519.ForwardSecureSignature `json:"eSignature" gencodec:"required"`
	Credential CredentialStorage              `json:"credential" gencodec:"required"`
	TrieProof  NodeList                       `json:"trieProof" gencodec:"required"`
}

type StampingCertificateStorage struct {
	Height  uint64                 `json:"height" gencodec:"required"`
	Value   common.Hash            `json:"value" gencodec:"required"`
	VoteSet []*StampingVoteStorage `json:"voteSet" gencodec:"required"`
}

func (s *StampingCertificateStorage) ToStampingCertificate() *StampingCertificate {
	vs := make([]*StampingVote, 0, len(s.VoteSet))
	for _, vote := range s.VoteSet {
		vs = append(vs, &StampingVote{
			Value:      s.Value,
			ESignValue: vote.ESignValue,
			Credential: Credential{
				Address: vote.Credential.Address,
				Height:  s.Height,
				Proof:   vote.Credential.Proof,
				//
				Round: 0,
				Step:  0,
			},
			TrieProof: vote.TrieProof,
		})
	}

	return &StampingCertificate{
		Height: s.Height,
		Hash:   s.Value,
		Votes:  vs,
	}
}
