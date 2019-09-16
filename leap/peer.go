package leap

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kaleidochain/kaleido/consensus/algorand/core"

	"github.com/kaleidochain/kaleido/p2p"
	"github.com/kaleidochain/kaleido/p2p/enode"

	"github.com/kaleidochain/kaleido/core/types"

	"github.com/ethereum/go-ethereum/log"
)

var (
	errClosed            = errors.New("peer set is closed")
	errAlreadyRegistered = errors.New("peer is already registered")
	errNotRegistered     = errors.New("peer is not registered")
)

const (
	handshakeTimeout = 5 * time.Second
	msgQueueSize     = 1024
)

// peerIdKey returns id key for internal peer
func peerIdKey(id enode.ID) string {
	return id.TerminalString()
}

type peer struct {
	id      string
	version uint32

	*p2p.Peer
	rw        p2p.MsgReadWriter
	closeChan chan struct{}
	msgChan   chan message
	voteChan  chan *types.StampingVote

	scStatus SCStatus
	counter  *HeightVoteSet

	mutex sync.RWMutex

	recvChan chan message
	sendChan chan message

	chain *SCChain
}

func newPeer(p *p2p.Peer, rw p2p.MsgReadWriter) *peer {
	return &peer{
		id:        peerIdKey(p.ID()),
		Peer:      p,
		rw:        rw,
		counter:   NewHeightVoteSet(),
		closeChan: make(chan struct{}),
		msgChan:   make(chan message, msgQueueSize),
		voteChan:  make(chan *types.StampingVote, msgQueueSize),
		recvChan:  make(chan message, msgChanSize),
		sendChan:  make(chan message, msgChanSize),
	}
}

func (p *peer) setChain(chain *SCChain) {
	p.chain = chain
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
	return log.New("pid", p.id, "HR", p.statusString())
}

func (p *peer) statusString() string {
	return fmt.Sprintf("%d/%d/%d/%d", p.scStatus.Fz, p.scStatus.Proof, p.scStatus.Candidate, p.scStatus.Height)
}

func (p *peer) ChainStatus() SCStatus {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.scStatus
}

func (p *peer) string() string {
	return fmt.Sprintf("%s-%d-%d-%d-%d", p.id, p.scStatus.Fz, p.scStatus.Proof, p.scStatus.Candidate, p.scStatus.Height)
}

func (p *peer) Handshake(status SCStatus) error {
	// Send out own handshake in a new thread
	errCh := make(chan error, 2)
	var handshake HandshakeData // safe to read after two values have been received from errCh

	go func() {
		errCh <- p2p.Send(p.rw, HandshakeMsg, &HandshakeData{
			Version: uint32(p.version),
			SCStatus: SCStatus{
				Height:    status.Height,
				Candidate: status.Candidate,
				Proof:     status.Proof,
				Fz:        status.Fz,
			},
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
	p.updateStatus(handshake.SCStatus)
	return nil
}

func (p *peer) readStatus(handshake *HandshakeData) (err error) {
	msg, err := p.rw.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Code != HandshakeMsg {
		return errResp(ErrNoStatusMsg, "first msg has code %x (!= %x)", msg.Code, HandshakeMsg)
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

func (p *peer) SendSCVote(vote *types.StampingVote) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if vote.Height <= p.scStatus.Candidate {
		p.Log().Trace("SendVote too low", "vote", vote)
		return fmt.Errorf(fmt.Sprintf("SendVote too low, peer status:%s, vote:%v", p.statusString(), vote))
	}

	if p.counter.hasVote(vote) {
		p.Log().Trace("SendVote has vote", "vote", vote, "counter", p.counter.Print(vote.Height))
		return fmt.Errorf(fmt.Sprintf("SendVote has vote, peer status:%s, vote:%v", p.statusString(), vote))
	}

	p.sendVoteAndSetHasVoteNoLock(vote)
	return nil
}

func (p *peer) sendVoteAndSetHasVoteNoLock(vote *types.StampingVote) {
	p.send(message{
		code: StampingVoteMsg,
		data: vote,
		from: p.id,
	})

	p.counter.SetHasVote(ToHasSCVoteData(vote))
	p.Log().Trace("SendVote OK", "vote", vote)
}

func (p *peer) SetHasVote(data *HasSCVoteData) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if data.Height <= p.scStatus.Candidate {
		return
	}

	p.counter.SetHasVote(data)
	p.Log().Trace("SetHasVote OK", "data", data, "Status", p.statusString())
}

func (p *peer) SendMsg(msg message) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.send(message{
		code: msg.code,
		data: msg.data,
		from: p.id,
	})
}

func (p *peer) send(msg message) {
	select {
	case p.sendChan <- msg:
	default:
		p.Log().Info("sendChan full, msg:%v", msg)
	}
}

func (p *peer) handleMsg() {
	for {
		select {
		case msg := <-p.recvChan:
			switch msg.code {
			case StampingVoteMsg:
				p.counter.SetHasVote(ToHasSCVoteData(msg.data.(*types.StampingVote)))
				p.chain.OnReceive(StampingVoteMsg, msg.data, p.string())
			case StampingStatusMsg:
				status := msg.data.(*SCStatus)
				begin, end, updated := p.updateStatus(*status)
				if updated {
					p.updateCounter(begin, end)
				}
			case HasSCVoteMsg:
				p.counter.SetHasVote(msg.data.(*HasSCVoteData))
			}
		}
	}
}

func (p *peer) updateStatus(msg SCStatus) (uint64, uint64, bool) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if msg.Candidate < p.scStatus.Candidate || msg.Height < p.scStatus.Height {
		return 0, 0, false
	}

	p.Log().Debug("Peer set newer HR",
		"current", p.statusString(),
		"newer", fmt.Sprintf("%d/%d/%d/%d", msg.Fz, msg.Proof, msg.Candidate, msg.Height))

	beforeC := p.scStatus.Candidate
	p.scStatus = msg

	return beforeC, p.scStatus.Candidate, true
}

func (p *peer) updateCounter(begin, end uint64) {
	p.counter.Remove(begin, end)
	p.Log().Debug("remote counter", "begin", begin, "end", end)
}

func (p *peer) PickAndSend(votes []*types.StampingVote) error {
	if len(votes) == 0 {
		return fmt.Errorf("has no votes")
	}

	vote := p.counter.RandomNotIn(votes)
	if vote == nil {
		return fmt.Errorf("has no vote to be selected, counter:%s", p.counter.Print(votes[0].Height))
	}

	if err := p.SendSCVote(vote); err == nil {
	} // else {} ??

	return nil
}

func (p *peer) PickBuildingAndSend(votes *StampingVotes) error {
	if votes == nil || len(votes.votes) == 0 {
		return fmt.Errorf("has no building votes")
	}

	for _, vote := range votes.votes {
		if !p.counter.HasVote(vote) {
			if err := p.SendSCVote(vote); err == nil {
			} // else {} ??
			return nil
		}
	}

	return fmt.Errorf("selected no vote")
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
			p.SendSCVote(vote)
		}
	}
}

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
