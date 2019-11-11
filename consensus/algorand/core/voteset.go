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
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"

	"github.com/kaleidochain/kaleido/core/types"

	"github.com/ethereum/go-ethereum/log"
	"github.com/kaleidochain/kaleido/common"
)

const (
	potentialValueCap = 3
)

// HeightVoteSet -> RoundVoteSet -> StepVoteSet -> VoteSet, UserSet

// UserSet
type UserSet struct {
	exists map[string]bool
}

func (us *UserSet) Has(user common.Address) bool {
	return us.exists[user.Str()]
}

func (us *UserSet) Add(user common.Address) {
	if !us.exists[user.Str()] {
		us.exists[user.Str()] = true
	}
}

// ----------------

// VoteSet stores the value info for same value by same vote type in same round
type VoteSet struct {
	Weight uint64 // sum of all voter's weight
	Votes  []*VoteData
}

// ----------------

// StepVoteSet saves all votes for each step
type StepVoteSet struct {
	Values         map[string]*VoteSet // value => value info
	MajorValue     common.Hash         // the major value reached 2/3+ (2t+1)
	PotentialValue []common.Hash       // the potential values reached 1/3+ (t+1), maybe more than one
	userSet        UserSet
}

func RandomSelect(from []*VoteData) *VoteData {
	if len(from) == 0 {
		return nil
	}
	msg := from[rand.Intn(len(from))]
	return msg
}

func (svs *StepVoteSet) RandomNotIn(other *StepVoteSet) *VoteData {
	if len(svs.Values) == 0 {
		return nil
	}

	var otherUserSet *UserSet
	if other != nil {
		otherUserSet = &other.userSet
	}

	// pick major value first
	if !common.EmptyHash(svs.MajorValue) {
		major := svs.Values[svs.MajorValue.Str()]

		tmp := make([]*VoteData, 0, len(major.Votes))
		tmp = appendNotIn(tmp, major.Votes, otherUserSet)
		if len(tmp) > 0 {
			return RandomSelect(tmp)
		}
	}

	// then pick potential values
	if len(svs.PotentialValue) > 0 {
		total := 0
		for _, v := range svs.PotentialValue {
			total += len(svs.Values[v.Str()].Votes)
		}
		tmp := make([]*VoteData, 0, total)
		for _, v := range svs.PotentialValue {
			tmp = appendNotIn(tmp, svs.Values[v.Str()].Votes, otherUserSet)
		}
		if len(tmp) > 0 {
			return RandomSelect(tmp)
		}
	}

	// then others
	tmp := make([]*VoteData, 0)
	for _, self := range svs.Values {
		tmp = appendNotIn(tmp, self.Votes, otherUserSet)
	}
	return RandomSelect(tmp)
}

func appendNotIn(tmp []*VoteData, votes []*VoteData, userSet *UserSet) []*VoteData {
	for _, vote := range votes {
		if userSet == nil || !userSet.Has(vote.Address) {
			tmp = append(tmp, vote)
		}
	}
	return tmp
}

func NewStepVoteSet() *StepVoteSet {
	return &StepVoteSet{
		Values:         make(map[string]*VoteSet),
		PotentialValue: make([]common.Hash, 0, potentialValueCap),
		userSet:        UserSet{make(map[string]bool)},
	}
}

func (svs *StepVoteSet) hasPotentialValue(value common.Hash) bool {
	// This is a small set(<=3), we can iterate them efficiently
	for i := 0; i < len(svs.PotentialValue); i++ {
		if value == svs.PotentialValue[i] {
			return true
		}
	}
	return false
}

//-------------------------------------

// RoundVoteSet saves all votes for this round
type RoundVoteSet struct {
	mutex           sync.RWMutex
	softVoteSet     *StepVoteSet            // one Filtering Step => soft vote
	certVoteSet     *StepVoteSet            // one Certifying Step => cert vote
	nextVoteSet     map[uint32]*StepVoteSet // many Finishing steps => next vote
	potentialValues []common.Hash
}

func NewRoundVoteSet() *RoundVoteSet {
	roundCounter := &RoundVoteSet{}
	roundCounter.softVoteSet = NewStepVoteSet()
	roundCounter.certVoteSet = NewStepVoteSet()
	roundCounter.nextVoteSet = make(map[uint32]*StepVoteSet)
	roundCounter.potentialValues = make([]common.Hash, 0, potentialValueCap)
	return roundCounter
}

func NewRoundVoteSetFromCertificates(votes []*VoteData, threshold uint64) *RoundVoteSet {
	lastCertVotes := NewRoundVoteSet()

	voteSet := &VoteSet{
		Weight: threshold,
		Votes:  make([]*VoteData, len(votes)),
	}
	copy(voteSet.Votes, votes)
	if len(votes) > 0 {
		lastCertVotes.certVoteSet.Values[votes[0].Value.Str()] = voteSet
	}

	for _, certVote := range votes {
		lastCertVotes.certVoteSet.userSet.Add(certVote.Address)
	}

	return lastCertVotes
}

func (rvs *RoundVoteSet) PickVoteToSend(other *RoundVoteSet) *VoteData {
	if rvs == nil {
		return nil
	}

	if other == nil {
		other = NewRoundVoteSet()
	}

	rvs.mutex.RLock()
	defer rvs.mutex.RUnlock()

	other.mutex.RLock()
	defer other.mutex.RUnlock()

	vote := rvs.certVoteSet.RandomNotIn(other.certVoteSet)
	if vote != nil {
		log.Trace("PickVoteToSend pick a cert-vote", "vote", vote)
		return vote
	}

	vote = rvs.softVoteSet.RandomNotIn(other.softVoteSet)
	if vote != nil {
		log.Debug("PickVoteToSend pick a soft-vote", "vote", vote)
		return vote
	}

	vote = rvs.pickNextVoteToSend(other)

	return vote
}

func (rvs *RoundVoteSet) PickNextVoteToSend(other *RoundVoteSet) *VoteData {
	if rvs == nil {
		return nil
	}

	if other == nil {
		other = NewRoundVoteSet()
	}

	rvs.mutex.RLock()
	defer rvs.mutex.RUnlock()

	other.mutex.RLock()
	defer other.mutex.RUnlock()

	return rvs.pickNextVoteToSend(other)
}

func (rvs *RoundVoteSet) pickNextVoteToSend(other *RoundVoteSet) *VoteData {
	// 这里要按step顺序来发，否则step落后的节点，收到的是不同step的，不能快速追赶
	steps := make([]int, 0, len(rvs.nextVoteSet))
	for step := range rvs.nextVoteSet {
		steps = append(steps, int(step))
	}

	// 从小到大排序
	// 尝试过从大到小选，会在step一直增长的情况下，导致小的选不上
	// 从小到大选比较保险和稳妥
	sort.Ints(steps)
	for _, step := range steps {
		vote := rvs.nextVoteSet[uint32(step)].RandomNotIn(other.nextVoteSet[uint32(step)])
		if vote != nil {
			return vote
		}
	}
	return nil
}

func (rvs *RoundVoteSet) getStepVoteSet(step uint32) *StepVoteSet {
	var valueCounter *StepVoteSet
	switch voteType := types.VoteTypeOfStep(step); voteType {
	case types.VoteTypeSoft:
		valueCounter = rvs.softVoteSet
	case types.VoteTypeCert:
		valueCounter = rvs.certVoteSet
	case types.VoteTypeNext:
		valueCounter = rvs.nextVoteSet[step]
	}
	return valueCounter
}

func (rvs *RoundVoteSet) ensureStepVoteSet(step uint32) *StepVoteSet {
	svs := rvs.getStepVoteSet(step)
	if svs == nil && types.VoteTypeOfStep(step) == types.VoteTypeNext {
		svs = NewStepVoteSet()
		rvs.nextVoteSet[step] = svs
	}
	return svs
}

func (rvs *RoundVoteSet) HasVote(step uint32, user common.Address) bool {
	rvs.mutex.RLock()
	defer rvs.mutex.RUnlock()

	svs := rvs.getStepVoteSet(step)
	if svs == nil {
		return false
	}
	return svs.userSet.Has(user)
}

func (rvs *RoundVoteSet) SetHasVote(step uint32, user common.Address) {
	rvs.mutex.Lock()
	defer rvs.mutex.Unlock()

	svs := rvs.ensureStepVoteSet(step)
	if svs == nil { // bad step
		return
	}

	svs.userSet.Add(user)
}

func (rvs *RoundVoteSet) IsNextVoteEnoughForEmptyAtAnyStep(empty common.Hash) bool {
	rvs.mutex.RLock()
	defer rvs.mutex.RUnlock()

	for _, svs := range rvs.nextVoteSet {
		if svs.MajorValue == empty {
			return true
		}
	}
	return false
}

// GetCertVotesOfValue returns cert votes of value
func (rvs *RoundVoteSet) GetCertVotesOfValue(value common.Hash) []*VoteData {
	rvs.mutex.RLock()
	defer rvs.mutex.RUnlock()

	vv := rvs.certVoteSet.Values[value.Str()]
	if vv == nil || len(vv.Votes) == 0 {
		return nil
	}

	votes := make([]*VoteData, len(vv.Votes))
	copy(votes, vv.Votes)
	return votes
}

func (rvs *RoundVoteSet) GetSoftMajorValue() common.Hash {
	rvs.mutex.RLock()
	defer rvs.mutex.RUnlock()

	return rvs.softVoteSet.MajorValue
}

func (rvs *RoundVoteSet) GetCertMajorValue() common.Hash {
	rvs.mutex.RLock()
	defer rvs.mutex.RUnlock()

	return rvs.certVoteSet.MajorValue
}

func (rvs *RoundVoteSet) hasPotentialValue(value common.Hash) bool {
	// This is a small set(<=3), we can iterate them efficiently
	for i := 0; i < len(rvs.potentialValues); i++ {
		if value == rvs.potentialValues[i] {
			return true
		}
	}
	return false
}

func (rvs *RoundVoteSet) HasPotentialValue(value common.Hash) bool {
	rvs.mutex.RLock()
	defer rvs.mutex.RUnlock()
	return rvs.hasPotentialValue(value)
}

func (rvs *RoundVoteSet) GetPotentialSoftedValues() []common.Hash {
	rvs.mutex.RLock()
	defer rvs.mutex.RUnlock()

	return rvs.softVoteSet.PotentialValue
}

func (rvs *RoundVoteSet) GetPotentialCertifiedValues() []common.Hash {
	rvs.mutex.RLock()
	defer rvs.mutex.RUnlock()

	return rvs.certVoteSet.PotentialValue
}

// enough表示此类投票是否已经出现+2/3的值了，目前在err==nil的情况下，与传入的值一样
func (rvs *RoundVoteSet) AddVoteAndCount(vote *VoteData, threshold uint64) (added, newPotential, enough bool, err error) {
	rvs.mutex.Lock()
	defer rvs.mutex.Unlock()

	stepVoteSet := rvs.ensureStepVoteSet(vote.Step)
	if stepVoteSet == nil {
		return false, false, false, errors.New("bad vote step")
	}

	if stepVoteSet.userSet.Has(vote.Address) {
		return false, false, false, errors.New("duplicate vote")
	}

	if !common.EmptyHash(stepVoteSet.MajorValue) {
		return false, true, true, errors.New(fmt.Sprintf("MajorValue %x exists", stepVoteSet.MajorValue[:3]))
	}

	voteValue := vote.Value
	voteSet, exists := stepVoteSet.Values[voteValue.Str()]
	if !exists {
		voteSet = &VoteSet{}
		stepVoteSet.Values[voteValue.Str()] = voteSet
	}

	if voteSet.Weight < threshold {
		voteSet.Votes = append(voteSet.Votes, vote)
		voteSet.Weight += vote.Weight
		stepVoteSet.userSet.Add(vote.Address)

		added = true
	}

	if voteSet.Weight*2 >= threshold+1 && !stepVoteSet.hasPotentialValue(voteValue) {
		stepVoteSet.PotentialValue = append(stepVoteSet.PotentialValue, voteValue)

		if !rvs.hasPotentialValue(voteValue) {
			rvs.potentialValues = append(rvs.potentialValues, voteValue)
		}

		newPotential = true
	}

	if voteSet.Weight >= threshold {
		stepVoteSet.MajorValue = voteValue

		enough = true
	}

	weightStr := fmt.Sprintf("%03d", voteSet.Weight)
	log.Debug(fmt.Sprintf("AddVoteAndCount OK Added=%v Potential=%v Enough=%v", added, newPotential, enough),
		"MajorValue", fmt.Sprintf("%x(%s/%03d)", stepVoteSet.MajorValue[:3], weightStr, threshold),
		"Vote", vote)

	return
}

//-------------------------------------

// HeightVoteSet saves all votes for this Height
type HeightVoteSet struct {
	roundVoteSet map[uint32]*RoundVoteSet // round -> counter
	mutex        sync.RWMutex
}

// NewHeightVoteSet makes a new object
func NewHeightVoteSet() *HeightVoteSet {
	return &HeightVoteSet{
		roundVoteSet: make(map[uint32]*RoundVoteSet),
	}
}

// AddVoteAndCheck adds and counts vote, added is true only if the vote value is not enough, enough is
// true only if the value is enough after added. So added && enough means the value is firstly enough.
// err will be set only if the vote is invalid. err should cause the peer removed.
func (hvs *HeightVoteSet) AddVoteAndCount(vote *VoteData, threshold uint64) (added, potential, enough bool, err error) {
	rvs := hvs.EnsureRoundVoteSet(vote.Round)
	return rvs.AddVoteAndCount(vote, threshold)
}

// EnsureRoundVoteSet return the RoundVoteSet of round, create it if not exists
func (hvs *HeightVoteSet) EnsureRoundVoteSet(round uint32) *RoundVoteSet {
	hvs.mutex.RLock()
	roundVoteSet, exists := hvs.roundVoteSet[round]
	hvs.mutex.RUnlock()

	if !exists {
		hvs.mutex.Lock()

		// must double check
		roundVoteSet, exists = hvs.roundVoteSet[round]
		if !exists {
			roundVoteSet = NewRoundVoteSet()
			hvs.roundVoteSet[round] = roundVoteSet
		}

		hvs.mutex.Unlock()
	}

	return roundVoteSet
}

// RoundVoteSet returns the RoundVoteSet of round, return nil if not exists
func (hvs *HeightVoteSet) RoundVoteSet(round uint32) *RoundVoteSet {
	hvs.mutex.RLock()
	defer hvs.mutex.RUnlock()

	return hvs.roundVoteSet[round]
}

// HasVote returns true if user had voted in certain round and step
func (hvs *HeightVoteSet) HasVote(round uint32, step uint32, user common.Address) bool {
	rvs := hvs.RoundVoteSet(round)
	if rvs == nil {
		return false
	}

	return rvs.HasVote(step, user)
}

// SetHasVote sets we had received the vote for round and step by user
func (hvs *HeightVoteSet) SetHasVote(round uint32, step uint32, user common.Address) {
	rvs := hvs.EnsureRoundVoteSet(round)

	rvs.SetHasVote(step, user)
}

// IsSoftVoteEnoughForNonEmptyForAnyRound returns true with the value if some value's soft votes is enough in any round
func (hvs *HeightVoteSet) IsSoftVoteEnoughForNonEmptyForAnyRound(empty common.Hash) (bool, common.Hash) {
	hvs.mutex.RLock()
	rounds := make([]int, len(hvs.roundVoteSet))
	rvsMap := make(map[int]*RoundVoteSet)
	i := 0
	for r, rvs := range hvs.roundVoteSet {
		rounds[i] = int(r)
		rvsMap[i] = rvs
		i = i + 1
	}
	hvs.mutex.RUnlock()

	// search from oldest, by sorting the rounds
	// 正常情况下，最小的最容易满足这个条件
	sort.Ints(rounds)

	for _, round := range rounds {
		value := rvsMap[round].GetSoftMajorValue()
		if !common.EmptyHash(value) && value != empty {
			return true, value
		}
	}

	return false, empty
}

// IsSoftVoteEnoughForNonEmpty returns true with the value if some value's soft votes is enough in round
func (hvs *HeightVoteSet) IsSoftVoteEnoughForNonEmpty(empty common.Hash, round uint32) (bool, common.Hash) {
	rvs := hvs.RoundVoteSet(round)
	if rvs == nil {
		return false, empty
	}

	value := rvs.GetSoftMajorValue()
	if !common.EmptyHash(value) && value != empty {
		return true, value
	}

	return false, empty
}

// IsNextVoteEnoughForEmptyAtAnyStep returns if next vote is enough for empty
func (hvs *HeightVoteSet) IsNextVoteEnoughForEmptyAtAnyStep(round uint32, empty common.Hash) bool {
	rvs := hvs.RoundVoteSet(round)
	if rvs == nil {
		return false
	}

	return rvs.IsNextVoteEnoughForEmptyAtAnyStep(empty)
}

// GetCertVotesOfValue returns cert votes for value in certain round
func (hvs *HeightVoteSet) GetCertVotesOfValue(round uint32, value common.Hash) []*VoteData {
	rvs := hvs.RoundVoteSet(round)
	if rvs == nil {
		return nil
	}

	return rvs.GetCertVotesOfValue(value)
}

func (hvs *HeightVoteSet) GetCertVoteStorage(round uint32, value common.Hash) []*types.CertVoteStorage {
	certVotes := hvs.GetCertVotesOfValue(round, value)
	if certVotes == nil {
		return nil
	}

	cvs := make([]*types.CertVoteStorage, 0, len(certVotes))
	for _, vote := range certVotes {
		cvs = append(cvs, &types.CertVoteStorage{
			ESignValue: vote.ESignValue,
			Credential: types.CredentialStorage{
				Address: vote.Address,
				Proof:   vote.Proof,
			},
		})
	}

	return cvs
}

func (hvs *HeightVoteSet) IsPotentialValue(round uint32, value common.Hash) bool {
	rvs := hvs.RoundVoteSet(round)
	if rvs == nil {
		return false
	}

	return rvs.HasPotentialValue(value)
}
