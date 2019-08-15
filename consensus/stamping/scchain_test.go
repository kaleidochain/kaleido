package stamping

import (
	"math/rand"
	"testing"
)

func buildStampingCertificate(height uint64, proofHeader *Header) *StampingCertificate {
	randNum := rand.Intn(100)
	if randNum < 65 {
		return NewStampingCertificate(height, proofHeader)
	}
	return nil
}

type block struct {
	header *Header
	fc     *FinalCertificate
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
	}()
	return ch
}

const (
	newBlockEvent = 1
)

type event struct {
	Height uint64
	Type   uint
}

func makeStampingGenerator(config *Config, chain *Chain, eventCh <-chan event) <-chan *StampingCertificate {
	ch := make(chan *StampingCertificate)
	go func() {
		for e := range eventCh {
			proofHeader := chain.Header(e.Height - config.B)
			s := buildStampingCertificate(e.Height, proofHeader)
			ch <- s
		}
	}()
	return ch
}

func TestNewChain(t *testing.T) {
	const maxHeight = 10000
	chain := NewChain()

	blockCh := makeBlockGenerator(defaultConfig, chain, maxHeight)
	eventCh := make(chan event)
	stampingCh := makeStampingGenerator(defaultConfig, chain, eventCh)

	for {
		select {
		case block := <-blockCh:
			chain.AddBlock(block.header, block.fc)
			eventCh <- event{Height: block.header.Height, Type: newBlockEvent}
		case s := <-stampingCh:
			chain.AddStampingCertificate(s)
		}
	}

	chain.Print()
}
