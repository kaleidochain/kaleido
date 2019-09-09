package stamping

import (
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/log"
)

type peer struct {
	id string

	height   uint64
	scStatus SCStatus
	counter  *HeightVoteSet

	closeChan chan struct{}

	mutex sync.RWMutex

	recvChan chan message
	sendChan chan message

	chain *Chain
}

func newPeer(id string) *peer {
	return &peer{
		id:        id,
		counter:   NewHeightVoteSet(),
		closeChan: make(chan struct{}),
		recvChan:  make(chan message, msgChanSize),
		sendChan:  make(chan message, msgChanSize),
	}
}

func (p *peer) setChain(chain *Chain) {
	p.chain = chain
}

func (p *peer) Close() {
	close(p.closeChan)
}

func (p *peer) Log() log.Logger {
	return log.New("pid", p.id, "HR", p.statusString())
}

func (p *peer) statusString() string {
	return fmt.Sprintf("%d/%d/%d/%d", p.scStatus.Fz, p.scStatus.Proof, p.scStatus.Candidate, p.height)
}

func (p *peer) ChainStatus() StatusMsg {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return StatusMsg{
		SCStatus: p.scStatus,
		Height:   p.height,
	}
}

func (p *peer) string() string {
	return fmt.Sprintf("%s-%d-%d-%d-%d", p.id, p.scStatus.Fz, p.scStatus.Proof, p.scStatus.Candidate, p.height)
}

func (p *peer) SendSCVote(vote *StampingVote) error {
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

func (p *peer) sendVoteAndSetHasVoteNoLock(vote *StampingVote) {
	p.send(message{
		code: StampingVoteMsg,
		data: vote,
		from: p.id,
	})

	p.counter.SetHasVote(vote.ToHasSCVoteData())
	p.Log().Trace("SendVote OK", "vote", vote)
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

func (p *peer) handleMsg() error {
	for {
		select {
		case msg := <-p.recvChan:
			switch msg.code {
			case StampingVoteMsg:
				p.counter.SetHasVote(msg.data.(*StampingVote).ToHasSCVoteData())
				p.chain.OnReceive(StampingVoteMsg, msg.data, p.string())
			case StampingStatusMsg:
				status := msg.data.(*StatusMsg)
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

func (p *peer) updateStatus(msg StatusMsg) (uint64, uint64, bool) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if msg.Candidate < p.scStatus.Candidate || msg.Height < p.height {
		return 0, 0, false
	}

	p.Log().Debug("Peer set newer HR",
		"current", p.statusString(),
		"newer", fmt.Sprintf("%d/%d/%d/%d", msg.Fz, msg.Proof, msg.Candidate, msg.Height))

	beforeC := p.scStatus.Candidate
	p.scStatus = msg.SCStatus
	p.height = msg.Height

	return beforeC, p.scStatus.Candidate, true
}

func (p *peer) updateCounter(begin, end uint64) {
	p.counter.Remove(begin, end)
	p.Log().Debug("remote counter", "begin", begin, "end", end)
}

func (p *peer) PickAndSend(votes []*StampingVote) error {
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

func makePairPeer(c1, c2 *Chain) {
	p1 := newPeer(c1.name + "-" + c2.name)
	p1.setChain(c2)
	p1.scStatus = c2.scStatus
	p1.height = c2.currentHeight

	p2 := newPeer(c2.name + "-" + c1.name)
	p2.setChain(c1)
	p2.scStatus = c1.scStatus
	p2.height = c1.currentHeight

	c1.AddPeer(p2)
	c2.AddPeer(p1)

	go func() {
		for {
			select {
			case msg := <-p1.sendChan:
				p2.recvChan <- msg
			case msg := <-p2.sendChan:
				p1.recvChan <- msg
			case <-p1.closeChan:
				p1.Log().Info("Closed")
				return
			case <-p2.closeChan:
				p2.Log().Info("Closed")
				return
			}
		}
	}()
}
