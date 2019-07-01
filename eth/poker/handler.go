package poker

import (
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/log"

	"github.com/kaleidochain/kaleido/p2p/enode"

	gutils "github.com/kaleidochain/kaleido/gameutils"
	"github.com/kaleidochain/kaleido/node"
	"github.com/kaleidochain/kaleido/p2p"
)

type ProtocolManager struct {
	networkId uint64

	maxPeers     int
	peers        *PeerSet
	SubProtocols []p2p.Protocol
	node         *node.Node
	wg           sync.WaitGroup
}

func NewProtocolManager(networkId uint64, node *node.Node) (*ProtocolManager, error) {
	// Create the protocol manager with the base fields
	manager := &ProtocolManager{
		networkId: networkId,
		peers:     NewPeerSet(),
		node:      node,
	}

	// Initiate a sub-protocol for every implemented version we can handle
	manager.SubProtocols = make([]p2p.Protocol, 0, len(ProtocolVersions))
	for i, version := range ProtocolVersions {
		version := version // Closure for the run
		manager.SubProtocols = append(manager.SubProtocols, p2p.Protocol{
			Name:    ProtocolName,
			Version: version,
			Length:  ProtocolLengths[i],
			Run: func(p *p2p.Peer, rw p2p.MsgReadWriter) error {
				peer := manager.newPeer(int(version), p, rw)
				return manager.handle(peer)
			},
			PeerInfo: func(id enode.ID) interface{} {
				if p := manager.peers.Peer(idKey(id)); p != nil {
					return p.Info()
				}
				return nil
			},
		})
	}
	if len(manager.SubProtocols) == 0 {
		return nil, errIncompatibleConfig
	}
	return manager, nil
}

func (pm *ProtocolManager) removePeer(id string) {
	// Short circuit if the peer was already removed
	peer := pm.peers.Peer(id)
	if peer == nil {
		return
	}

	// Unregister the peer from the downloader and Ethereum peer set
	if err := pm.peers.Unregister(id); err != nil {
		log.Debug("Peer removal failed", "peer", id, "err", err)
	}
	// Hard disconnect at the networking layer
	peer.Peer.Disconnect(p2p.DiscUselessPeer)
}

func (pm *ProtocolManager) Start(maxPeers int) {
	pm.maxPeers = maxPeers
}

func (pm *ProtocolManager) Stop() {
	// Disconnect existing sessions.
	// This also closes the gate for any new registrations on the peer set.
	// sessions which are already established but not added to pm.peers yet
	// will exit when they try to register.
	pm.peers.Close()

	// Wait for all peer handler goroutines and the loops to come down.
	pm.wg.Wait()
}

func (pm *ProtocolManager) newPeer(pv int, p *p2p.Peer, rw p2p.MsgReadWriter) *Peer {
	return NewPeer(pv, p, rw)
}

// handle is the callback invoked to manage the life cycle of an eth peer. When
// this function terminates, the peer is disconnected.
func (pm *ProtocolManager) handle(p *Peer) error {
	// Ignore maxPeers if this is a trusted peer
	if pm.peers.Len() >= pm.maxPeers {
		fmt.Println("ret")
		return p2p.DiscTooManyPeers
	}
	p.Log().Debug("poker peer connected", "name", p.Name())

	if err := p.Handshake(pm.networkId); err != nil {
		p.Log().Debug("poker handshake failed", "err", err)
		return err
	}
	// Register the peer locally
	if err := pm.peers.Register(p); err != nil {
		p.Log().Error("poker peer registration failed", "err", err)
		return err
	}
	defer pm.removePeer(p.ID)

	// main loop. handle incoming messages.
	for {
		if err := pm.handleMsg(p); err != nil {
			p.Log().Debug("poker message handling failed", "err", err)
			return err
		}
	}
}

// handleMsg is invoked whenever an inbound message is received from a remote
// peer. The remote connection is torn down upon returning any error.
func (pm *ProtocolManager) handleMsg(p *Peer) error {
	// Read the next message from the remote peer, and ensure it's fully consumed
	msg, err := p.RW.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Size > ProtocolMaxMsgSize {
		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, ProtocolMaxMsgSize)
	}
	defer msg.Discard()

	p.Log().Debug("ProtocolManager handleMsg", "msg.Code = ", msg.Code)
	fmt.Println("ProtocolManager handleMsg", "msg.Code = ", msg.Code)
	// Handle the message depending on its contents
	switch {
	case msg.Code == gutils.StatusMsgCode:
		// Status messages should never arrive after the handshake
		return errResp(ErrExtraStatusMsg, "uncontrolled status message")

	case msg.Code == gutils.LeaveMsgCode:
		var query gutils.LeaveData
		if err := msg.Decode(&query); err != nil {
			return errResp(ErrDecode, "%v: %v", msg, err)
		}

	default:
		return errResp(ErrInvalidMsgCode, "%v", msg.Code)
	}
	return nil
}
