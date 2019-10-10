package leap

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/kaleidochain/kaleido/params"

	"github.com/kaleidochain/kaleido/core/types"

	"github.com/kaleidochain/kaleido/common"
)

const (
	HandshakeMsg         = 0x00
	StampingStatusMsg    = 0x01
	StampingVoteMsg      = 0x02
	HasSCVoteMsg         = 0x03
	GetNextBreadcrumbMsg = 0x04
	NextBreadcrumbMsg    = 0x05
	GetHeadersMsg        = 0x06
	HeadersMsg           = 0x07
)

var CodeToString = map[uint64]string{
	HandshakeMsg:      "HandshakeMsg",
	StampingStatusMsg: "StampingStatusMsg",
	StampingVoteMsg:   "StampingVoteMsg",
	HasSCVoteMsg:      "HasSCVoteMsg",
}

const msgChanSize = 4096

type getNextBreadcrumbData struct {
	Begin, End uint64
	Status     types.StampingStatus
}

type getHeadersData struct {
	Begin, End         uint64
	Forward, IncludeFc bool
}

type message struct {
	code uint64
	data interface{}
	from string
}

type HandshakeData struct {
	Version   uint32
	NetworkId uint64
	Genesis   common.Hash
	Stamping  params.StampingConfig
	types.StampingStatus
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

func (h *HeightVoteSet) HasVote(vote *types.StampingVote) bool {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	return h.hasVote(vote)
}

func (h *HeightVoteSet) hasVote(vote *types.StampingVote) bool {
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

func (h *HeightVoteSet) RandomNotIn(votes []*types.StampingVote) *types.StampingVote {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	var notInVotes []*types.StampingVote
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

func ToHasSCVoteData(vote *types.StampingVote) *HasSCVoteData {
	return NewHasSCVoteData(vote.Address, vote.Height)
}

type StampingVotes struct {
	votes  map[string]*types.StampingVote
	weight uint64
	ts     int64
}

func (s *StampingVotes) hasVote(address common.Address) bool {
	_, ok := s.votes[address.String()]
	return ok
}

func (s *StampingVotes) addVote(vote *types.StampingVote) {
	s.votes[vote.Address.String()] = vote
}

func (s *StampingVotes) setEnoughTs() {
	s.ts = time.Now().Unix()
}

func NewStampingVotes() *StampingVotes {
	return &StampingVotes{
		votes: make(map[string]*types.StampingVote),
	}
}
