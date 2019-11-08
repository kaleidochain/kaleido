package leap

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kaleidochain/kaleido/params"

	"github.com/ethereum/go-ethereum/log"
	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/core/types"
	"github.com/kaleidochain/kaleido/p2p"
	"github.com/kaleidochain/kaleido/p2p/enode"
)

var (
	errClosed            = errors.New("peer/peer set is closed")
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
	rw             p2p.MsgReadWriter
	closeChan      chan struct{}
	msgChan        chan message
	voteChan       chan *types.StampingVote
	breadcrumbChan chan *breadcrumb
	headersChan    chan []*types.Header
	blockChan      chan *blockData

	scStatus types.StampingStatus
	counter  *HeightVoteSet

	mutex sync.RWMutex

	chain *StampingChain
}

func newPeer(version uint32, p *p2p.Peer, rw p2p.MsgReadWriter) *peer {
	return &peer{
		id:             peerIdKey(p.ID()),
		Peer:           p,
		rw:             rw,
		counter:        NewHeightVoteSet(),
		closeChan:      make(chan struct{}),
		msgChan:        make(chan message, msgQueueSize),
		voteChan:       make(chan *types.StampingVote, msgQueueSize),
		breadcrumbChan: make(chan *breadcrumb, 1),
		headersChan:    make(chan []*types.Header, 1),
		blockChan:      make(chan *blockData, 1),
	}
}

func (p *peer) setChain(chain *StampingChain) {
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
	return log.New("pid", p.id, "ip", p.RemoteAddr().String(), "HR", p.statusString())
}

func (p *peer) statusString() string {
	return fmt.Sprintf("%d/%d/%d/%d", p.scStatus.Fz, p.scStatus.Proof, p.scStatus.Candidate, p.scStatus.Height)
}

func (p *peer) StatusString() string {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.statusString()
}

func (p *peer) ChainStatus() types.StampingStatus {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.scStatus
}

func (p *peer) string() string {
	return fmt.Sprintf("%s-%d-%d-%d-%d", p.id, p.scStatus.Fz, p.scStatus.Proof, p.scStatus.Candidate, p.scStatus.Height)
}

func (p *peer) Handshake(networkId uint64, genesis common.Hash, stampingConfig params.StampingConfig, status types.StampingStatus) error {
	// Send out own handshake in a new thread
	errCh := make(chan error, 2)
	var handshake HandshakeData // safe to read after two values have been received from errCh

	go func() {
		errCh <- p2p.Send(p.rw, HandshakeMsg, &HandshakeData{
			Version:   p.version,
			NetworkId: networkId,
			Genesis:   genesis,
			Stamping:  stampingConfig,
			StampingStatus: types.StampingStatus{
				Height:    status.Height,
				Candidate: status.Candidate,
				Proof:     status.Proof,
				Fz:        status.Fz,
			},
		})
	}()
	go func() {
		errCh <- p.readStatus(networkId, genesis, stampingConfig, &handshake)
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
	p.updateStatus(handshake.StampingStatus)
	return nil
}

func (p *peer) readStatus(networkId uint64, genesis common.Hash, stampingConfig params.StampingConfig, handshake *HandshakeData) (err error) {
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
	if handshake.Genesis != genesis {
		return errResp(ErrGenesisBlockMismatch, "%x (!= %x)", handshake.Genesis[:8], genesis[:8])
	}
	if handshake.Stamping != stampingConfig {
		return errResp(ErrStampingConfigMismatch, "%v (!= %v)", handshake.Stamping, stampingConfig)
	}
	if handshake.NetworkId != networkId {
		return errResp(ErrNetworkIdMismatch, "%d (!= %d)", handshake.NetworkId, networkId)
	}
	return nil
}

func (p *peer) SendStampingVote(vote *types.StampingVote) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if vote.Height <= p.scStatus.Candidate {
		p.Log().Trace("SendStampingVote too low", "vote", vote)
		return fmt.Errorf(fmt.Sprintf("SendStampingVote too low, peer status:%s, vote:%v", p.statusString(), vote))
	}

	if p.counter.HasVote(vote) {
		p.Log().Trace("SendStampingVote has vote", "vote", vote, "counter", p.counter.Print(vote.Height))
		return fmt.Errorf(fmt.Sprintf("SendStampingVote has vote, peer status:%s, vote:%v", p.statusString(), vote))
	}

	p.sendVoteAndSetHasVoteNoLock(vote)
	return nil
}

func (p *peer) sendVoteAndSetHasVoteNoLock(vote *types.StampingVote) {
	err := p2p.Send(p.rw, StampingVoteMsg, vote)
	if err != nil {
		p.Log().Debug("SendStampingVote fail", "vote", vote, "err", err)
		return
	}

	p.counter.SetHasVote(ToHasSCVoteData(vote))
	p.Log().Trace("SendStampingVote OK", "vote", vote)
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

func (p *peer) SendStatus(status *types.StampingStatus) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	err := p2p.Send(p.rw, StampingStatusMsg, status)
	if err != nil {
		p.Log().Debug("SendStampingVote fail", "status", status, "err", err)
		return
	}
}

func (p *peer) updateStatus(msg types.StampingStatus) (uint64, uint64, bool) {
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

	if err := p.SendStampingVote(vote); err == nil {
	} // else {} ??

	return nil
}

func (p *peer) PickBuildingAndSend(votes *StampingVotes) error {
	if votes == nil || len(votes.votes) == 0 {
		return fmt.Errorf("has no building votes")
	}

	for _, vote := range votes.votes {
		if !p.counter.HasVote(vote) {
			if err := p.SendStampingVote(vote); err == nil {
			} // else {} ??
			return nil
		}
	}

	return fmt.Errorf("selected no vote")
}

func (p *peer) SendMsgAsync(code uint64, data interface{}) {
	select {
	case p.msgChan <- message{code: code, data: data}:
	default:
		p.Log().Warn("msgChan full")
	}
}

func (p *peer) SendStampingVoteAsync(vote *types.StampingVote) {
	select {
	case p.voteChan <- vote:
	default:
		p.Log().Warn("voteChan full")
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
				p.Log().Debug("Send fail", "code", CodeToString[msg.code], "data", msg.data)
			} else {
				p.Log().Trace("Send sent OK", "code", CodeToString[msg.code], "data", msg.data)
			}
		case vote := <-p.voteChan:
			p.SendStampingVote(vote)
		}
	}
}

func (p *peer) Header(height uint64) *types.Header {
	headers := p.GetHeaders(height, height, true, true)
	if len(headers) == 0 {
		return nil
	}

	return headers[0]
}

func (p *peer) GetHeaders(begin, end uint64, forward, includeFc bool) (headers []*types.Header) {
	go func() {
		if err := p.requestHeaders(begin, end, forward, includeFc); err != nil {
			p.Log().Error("requestHeaders failed", "begin", begin, "end", end, "forward", forward, "err", err)
		}
	}()

	select {
	case headers = <-p.headersChan:
		return
	case <-p.closeChan:
		return
	}
}

func (p *peer) GetNextBreadcrumb(begin, end uint64, status types.StampingStatus) (bc *breadcrumb, err error) {
	go func() {
		if e := p.requestNextBreadcrumb(begin, end, status); e != nil {
			p.Log().Error("requestNextBreadcrumb failed", "begin", begin, "end", end, "err", e)
		}
	}()

	select {
	case bc = <-p.breadcrumbChan:
		return
	case <-p.closeChan:
		return nil, errClosed
	}
}

func (p *peer) GetBlockAndReceipts(hash common.Hash) (block *blockData, err error) {
	go func() {
		if e := p.requestBlockAndReceipts(hash); e != nil {
			p.Log().Error("requestBlockAndReceipts failed", "hash", hash, "err", e)
		}
	}()

	select {
	case block = <-p.blockChan:
		return
	case <-p.closeChan:
		return nil, errClosed
	}
}

func (p *peer) SendNextBreadcrumb(bc *breadcrumb) error {
	return p2p.Send(p.rw, NextBreadcrumbMsg, bc)
}

func (p *peer) SendHeaders(headers []*types.Header) error {
	return p2p.Send(p.rw, HeadersMsg, headers)
}

func (p *peer) SendBlockAndReceipts(block *types.Block, receiptes types.Receipts) error {
	return p2p.Send(p.rw, BodyAndReceiptsMsg, &blockData{Block: block, Receipts: receiptes})
}

func (p *peer) requestNextBreadcrumb(begin, end uint64, status types.StampingStatus) error {
	p.Log().Debug("Fetching next breadcrumb", "begin", begin, "end", end)
	return p2p.Send(p.rw, GetNextBreadcrumbMsg, &getNextBreadcrumbData{Begin: begin, End: end, Status: status})
}

func (p *peer) requestHeaders(begin, end uint64, forward, includeFc bool) error {
	p.Log().Debug("Fetching batch of headers", "begin", begin, "end", end, "forward", forward, "include", includeFc)
	return p2p.Send(p.rw, GetHeadersMsg, &getHeadersData{Begin: begin, End: end, Forward: forward, IncludeFc: includeFc})
}

func (p *peer) requestBlockAndReceipts(hash common.Hash) error {
	p.Log().Debug("Fetching block", "hash", hash.String())
	return p2p.Send(p.rw, GetBodyAndReceiptsMsg, hash)
}

func (p *peer) DeliverBCData(breadcrumbChan chan *breadcrumb, b *breadcrumb) {
	select {
	case breadcrumbChan <- b:
		return
	case <-p.closeChan:
		return
	}
}

func (p *peer) DeliverHeadersData(headersChan chan []*types.Header, headers []*types.Header) {
	select {
	case headersChan <- headers:
		return
	case <-p.closeChan:
		return
	}
}

func (p *peer) DeliverBlock(blockChan chan *blockData, block *blockData) {
	select {
	case blockChan <- block:
		return
	case <-p.closeChan:
		return
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

// Returm random peer
func (ps *peerSet) GetBestPeer() *peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	for _, p := range ps.peers {
		return p
	}

	return nil
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
