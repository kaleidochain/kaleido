package stamping

import (
	"fmt"
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

func makeBlockGenerator(chain *Chain, maxHeight uint64, eventCh chan<- event) {
	go func() {
		parent := genesisHeader
		for height := uint64(1); height < maxHeight; height++ {
			header := NewHeader(height, parent)
			fc := NewFinalCertificate(height, parent)
			parent = header

			chain.AddBlock(header, fc)
			eventCh <- event{Height: header.Height, Type: newBlockEvent}
		}
		close(eventCh)
	}()
}

func makeStampingGenerator(config *Config, chain *Chain, eventCh <-chan event) <-chan *StampingCertificate {
	ch := make(chan *StampingCertificate)
	go func() {
		for e := range eventCh {
			if e.Height <= config.B {
				continue
			}

			proofHeader := chain.header(e.Height - config.B)
			if rand.Intn(100) < config.Probability {
				fmt.Printf("chain.Header(%d)\n", e.Height-config.B)
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

	eventCh := make(chan event, 100)
	makeBlockGenerator(chain, maxHeight, eventCh)
	stampingCh := makeStampingGenerator(defaultConfig, chain, eventCh)

	for s := range stampingCh {
		chain.AddStampingCertificate(s)
	}

	chain.Print()
}
