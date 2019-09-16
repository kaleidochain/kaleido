package leap

import (
	"io"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/consensus"
	"github.com/kaleidochain/kaleido/core"
	"github.com/kaleidochain/kaleido/core/types"
	"github.com/kaleidochain/kaleido/event"
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
	eth     Backend
	config  *params.ChainConfig
	scChain *SCChain

	SubProtocols []p2p.Protocol
	peers        *peerSet

	// wait group is used for graceful shutdowns during downloading
	// and processing
	wg sync.WaitGroup
}

func NewProtocolManager(eth Backend, chain *SCChain, config *params.ChainConfig, mux *event.TypeMux, engine consensus.Engine) *ProtocolManager {
	pm := &ProtocolManager{
		eth:    eth,
		config: config,
		peers:  newPeerSet(),
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
				peer := newPeer(p, rw)
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
	err := p.Handshake(pm.HR())
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
	go pm.gossipDataLoop(p)

	// main loop
	for {
		if err := pm.handleLoop(p); err != nil {
			if err == io.EOF {
				p.Log().Debug("peer closed")
			} else {
				p.Log().Error("Algorand handleLoop fail", "err", err)
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
	/*
		case core.HandshakeMsg:
			// Handshake messages should never arrive after the handshake
			return errResp(ErrExtraHandshakeMsg, "uncontrolled handshake message")

		case core.StatusMsg:
			var status core.StatusData
			if err := msg.Decode(&status); err != nil {
				return errResp(ErrDecode, "msg %v: %v", msg, err)
			}
			p.UpdateHR(status.Height, status.Round)

		case core.ProposalLeaderMsg:
			var data core.ProposalLeaderData
			if err := msg.Decode(&data); err != nil {
				return errResp(ErrDecode, "msg %v: %v", msg, err)
			}
			p.UpdateHR(data.Height, data.Round)
			p.SetHasProposalValue(data.ToHasProposalData())
			pm.ctx.OnReceive(core.ProposalLeaderMsg, &data, p.String())

		case core.ProposalBlockMsg:
			var data core.ProposalBlockData
			if err := msg.Decode(&data); err != nil {
				return errResp(ErrDecode, "msg %v: %v", msg, err)
			}
			p.UpdateHR(data.Height, data.Round)
			p.SetHasProposalBlock(data.ToHasProposalData())
			pm.ctx.OnReceive(core.ProposalBlockMsg, &data, p.String())

		case core.VoteMsg:
			var data core.VoteData
			if err := msg.Decode(&data); err != nil {
				return errResp(ErrDecode, "msg %v: %v", msg, err)
			}
			p.UpdateHR(data.Height, data.Round)
			p.SetHasVote(core.ToHasVote(&data))
			pm.ctx.OnReceive(core.VoteMsg, &data, p.String())

		case core.HasVoteMsg:
			var data core.HasVoteData
			if err := msg.Decode(&data); err != nil {
				return errResp(ErrDecode, "msg %v: %v", msg, err)
			}
			p.UpdateHR(data.Height, data.Round)
			p.SetHasVote(&data)

		case core.HasProposalLeaderMsg:
			var data core.HasProposalData
			if err := msg.Decode(&data); err != nil {
				return errResp(ErrDecode, "msg %v: %v", msg, err)
			}
			p.UpdateHR(data.Height, data.Round)
			p.SetHasProposalValue(&data)

		case core.HasProposalBlockMsg:
			var data core.HasProposalData
			if err := msg.Decode(&data); err != nil {
				return errResp(ErrDecode, "msg %v: %v", msg, err)
			}
			p.UpdateHR(data.Height, data.Round)
			p.SetHasProposalBlock(&data)
	*/

	default:
		return errResp(ErrInvalidMsgCode, "%v", msg.Code)
	}

	return nil
}

func (pm *ProtocolManager) Broadcast(code uint64, data interface{}) {
	switch code {
	/*
		case core.StatusMsg:
			fallthrough
		case core.HasVoteMsg:
			fallthrough
		case core.HasProposalLeaderMsg:
			fallthrough
		case core.HasProposalBlockMsg:
			pm.peers.ForEach(func(p *peer) {
				p.SendMsgAsync(code, data)
			})
		case core.ProposalLeaderMsg:
			msg := data.(*core.ProposalLeaderData)
			pm.peers.ForEach(func(p *peer) {
				if p.height == msg.Height { // fast check without lock
					p.SendProposalLeaderAsync(msg)
				}
			})
		case core.VoteMsg:
			msg := data.(*core.VoteData)
			pm.peers.ForEach(func(p *peer) {
				if p.height == msg.Height { // fast check without lock
					p.SendVoteAsync(msg)
				}
			})
	*/
	default:
		log.Error("Algorand broadcast ignore unknown message",
			"code", CodeToString[code], "data", data)
	}
}

func (pm *ProtocolManager) gossipVotesLoop(p *peer) {
	pm.wg.Add(1)
	defer pm.wg.Done()

	p.Log().Debug("gossipVotesLoop start")
	defer p.Log().Debug("gossipVotesLoop exit")

	suppressLogForHeight := uint64(0)
	suppressLogForRound := time.Now()

	needSleep := false
	for {
		if true || needSleep {
			time.Sleep(pm.GossipInterval())
		}
		needSleep = false

		if p.IsClosed() {
			return
		}

		peerHeight, peerRound, _ := p.HR()
		selfHeight, selfRound := pm.HR()

		if peerHeight == 0 || selfHeight == 0 { // gossip not enabled
			needSleep = true
			continue
		}

		if peerHeight > selfHeight { // we are late
			if suppressLogForHeight == 0 || suppressLogForHeight != peerHeight {
				suppressLogForHeight = peerHeight
				p.Log().Trace("gossipVotesLoop I am late, waiting sync",
					"peer", peerHeight, "self", selfHeight)
			}
			needSleep = true
			continue
		}

		if peerHeight+gossipMaxHeightDiff < selfHeight {
			needSleep = true
			continue
		}

		if peerHeight < selfHeight {
			var roundVoteSet *core.RoundVoteSet = nil
			var round uint32 = 0
			if peerHeight+1 == selfHeight {
				roundVoteSet, round = pm.getParentRoundVoteSet(peerHeight)
			} else {
				if certVotes, certVoteRound := pm.getCertVotesByHeight(peerHeight); certVotes != nil {
					threshold, _ := core.GetCommitteeNumber(peerHeight, types.RoundStep3Certifying)
					roundVoteSet = core.NewRoundVoteSetFromCertificates(certVotes, threshold)
					round = certVoteRound
				}
			}

			if sent := p.PickAndSend(roundVoteSet, peerHeight, round); sent {
				p.Log().Debug("gossipVotesLoop peer is late on Height, send certificates(cached) to peer",
					"peer", peerHeight, "self", selfHeight)
				continue
			}

			needSleep = true
			continue
		}

		// peerHeight == selfHeight

		if peerRound < selfRound {
			// pick and send next vote of peerRound
			sent := p.PickNextVoteAndSend(pm.RoundVoteSet(selfHeight, selfRound-1), selfHeight, selfRound-1)
			if sent {
				p.Log().Debug("gossipVotesLoop peer is late on Round, send next vote to peer",
					"peer", peerRound, "self", selfRound)
				continue
			}

			needSleep = true
			continue
		}

		if peerRound > selfRound { // we are late
			if time.Now().Sub(suppressLogForRound) > 10*time.Second {
				suppressLogForRound = time.Now()
				p.Log().Trace("gossipVotesLoop I am late on Round, waiting for next vote of current height from peer",
					"peer", peerRound, "self", selfRound)
			}
			needSleep = true
			continue
		}

		// here peerRound == selfRound
		sent := p.PickAndSend(pm.RoundVoteSet(selfHeight, selfRound), selfHeight, selfRound)
		if sent {
			p.Log().Debug("gossipVotesLoop pick a vote of current height to send", "height", selfHeight, "round", peerRound)

			continue
		}

		if peerRound > 1 {
			sent := p.PickNextVoteAndSend(pm.RoundVoteSet(selfHeight, selfRound-1), selfHeight, selfRound-1)
			if sent {
				p.Log().Debug("gossipVotesLoop pick a previous NextVote of current height to send", "height", selfHeight, "round", peerRound-1)

				continue
			}
		}

		if time.Now().Sub(suppressLogForRound) > 10*time.Second {
			suppressLogForRound = time.Now()
			p.Log().Trace("gossipVotesLoop no vote to send, sleep a while")
		}
		needSleep = true
	}
}

func (pm *ProtocolManager) HR() (uint64, uint32) {
	return pm.ctx.HR()
}

func (pm *ProtocolManager) GossipInterval() time.Duration {
	return pm.eth.GossipInterval()
}

func (pm *ProtocolManager) RoundVoteSet(height uint64, round uint32) *core.RoundVoteSet {
	return pm.ctx.RoundVoteSet(height, round)
}

func (pm *ProtocolManager) GetLeaderProposalValue(height uint64, round uint32) *core.ProposalLeaderData {
	return pm.ctx.GetProposalLeader(height, round)
}

func (pm *ProtocolManager) GetProposalBlock(height uint64, value common.Hash) *core.ProposalBlockData {
	return pm.ctx.GetProposalBlock(height, value)
}

func (pm *ProtocolManager) getProposalBlockByHeight(height uint64) *core.ProposalBlockData {
	block := pm.eth.BlockChain().GetBlockByNumber(height)
	if block == nil {
		return nil
	}

	// remove Certificate from header
	headerNoCert := block.Header()
	headerNoCert.Certificate = new(types.Certificate)

	certificate := block.Certificate()
	data := core.NewProposalBlockDataFromProposalStorage(&certificate.Proposal, block.WithSeal(headerNoCert))
	return data
}

func (pm *ProtocolManager) getCertVotesByHeight(height uint64) ([]*core.VoteData, uint32) {
	header := pm.eth.BlockChain().GetHeaderByNumber(height)
	if header == nil {
		return nil, types.BadRound
	}

	certificate := header.Certificate
	votes := make([]*core.VoteData, len(certificate.CertVoteSet))
	for i, certVote := range certificate.CertVoteSet {
		if certVote == nil {
			continue
		}

		votes[i] = core.NewVoteDataFromCertVoteStorage(certVote, height, certificate.Round, certificate.Value)
	}
	return votes, certificate.Round
}

func (pm *ProtocolManager) getParentProposalBlockData(height uint64) *core.ProposalBlockData {
	return pm.ctx.GetParentProposalBlockData(height)
}

func (pm *ProtocolManager) getParentRoundVoteSet(height uint64) (*core.RoundVoteSet, uint32) {
	return pm.ctx.GetParentRoundVoteSet(height)
}

func (pm *ProtocolManager) Sub(marker []byte, duration time.Duration) {
}

func (pm *ProtocolManager) UnSub(marker []byte) {
}

func (pm *ProtocolManager) Request(marker []byte, blockMarker common.Hash) {
}

func (pm *ProtocolManager) Publish(marker []byte, msgCode uint64, data interface{}) {
}
