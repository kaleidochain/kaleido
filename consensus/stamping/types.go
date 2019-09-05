package stamping

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/kaleidochain/kaleido/common"
)

const (
	StampingStatusMsg = 0x00
	StampingVoteMsg   = 0x01
	HasVoteMsg        = 0x02
)

const msgChanSize = 4096

type message struct {
	code uint64
	data interface{}
	from string
}

// UserSet
type UserSet struct {
	exists map[string]bool
}

func NewUserSet() *UserSet {
	return &UserSet{
		exists: make(map[string]bool),
	}
}

func (us *UserSet) Has(user common.Address) bool {
	return us.exists[user.Str()]
}

func (us *UserSet) Add(user common.Address) {
	if !us.exists[user.Str()] {
		us.exists[user.Str()] = true
	}
}

type HeightVoteSet struct {
	userSet map[uint64]*UserSet
	mutex   sync.RWMutex
}

func (h *HeightVoteSet) HasVote(vote *StampingVote) bool {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	return h.hasVote(vote)
}

func (h *HeightVoteSet) hasVote(vote *StampingVote) bool {
	if h.userSet == nil {
		return false
	}

	if h.userSet[vote.Height] == nil {
		return false
	}

	useSet := h.userSet[vote.Height]
	return useSet.Has(vote.Address)
}

func (h *HeightVoteSet) SetHasVote(has *HasVoteData) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	var userSet *UserSet
	if h.userSet[has.Height] == nil {
		userSet = NewUserSet()
		h.userSet[has.Height] = userSet
	} else {
		userSet = h.userSet[has.Height]
	}

	userSet.Add(has.Address)
}

func (h *HeightVoteSet) RandomNotIn(votes []*StampingVote) *StampingVote {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	var notInVotes []*StampingVote
	for i := range votes {
		if !h.hasVote(votes[i]) {
			notInVotes = append(notInVotes, votes[i])
		}
	}

	if len(notInVotes) > 0 {
		return notInVotes[rand.Intn(len(notInVotes))]
	}

	return nil
}

func (h *HeightVoteSet) Remove(beginHeight, endHeight uint64) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	for height := beginHeight; height < endHeight; height++ {
		delete(h.userSet, height)
	}
}

func (h *HeightVoteSet) Print(height uint64) string {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.userSet == nil {
		return "no record"
	}
	if h.userSet[height] == nil {
		return fmt.Sprintf("no height(%d) record", height)
	}

	all := ""
	for k, v := range h.userSet[height].exists {
		all += fmt.Sprintf("%s:%t ", hex.EncodeToString([]byte(k)), v)
	}
	return all
}

func NewHeightVoteSet() *HeightVoteSet {
	return &HeightVoteSet{
		userSet: make(map[uint64]*UserSet),
	}
}

type FinalCertificate struct {
	Height     uint64
	Hash       common.Hash
	ParentSeed common.Hash
	ParentRoot common.Hash
}

func NewFinalCertificate(header, parent *Header) *FinalCertificate {
	return &FinalCertificate{
		Height:     header.Height,
		Hash:       header.Hash(),
		ParentSeed: parent.Seed,
		ParentRoot: parent.Root,
	}
}

func (fc *FinalCertificate) Verify(header, parent *Header) bool {
	return fc.Height == header.Height &&
		fc.Hash == header.Hash() &&
		fc.Height == parent.Height+1 &&
		fc.ParentSeed == parent.Seed &&
		fc.ParentRoot == parent.Root
}

type StampingCertificate struct {
	Height uint64
	Seed   common.Hash
	Root   common.Hash
	Votes  []*StampingVote
}

func NewStampingCertificateWithVotes(height uint64, proofHeader *Header) *StampingCertificate {
	return &StampingCertificate{
		Height: height,
		Seed:   proofHeader.Seed,
		Root:   proofHeader.Root,
	}
}

func NewStampingCertificate(height uint64, proofHeader *Header, votes []*StampingVote) *StampingCertificate {
	return &StampingCertificate{
		Height: height,
		Seed:   proofHeader.Seed,
		Root:   proofHeader.Root,
		Votes:  votes,
	}
}

func (sc *StampingCertificate) Verify(config *Config, header, proofHeader *Header) bool {
	return sc.Height > config.HeightB() &&
		sc.Height == header.Height &&
		header.Height == proofHeader.Height+config.B &&
		sc.Seed == proofHeader.Seed &&
		sc.Root == proofHeader.Root
}

type HasVoteData struct {
	Address common.Address
	Height  uint64
}

func NewHasVoteData(address common.Address, height uint64) *HasVoteData {
	return &HasVoteData{
		Address: address,
		Height:  height,
	}
}

type StampingVote struct {
	Height  uint64
	Address common.Address
	Weight  uint64
}

func NewStampingVote(height uint64, address common.Address, weight uint64) *StampingVote {
	return &StampingVote{
		Height:  height,
		Address: address,
		Weight:  weight,
	}
}

func (vote *StampingVote) String() string {
	return fmt.Sprintf("%d(%d) by %s", vote.Height, vote.Weight, vote.Address.String())
}

func (vote *StampingVote) ToHasVoteData() *HasVoteData {
	return NewHasVoteData(vote.Address, vote.Height)
}

type StampingVotes struct {
	votes  map[common.Address]*StampingVote
	weight uint64
	ts     int64
}

func (s *StampingVotes) hasVote(address common.Address) bool {
	_, ok := s.votes[address]
	return ok
}

func (s *StampingVotes) addVote(vote *StampingVote) {
	s.votes[vote.Address] = vote
}

func (s *StampingVotes) setEnoughTs() {
	s.ts = time.Now().Unix()
}

func NewStampingVotes() *StampingVotes {
	return &StampingVotes{
		votes: make(map[common.Address]*StampingVote),
	}
}
