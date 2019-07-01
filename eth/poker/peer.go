package poker

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/kaleidochain/kaleido/p2p/enode"

	"github.com/kaleidochain/kaleido/common"

	gutils "github.com/kaleidochain/kaleido/gameutils"
	"github.com/kaleidochain/kaleido/p2p"
)

var (
	errClosed            = errors.New("peer set is closed")
	errAlreadyRegistered = errors.New("peer is already registered")
	errNotRegistered     = errors.New("peer is not registered")
)

const (
	handshakeTimeout = 5 * time.Second
)

func errResp(code errCode, format string, v ...interface{}) error {
	return fmt.Errorf("%v - %v", code, fmt.Sprintf(format, v...))
}

var errIncompatibleConfig = errors.New("incompatible configuration")

// PeerInfo represents a short summary of the Ethereum sub-protocol metadata known
// about a connected peer.
type PeerInfo struct {
	Version int `json:"version"`
}

type Peer struct {
	ID string

	*p2p.Peer
	RW p2p.MsgReadWriter

	version  int         // Protocol version negotiated
	forkDrop *time.Timer // Timed connection dropper if forks aren't validated in time

	head common.Hash
	td   *big.Int
	lock sync.RWMutex
}

func idKey(id enode.ID) string {
	return fmt.Sprintf("%x", id.Bytes()[:8])
}

func NewPeer(version int, p *p2p.Peer, rw p2p.MsgReadWriter) *Peer {
	return &Peer{
		Peer:    p,
		RW:      rw,
		version: version,
		ID:      idKey(p.ID()),
	}
}

// Info gathers and returns a collection of metadata known about a peer.
func (p *Peer) Info() *PeerInfo {
	return &PeerInfo{
		Version: p.version,
	}
}

// Handshake executes the eth protocol handshake, negotiating version number,
// network IDs, difficulties, head and genesis blocks.
func (p *Peer) Handshake(network uint64) error {
	// Send out own handshake in a new thread
	errc := make(chan error, 2)
	var status gutils.StatusData // safe to read after two values have been received from errc

	go func() {
		errc <- p2p.Send(p.RW, gutils.StatusMsgCode, &gutils.StatusData{
			ProtocolVersion: uint32(p.version),
			NetworkId:       network,
		})
	}()
	go func() {
		errc <- p.readStatus(network, &status)
	}()
	timeout := time.NewTimer(handshakeTimeout)
	defer timeout.Stop()
	for i := 0; i < 2; i++ {
		select {
		case err := <-errc:
			if err != nil {
				return err
			}
		case <-timeout.C:
			return p2p.DiscReadTimeout
		}
	}
	return nil
}

func (p *Peer) readStatus(network uint64, status *gutils.StatusData) (err error) {
	msg, err := p.RW.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Code != gutils.StatusMsgCode {
		return errResp(ErrNoStatusMsg, "first msg has code %x (!= %x)", msg.Code, gutils.StatusMsgCode)
	}
	if msg.Size > ProtocolMaxMsgSize {
		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, ProtocolMaxMsgSize)
	}
	// Decode the handshake and make sure everything matches
	if err := msg.Decode(&status); err != nil {
		return errResp(ErrDecode, "msg %v: %v", msg, err)
	}
	if status.NetworkId != network {
		return errResp(ErrNetworkIdMismatch, "%d (!= %d)", status.NetworkId, network)
	}
	if int(status.ProtocolVersion) != p.version {
		return errResp(ErrProtocolVersionMismatch, "%d (!= %d)", status.ProtocolVersion, p.version)
	}
	return nil
}

func (p *Peer) SendMsg(msgcode uint64, data interface{}) error {
	return p2p.Send(p.RW, msgcode, data)
}

// String implements fmt.Stringer.
func (p *Peer) String() string {
	return fmt.Sprintf("Peer %s [%s]", p.ID,
		fmt.Sprintf("poker/%2d", p.version),
	)
}

// peerSet represents the collection of active peers currently participating in
// the Ethereum sub-protocol.
type PeerSet struct {
	peers  map[string]*Peer
	lock   sync.RWMutex
	closed bool
}

// newPeerSet creates a new peer set to track the active participants.
func NewPeerSet() *PeerSet {
	return &PeerSet{
		peers: make(map[string]*Peer),
	}
}

// Register injects a new peer into the working set, or returns an error if the
// peer is already known.
func (ps *PeerSet) Register(p *Peer) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	if ps.closed {
		return errClosed
	}
	if _, ok := ps.peers[p.ID]; ok {
		return errAlreadyRegistered
	}
	ps.peers[p.ID] = p
	return nil
}

// Unregister removes a remote peer from the active set, disabling any further
// actions to/from that particular entity.
func (ps *PeerSet) Unregister(id string) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	if _, ok := ps.peers[id]; !ok {
		return errNotRegistered
	}
	delete(ps.peers, id)
	return nil
}

// Peer retrieves the registered peer with the given id.
func (ps *PeerSet) Peer(id string) *Peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return ps.peers[id]
}

// Peer retrieves the registered peer with the given id.
func (ps *PeerSet) CenterPeer() *Peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	var p *Peer = nil
	for _, v := range ps.peers {
		p = v
		break
	}
	return p
}

// Len returns if the current number of peers in the set.
func (ps *PeerSet) Len() int {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return len(ps.peers)
}

// Close disconnects all peers.
// No new peers can be registered after Close has returned.
func (ps *PeerSet) Close() {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	for _, p := range ps.peers {
		p.Disconnect(p2p.DiscQuitting)
	}
	ps.closed = true
}
