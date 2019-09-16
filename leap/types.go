package leap

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/kaleidochain/kaleido/consensus/algorand/core"

	"github.com/kaleidochain/kaleido/common"
)

const (
	StampingStatusMsg = 0x00
	StampingVoteMsg   = 0x01
	HasSCVoteMsg      = 0x02
)

var CodeToString = map[uint64]string{
	StampingStatusMsg: "StampingStatusMsg",
	StampingVoteMsg:   "StampingVoteMsg",
	HasSCVoteMsg:      "HasSCVoteMsg",
}

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

func (h *HeightVoteSet) HasVote(vote *core.StampingVote) bool {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	return h.hasVote(vote)
}

func (h *HeightVoteSet) hasVote(vote *core.StampingVote) bool {
	if h.userSet == nil {
		return false
	}

	if h.userSet[vote.Height] == nil {
		return false
	}

	useSet := h.userSet[vote.Height]
	return useSet.Has(vote.Address)
}

func (h *HeightVoteSet) SetHasVote(has *HasSCVoteData) {
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

func (h *HeightVoteSet) RandomNotIn(votes []*core.StampingVote) *core.StampingVote {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	var notInVotes []*core.StampingVote
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
	Hash   common.Hash // TODO: need verify
	Seed   common.Hash
	Root   common.Hash
	Votes  []*core.StampingVote
}

func NewStampingCertificate(height uint64, proofHeader *Header) *StampingCertificate {
	return &StampingCertificate{
		Height: height,
		Seed:   proofHeader.Seed,
		Root:   proofHeader.Root,
	}
}

func NewStampingCertificateWithVotes(height uint64, proofHeader *Header, votes []*core.StampingVote) *StampingCertificate {
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
	// TODO: verify votes
}

func (sc *StampingCertificate) AddVote(vote *core.StampingVote) {
	sc.Votes = append(sc.Votes, vote)
}

type HasSCVoteData struct {
	Address common.Address
	Height  uint64
}

func NewHasSCVoteData(address common.Address, height uint64) *HasSCVoteData {
	return &HasSCVoteData{
		Address: address,
		Height:  height,
	}
}

func ToHasSCVoteData(vote *core.StampingVote) *HasSCVoteData {
	return NewHasSCVoteData(vote.Address, vote.Height)
}

type StampingVotes struct {
	votes  map[common.Address]*core.StampingVote
	weight uint64
	ts     int64
}

func (s *StampingVotes) hasVote(address common.Address) bool {
	_, ok := s.votes[address]
	return ok
}

func (s *StampingVotes) addVote(vote *core.StampingVote) {
	s.votes[vote.Address] = vote
}

func (s *StampingVotes) setEnoughTs() {
	s.ts = time.Now().Unix()
}

func NewStampingVotes() *StampingVotes {
	return &StampingVotes{
		votes: make(map[common.Address]*core.StampingVote),
	}
}
