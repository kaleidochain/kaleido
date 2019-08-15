package stamping

import (
	"math/rand"
	"testing"
)

type block struct {
	header *Header
	fc     *FinalCertificate
}

const (
	newBlockEvent = 1
)

type event struct {
	Height uint64
	Type   uint
}

func makeBlockGenerator(config *Config, chain *Chain, maxHeight uint64) <-chan block {
	ch := make(chan block)
	go func() {
		parent := genesisHeader
		for height := uint64(1); height < maxHeight; height++ {
			header := NewHeader(height, parent)
			fc := NewFinalCertificate(height, parent)
			ch <- block{header, fc}
		}

		close(ch)
	}()
	return ch
}

func makeStampingGenerator(config *Config, chain *Chain, eventCh <-chan event) <-chan *StampingCertificate {
	ch := make(chan *StampingCertificate)
	go func() {
		for e := range eventCh {
			if e.Height <= config.B {
				continue
			}

			proofHeader := chain.Header(e.Height - config.B)
			if rand.Intn(100) < config.Probability {
				s := NewStampingCertificate(e.Height, proofHeader)
				ch <- s
			}
		}
		close(ch)
	}()
	return ch
}

func TestNewChain(t *testing.T) {
	const maxHeight = 300
	chain := NewChain()

	blockCh := makeBlockGenerator(defaultConfig, chain, maxHeight)
	eventCh := make(chan event)
	stampingCh := makeStampingGenerator(defaultConfig, chain, eventCh)

ForLoop:
	for {
		select {
		case block, ok := <-blockCh:
			if ok {
				chain.AddBlock(block.header, block.fc)
				eventCh <- event{Height: block.header.Height, Type: newBlockEvent}
			} else {
				close(eventCh)
			}
		case s, ok := <-stampingCh:
			if ok {
				chain.AddStampingCertificate(s)
			} else {
				break ForLoop
			}
		}
	}

	chain.Print()
}
