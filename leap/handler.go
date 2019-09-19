package leap

import (
	"io"
	"sync"
	"time"

	"github.com/kaleidochain/kaleido/core/types"

	"github.com/ethereum/go-ethereum/log"
	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/consensus"
	"github.com/kaleidochain/kaleido/core"
	"github.com/kaleidochain/kaleido/p2p"
	"github.com/kaleidochain/kaleido/p2p/enode"
	"github.com/kaleidochain/kaleido/params"
)

// Backend wraps all methods required for mining.
type Backend interface {
	BlockChain() *core.BlockChain
	TxPool() *core.TxPool
	GossipInterval() time.Duration
}

type NodeInfo struct {
	// TODO: we should define our own NodeInfo struct
}

type ProtocolManager struct {
	eth       Backend
	config    *params.ChainConfig
	networkId uint64
	scChain   *SCChain

	SubProtocols []p2p.Protocol
	peers        *peerSet

	// wait group is used for graceful shutdowns during downloading
	// and processing
	wg sync.WaitGroup
}

func NewProtocolManager(eth Backend, chain *SCChain, config *params.ChainConfig, engine consensus.Engine, networkId uint64) *ProtocolManager {
	pm := &ProtocolManager{
		eth:       eth,
		config:    config,
		networkId: networkId,
		scChain:   chain,
		peers:     newPeerSet(),
	}

	log.Info("Initialising Leap protocol", "versions", ProtocolVersions)

	// Initiate a sub-protocol for every implemented version we can runPeer
	pm.SubProtocols = make([]p2p.Protocol, 0, len(ProtocolVersions))
	for i, version := range ProtocolVersions {
		// initialise the protocol
		version := version // Closure for the run
		pm.SubProtocols = append(pm.SubProtocols, p2p.Protocol{
			Name:    ProtocolName,
			Version: version,
			Length:  ProtocolLengths[i],
			Run: func(p *p2p.Peer, rw p2p.MsgReadWriter) error {
				log.Info("New leap peer connected", "version", version)
				peer := newPeer(uint32(version), p, rw)
				pm.wg.Add(1)
				defer pm.wg.Done()
				return pm.runPeer(peer)
			},
			NodeInfo: func() interface{} {
				return pm.NodeInfo()
			},
			PeerInfo: func(id enode.ID) interface{} {
				if p := pm.peers.Peer(id); p != nil {
					return p.Info()
				}
				return nil
			},
		})
	}

	return pm
}

// NodeInfo returns metadata about the host node
func (pm *ProtocolManager) NodeInfo() *NodeInfo {
	return &NodeInfo{}
}

func (pm *ProtocolManager) runPeer(p *peer) error {
	// first update HR to bootstrap gossip
	// handshake must be done at first
	err := p.Handshake(pm.networkId, pm.eth.BlockChain().Genesis().Hash(), pm.scChain.ChainStatus())
	if err != nil {
		if err == io.EOF {
			p.Log().Debug("peer closed on handshake")
		} else {
			p.Log().Error("handshake failed", "err", err)
		}
		return err
	}

	if err := pm.peers.Register(p); err != nil {
		p.Log().Error("Leap register peer fail", "err", err)
		return err
	}
	go p.broadcaster()
	defer pm.peers.Unregister(p)
	go pm.gossipVotesLoop(p)

	// main loop
	for {
		if err := pm.handleLoop(p); err != nil {
			if err == io.EOF {
				p.Log().Debug("peer closed")
			} else {
				p.Log().Error("Leap handleLoop fail", "err", err)
			}
			return err
		}
	}
}

func (pm *ProtocolManager) handleLoop(p *peer) error {
	// Read the next message from the remote peer, and ensure it's fully consumed
	msg, err := p.rw.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Size > ProtocolMaxMsgSize {
		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, ProtocolMaxMsgSize)
	}
	defer msg.Discard()

	switch msg.Code {
	case HandshakeMsg:
		// Handshake messages should never arrive after the handshake
		return errResp(ErrExtraHandshakeMsg, "uncontrolled handshake message")

	case StampingStatusMsg:
		var status SCStatus
		if err := msg.Decode(&status); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.updateStatus(status)

	case StampingVoteMsg:
		var data types.StampingVote
		if err := msg.Decode(&data); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.SetHasVote(ToHasSCVoteData(&data))
		pm.scChain.OnReceive(StampingVoteMsg, &data, p.String())
	case HasSCVoteMsg:
		var data *HasSCVoteData
		if err := msg.Decode(&data); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.counter.SetHasVote(data)
	default:
		return errResp(ErrInvalidMsgCode, "%v", msg.Code)
	}

	return nil
}

func (pm *ProtocolManager) Broadcast(code uint64, data interface{}) {
	switch code {
	case HasSCVoteMsg:
		fallthrough
	case StampingStatusMsg:
		pm.peers.ForEach(func(p *peer) {
			p.SendMsgAsync(code, data)
		})
	case StampingVoteMsg:
		vote := data.(*types.StampingVote)
		pm.peers.ForEach(func(p *peer) {
			if p.ChainStatus().Height >= vote.Height {
				p.SendStampingVoteAsync(vote)
			}
		})
	default:
		log.Error("Leap broadcast ignore unknown message",
			"code", CodeToString[code], "data", data)
	}
}

func (pm *ProtocolManager) gossipVotesLoop(p *peer) {
	pm.wg.Add(1)
	defer pm.wg.Done()

	p.Log().Debug("gossipVotesLoop start")
	defer p.Log().Debug("gossipVotesLoop exit")

	needSleep := false
	for {
		if true || needSleep {
			time.Sleep(pm.GossipInterval())
		}
		needSleep = false

		if p.IsClosed() {
			return
		}

		scStatus := pm.scChain.ChainStatus()
		peerScStatus := p.ChainStatus()

		p.Log().Trace("gossip begin", "status", pm.scChain.StatusString())

		if scStatus.Height < peerScStatus.Candidate || scStatus.Candidate > peerScStatus.Height {
			needSleep = true
			continue
		}

		if peerScStatus.Candidate < scStatus.Candidate {
			if pm.scChain.pickFrozenSCVoteToPeer(peerScStatus.Candidate, scStatus.Candidate, p) {
				needSleep = true
				continue
			}
		}

		//(C, H]
		windowFloor := MaxUint64(scStatus.Candidate, peerScStatus.Candidate)
		windowCeil := MinUint64(scStatus.Height, peerScStatus.Height)
		if pm.scChain.PickBuildingSCVoteToPeer(windowFloor, windowCeil, p) {
			needSleep = true
			continue
		}

		needSleep = true
	}
}

func (pm *ProtocolManager) GetBestPeer() *peer {
	return pm.peers.GetBestPeer()
}

func (pm *ProtocolManager) GetArchivePeer() *peer {
	return nil
}

func (pm *ProtocolManager) GossipInterval() time.Duration {
	return pm.eth.GossipInterval()
}

func (pm *ProtocolManager) Sub(marker []byte, duration time.Duration) {
}

func (pm *ProtocolManager) UnSub(marker []byte) {
}

func (pm *ProtocolManager) Request(marker []byte, blockMarker common.Hash) {
}

func (pm *ProtocolManager) Publish(marker []byte, msgCode uint64, data interface{}) {
}
