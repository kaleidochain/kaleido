package poker

import (
	"log"

	"github.com/kaleidochain/kaleido/node"
	"github.com/kaleidochain/kaleido/p2p"
	"github.com/kaleidochain/kaleido/rpc"
)

func init() {
	log.SetFlags(log.Lshortfile | log.Ldate | log.Lmicroseconds)
}

type Poker struct {
	protocolManager *ProtocolManager
}

//New creates a new Inter object
func New(ni uint64, stack *node.Node) (*Poker, error) {
	poker := &Poker{}

	pm, err := NewProtocolManager(ni, stack)
	if err != nil {
		return nil, err
	}
	poker.protocolManager = pm
	return poker, nil
}

// Protocols implements node.Service, returning all the currently configured
// network protocols to start.
func (s *Poker) Protocols() []p2p.Protocol {
	return s.protocolManager.SubProtocols
}

// Start implements node.Service, starting all internal goroutines needed by the
// Ethereum protocol implementation.
func (s *Poker) Start(srvr *p2p.Server) error {
	// Figure out a max peers count based on the server limits
	maxPeers := srvr.MaxPeers

	// Start the networking layer and the light server if requested
	s.protocolManager.Start(maxPeers)

	return nil
}

// Stop implements node.Service, terminating all internal goroutines used by the
// Ethereum protocol.
func (s *Poker) Stop() error {
	s.protocolManager.Stop()

	return nil
}

// APIs returns the collection of RPC services the ethereum package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *Poker) APIs() []rpc.API {
	return []rpc.API{}
}
