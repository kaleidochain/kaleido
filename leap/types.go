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
	HandshakeMsg      = 0x00
	StampingStatusMsg = 0x01
	StampingVoteMsg   = 0x02
	HasSCVoteMsg      = 0x03
)

var CodeToString = map[uint64]string{
	HandshakeMsg:      "HandshakeMsg",
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

type HandshakeData struct {
	Version   uint32
	NetworkId uint64
	Genesis   common.Hash
	SCStatus
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

func NewFinalCertificate(header, parent *types.Header) *FinalCertificate {
	return &FinalCertificate{
		/*Height:     header.Height,
		Hash:       header.Hash(),
		ParentSeed: parent.Seed,
		ParentRoot: parent.Root,

		*/
	}
}

func (fc *FinalCertificate) Verify(header, parent *types.Header) bool {
	/*return fc.Height == header.Height &&
	fc.Hash == header.Hash() &&
	fc.Height == parent.Height+1 &&
	fc.ParentSeed == parent.Seed &&
	fc.ParentRoot == parent.Root

	*/
	return false
}

type StampingCertificate struct {
	Height uint64
	Hash   common.Hash // TODO: need verify
	Seed   common.Hash
	Root   common.Hash
	Votes  []*types.StampingVote
}

func NewStampingCertificate(height uint64, proofHeader *types.Header) *StampingCertificate {
	return &StampingCertificate{
		/*
			Height: height,
			Seed:   proofHeader.Seed,
			Root:   proofHeader.Root,
		*/
	}
}

func NewStampingCertificateWithVotes(height uint64, proofHeader *types.Header, votes []*types.StampingVote) *StampingCertificate {
	return &StampingCertificate{
		/*
			Height: height,
			Seed:   proofHeader.Seed,
			Root:   proofHeader.Root,
			Votes:  votes,
		*/
	}
}

func (sc *StampingCertificate) Verify(config *params.ChainConfig, header, proofHeader *types.Header) bool {
	/*return sc.Height > config.HeightB() &&
	sc.Height == header.Height &&
	header.Height == proofHeader.Height+config.B &&
	sc.Seed == proofHeader.Seed &&
	sc.Root == proofHeader.Root

	*/
	// TODO: verify votes

	return false
}

func (sc *StampingCertificate) AddVote(vote *types.StampingVote) {
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

func ToHasSCVoteData(vote *types.StampingVote) *HasSCVoteData {
	return NewHasSCVoteData(vote.Address, vote.Height)
}

type StampingVotes struct {
	votes  map[common.Address]*types.StampingVote
	weight uint64
	ts     int64
}

func (s *StampingVotes) hasVote(address common.Address) bool {
	_, ok := s.votes[address]
	return ok
}

func (s *StampingVotes) addVote(vote *types.StampingVote) {
	s.votes[vote.Address] = vote
}

func (s *StampingVotes) setEnoughTs() {
	s.ts = time.Now().Unix()
}

func NewStampingVotes() *StampingVotes {
	return &StampingVotes{
		votes: make(map[common.Address]*types.StampingVote),
	}
}
