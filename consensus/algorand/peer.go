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

package algorand

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/kaleidochain/kaleido/p2p/enode"

	"github.com/kaleidochain/kaleido/common"

	"github.com/kaleidochain/kaleido/consensus/algorand/core"

	"github.com/kaleidochain/kaleido/core/types"

	"github.com/ethereum/go-ethereum/log"

	"github.com/kaleidochain/kaleido/p2p"
)

var (
	errClosed            = errors.New("peer set is closed")
	errAlreadyRegistered = errors.New("peer is already registered")
)

const (
	handshakeTimeout = 5 * time.Second
	msgQueueSize     = 1024
)

// peerIdKey returns id key for internal peer
func peerIdKey(id enode.ID) string {
	return id.TerminalString()
}

type PeerInfo struct {
	Version uint32
	Height  uint64
	Round   uint32
}

type message struct {
	code uint64
	data interface{}
}

type peer struct {
	id      string
	version uint32

	*p2p.Peer
	rw                 p2p.MsgReadWriter
	closeChan          chan struct{}
	msgChan            chan message
	voteChan           chan *core.VoteData
	proposalLeaderChan chan *core.ProposalLeaderData

	mutex            sync.RWMutex
	heightUpdateTime time.Time
	height           uint64
	round            uint32
	counter          *core.HeightVoteSet

	leaderProposalValue      map[uint32]*core.HasProposalData // round => leader's info
	receivedProposalBlockMap map[string]bool                  // value => bool
}

func newPeer(p *p2p.Peer, rw p2p.MsgReadWriter) *peer {
	newPeer := &peer{
		id:                 peerIdKey(p.ID()),
		Peer:               p,
		rw:                 rw,
		closeChan:          make(chan struct{}),
		msgChan:            make(chan message, msgQueueSize),
		voteChan:           make(chan *core.VoteData, msgQueueSize),
		proposalLeaderChan: make(chan *core.ProposalLeaderData, msgQueueSize),
	}
	return newPeer
}

func (p *peer) Close() {
	close(p.closeChan)
}

func (p *peer) IsClosed() bool {
	select {
	case <-p.closeChan:
		return true
	default:
		return false
	}
}

func (p *peer) Log() log.Logger {
	return log.New("id", p.ID(), "HR", p.hrString())
}

// Info gathers and returns a collection of metadata known about a peer.
func (p *peer) Info() *PeerInfo {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return &PeerInfo{
		Version: p.version,
		Height:  p.height,
		Round:   p.round,
	}
}

// String implements fmt.Stringer.
func (p *peer) String() string {
	return fmt.Sprintf("%s-v%d-%d-%d", p.id, p.version, p.height, p.round)
}

// HR retrieves a copy of the current Height/Round of peer.
func (p *peer) HR() (uint64, uint32, time.Time) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.height, p.round, p.heightUpdateTime
}

// hrString returns a string of the current Height/Round of peer.
func (p *peer) hrString() string {
	return fmt.Sprintf("%d/%d", p.height, p.round)
}

// UpdateHR updates the Height/Round of the peer.
func (p *peer) UpdateHR(height uint64, round uint32) {
	if p.shouldIgnore(height, round) {
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.shouldIgnore(height, round) { // double check
		return
	}

	if height == 0 && round == types.BadRound { // 0 is for suspend gossip
		p.Log().Debug("Peer suspend gossip", "currentHR", p.hrString())

		p.heightUpdateTime = time.Now()
		p.height = 0
		p.round = types.BadRound
		p.counter = nil
		p.leaderProposalValue = nil
		p.receivedProposalBlockMap = nil
		return
	}

	p.Log().Debug("Peer set newer HR",
		"currentHR", p.hrString(),
		"newerHR", fmt.Sprintf("%d/%d", height, round))

	if height > p.height {
		// higher height
		p.heightUpdateTime = time.Now()
		p.height = height
		p.round = round

		p.counter = core.NewHeightVoteSet()
		p.leaderProposalValue = make(map[uint32]*core.HasProposalData)
		p.receivedProposalBlockMap = make(map[string]bool)
	} else { // height == curHeight
		if round > p.round {
			p.round = round
		}
	}
}

func (p *peer) shouldIgnore(height uint64, round uint32) bool {
	if height == 0 && round == types.BadRound {
		return false
	}
	if height < p.height {
		return true
	}
	if height == p.height {
		if round <= p.round {
			return true
		}
	}

	return false
}

func (p *peer) Handshake(height uint64, round uint32) error {
	// Send out own handshake in a new thread
	errCh := make(chan error, 2)
	var handshake core.HandshakeData // safe to read after two values have been received from errCh

	go func() {
		errCh <- p2p.Send(p.rw, core.HandshakeMsg, &core.HandshakeData{
			Version: uint32(p.version),
			Height:  height,
			Round:   round,
		})
	}()
	go func() {
		errCh <- p.readStatus(&handshake)
	}()
	timeout := time.NewTimer(handshakeTimeout)
	defer timeout.Stop()
	for i := 0; i < 2; i++ {
		select {
		case err := <-errCh:
			if err != nil {
				return err
			}
		case <-timeout.C:
			return p2p.DiscReadTimeout
		}
	}

	p.version = handshake.Version
	p.UpdateHR(handshake.Height, handshake.Round)
	return nil
}

func (p *peer) readStatus(handshake *core.HandshakeData) (err error) {
	msg, err := p.rw.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Code != core.HandshakeMsg {
		return errResp(ErrNoStatusMsg, "first msg has code %x (!= %x)", msg.Code, core.HandshakeMsg)
	}
	if msg.Size > ProtocolMaxMsgSize {
		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, ProtocolMaxMsgSize)
	}
	// Decode the handshake and make sure everything matches
	if err := msg.Decode(&handshake); err != nil {
		return errResp(ErrDecode, "msg %v: %v", msg, err)
	}
	if handshake.Version != p.version {
		return errResp(ErrProtocolVersionMismatch, "%d (!= %d)", handshake.Version, p.version)
	}
	return nil
}

// PickNextVoteAndSend pick a next vote and send it, return if has picked a next vote
func (p *peer) PickNextVoteAndSend(roundVoteSet *core.RoundVoteSet, height uint64, round uint32) bool {
	if roundVoteSet == nil {
		return false
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if height != p.height {
		return false
	}

	vote := roundVoteSet.PickNextVoteToSend(p.counter.RoundVoteSet(round))
	if vote == nil {
		return false
	}

	p.sendVoteAndSetHasVoteNoLock(vote)
	return true
}

// PickAndSend pick a vote and send it, return if has picked a vote
func (p *peer) PickAndSend(roundVoteSet *core.RoundVoteSet, height uint64, round uint32) bool {
	if roundVoteSet == nil {
		return false
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if height != p.height {
		return false
	}

	vote := roundVoteSet.PickVoteToSend(p.counter.RoundVoteSet(round))
	if vote == nil {
		return false
	}

	p.sendVoteAndSetHasVoteNoLock(vote)
	return true
}

func (p *peer) SetHasVote(data *core.HasVoteData) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.height != data.Height {
		return
	}

	p.counter.SetHasVote(data.Round, data.Step, data.Address)
	p.Log().Trace("SetHasVote OK", "data", data, "HR", p.hrString())
}

func (p *peer) SendVote(data *core.VoteData) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.height != data.Height {
		return
	}

	if p.counter.HasVote(data.Round, data.Step, data.Address) {
		return
	}

	p.sendVoteAndSetHasVoteNoLock(data)
}

func (p *peer) sendVoteAndSetHasVoteNoLock(data *core.VoteData) {
	err := p2p.Send(p.rw, core.VoteMsg, data)
	if err != nil {
		p.Log().Debug("SendVote fail", "data", data, "err", err)
		return
	}

	p.counter.SetHasVote(data.Round, data.Step, data.Address)
	p.Log().Trace("SendVote OK", "data", data)
}

func (p *peer) SendProposalLeader(data *core.ProposalLeaderData) bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if data.Height != p.height {
		p.Log().Trace("SendProposalLeader fail, height not match",
			"dataHeight", data.Height, "peerHeight", p.height)
		return false
	}

	if p.hasProposalValueNoLock(data) {
		return false
	}

	err := p2p.Send(p.rw, core.ProposalLeaderMsg, data)
	if err != nil {
		p.Log().Debug("SendProposalValueMessage fail", "proposalValue", data, "err", err)
		return false
	}
	p.Log().Trace("SendProposalValueMessage sent OK", "proposalValue", data)

	p.setHasProposalValueNoLock(data.ToHasProposalData())

	return true
}

func (p *peer) SendProposalBlock(data *core.ProposalBlockData) bool {
	if data == nil {
		return false
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if data.Block.NumberU64() != p.height {
		return false
	}

	if p.hasProposalBlockNoLock(data) {
		return false
	}

	err := p2p.Send(p.rw, core.ProposalBlockMsg, data)
	if err != nil {
		p.Log().Debug("SendProposalBlock sent fail", "proposalBlock", data, "err", err)
		return false
	}
	p.Log().Trace("SendProposalBlock sent OK", "proposalBlock", data)

	p.setHasProposalBlockNoLock(data.ToHasProposalData())

	return true
}

func (p *peer) SetHasProposalValue(data *core.HasProposalData) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if data.Height != p.height {
		return
	}

	p.setHasProposalValueNoLock(data)
}

func (p *peer) setHasProposalValueNoLock(data *core.HasProposalData) {
	leader := p.leaderProposalValue[data.Round]
	if leader == nil || types.LessThanByProof(&data.Proof, &leader.Proof, data.Weight, leader.Weight) {
		p.leaderProposalValue[data.Round] = data
	}
}

func (p *peer) hasProposalValueNoLock(data *core.ProposalLeaderData) bool {
	leader := p.leaderProposalValue[data.Round]
	if leader == nil || types.LessThanByProof(&data.Proof, &leader.Proof, data.Weight, leader.Weight) {
		return false
	}
	return true
}

func (p *peer) GetProposalLeaderValue(round uint32) common.Hash {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	leader := p.leaderProposalValue[round]
	if leader != nil {
		return leader.Value
	}

	return common.Hash{}
}

func (p *peer) SetHasProposalBlock(data *core.HasProposalData) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if data.Height != p.height {
		return
	}

	p.setHasProposalBlockNoLock(data)
}

func (p *peer) setHasProposalBlockNoLock(data *core.HasProposalData) {
	p.receivedProposalBlockMap[data.Value.Str()] = true

	p.setHasProposalValueNoLock(data)
}

func (p *peer) hasProposalBlockNoLock(data *core.ProposalBlockData) bool {
	return p.receivedProposalBlockMap[data.Block.Hash().Str()]
}

func (p *peer) PickUnknownBlockIndex(values []common.Hash) int {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	index := make([]int, 0, len(values))
	for i, value := range values {
		if exists := p.receivedProposalBlockMap[value.Str()]; !exists {
			index = append(index, i)
		}
	}

	if len(index) > 0 {
		return index[rand.Intn(len(index))]
	}

	return -1
}

func (p *peer) SendMsgAsync(code uint64, data interface{}) {
	select {
	case p.msgChan <- message{code: code, data: data}:
	default:
		p.Log().Warn("msgChan full")
	}
}

func (p *peer) SendVoteAsync(data *core.VoteData) {
	select {
	case p.voteChan <- data:
	default:
		p.Log().Warn("voteChan full")
	}
}

func (p *peer) SendProposalLeaderAsync(data *core.ProposalLeaderData) {
	select {
	case p.proposalLeaderChan <- data:
	default:
		p.Log().Warn("proposalLeaderChan full")
	}
}

func (p *peer) broadcaster() {
	for {
		select {
		case <-p.closeChan:
			return
		case msg := <-p.msgChan:
			err := p2p.Send(p.rw, msg.code, msg.data)
			if err != nil {
				p.Log().Debug("Send fail", "code", core.CodeToString[msg.code], "data", msg.data)
			} else {
				p.Log().Trace("Send sent OK", "code", core.CodeToString[msg.code], "data", msg.data)
			}
		case vote := <-p.voteChan:
			p.SendVote(vote)
		case leader := <-p.proposalLeaderChan:
			p.SendProposalLeader(leader)
		}
	}
}

// ------------------

// peerSet represents the collection of active peers currently participating in
// the Ethereum sub-protocol.
type peerSet struct {
	peers  map[string]*peer
	lock   sync.RWMutex
	closed bool
}

// newPeerSet creates a new peer set to track the active participants.
func newPeerSet() *peerSet {
	return &peerSet{
		peers: make(map[string]*peer),
	}
}

// Register injects a new peer into the working set, or returns an error if the
// peer is already known.
func (ps *peerSet) Register(p *peer) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	if ps.closed {
		return errClosed
	}
	if _, ok := ps.peers[p.id]; ok {
		return errAlreadyRegistered
	}
	ps.peers[p.id] = p
	return nil
}

// Unregister removes a remote peer from the active set, disabling any further
// actions to/from that particular entity.
func (ps *peerSet) Unregister(p *peer) {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	if _, ok := ps.peers[p.id]; !ok {
		log.Warn("PeerSet has no this peer", "peer", p.id)
		return
	}
	delete(ps.peers, p.id)
	p.Close()
	return
}

// Peer retrieves the registered peer with the given id.
func (ps *peerSet) Peer(id enode.ID) *peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return ps.peers[peerIdKey(id)]
}

// Len returns if the current number of peers in the set.
func (ps *peerSet) Len() int {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return len(ps.peers)
}

// Close disconnects all peers.
// No new peers can be registered after Close has returned.
func (ps *peerSet) Close() {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	for _, p := range ps.peers {
		p.Disconnect(p2p.DiscQuitting)
	}
	ps.closed = true
}

// ForEach for each peer call function `do`
func (ps *peerSet) ForEach(do func(*peer)) {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	for _, p := range ps.peers {
		do(p)
	}
}
