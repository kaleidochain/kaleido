package stamping

import (
	"math/rand"
	"testing"
	"time"
)

func buildStampingCertificate(height uint64, proofHeader *Header) *StampingCertificate {
	randNum := rand.Intn(100)
	if randNum < 65 {
		return NewStampingCertificate(height, proofHeader)
	}
	return nil
}

func TestNewChain(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())

	chain := NewChain()

	const MaxHeight = 10000
	parent := genesisHeader
	for height := uint64(1); height < MaxHeight; height++ {
		header := NewHeader(height, parent)
		fc := NewFinalCertificate(height, parent)

		if err := chain.AddBlock(header, fc); err != nil {
			t.Errorf("AddBlock error, height:%d, err:%s", height, err)
		}
		parent = header

		if height > defaultConfig.B {
			proofHeader := chain.Header(height)

			sc := buildStampingCertificate(height, proofHeader)
			if err := chain.AddStampingCertificate(sc); err != nil {
				t.Errorf("AddStampingCertificate error, height:%d, err:%s", height, err)
			}
		}
	}

	chain.Print()
}
