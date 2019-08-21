package stamping

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

const (
	newBlockEvent = 1
)

type StampingMaker interface {
	Make(height uint64, proofHeader *Header) *StampingCertificate
}

type randomStampingMaker int

func (failureProbability randomStampingMaker) Make(height uint64, proofHeader *Header) *StampingCertificate {
	if rand.Intn(100) >= int(failureProbability) {
		return NewStampingCertificate(height, proofHeader)
	}
	return nil
}

type sequenceStampingMaker []uint64

func (sequence sequenceStampingMaker) Make(height uint64, proofHeader *Header) *StampingCertificate {
	for _, h := range sequence {
		if h == height {
			return NewStampingCertificate(height, proofHeader)
		}
	}
	return nil
}

func buildChainConcurrency(t *testing.T, config *Config, chain *Chain, begin, end uint64, maker StampingMaker) {
	blockHeightCh := make(chan uint64, 1)
	go func() {
		if begin == 0 {
			begin = 1
		}
		prev := chain.Header(begin - 1)
		for h := begin; h < end; h++ {
			header := NewHeader(h, prev)
			finalCertificate := NewFinalCertificate(header, prev)

			err := chain.AddBlock(header, finalCertificate)
			if err != nil {
				t.Fatalf("AddBlock failed, height=%d, err=%v", h, err)
			}

			blockHeightCh <- h
			prev = header
		}
		close(blockHeightCh)
	}()

	for height := range blockHeightCh {
		if height <= config.B {
			continue
		}

		proofHeader := chain.Header(height - config.B)
		if s := maker.Make(height, proofHeader); s != nil {
			err := chain.AddStampingCertificate(s)
			if err != nil {
				t.Fatalf("AddStampingCertificate failed, height=%d, err=%v", s.Height, err)
			}
		}
	}
}

func buildChainByRandom(t *testing.T, maxHeight uint64) *Chain {
	chain := NewChain()
	buildChainConcurrency(t, defaultConfig, chain, 1, maxHeight+1, randomStampingMaker(defaultConfig.Probability))
	return chain
}

func buildChainBySequence(t *testing.T, b uint64, maxHeight uint64, seq []uint64) *Chain {
	config := &Config{
		B:           b,
		Probability: 0,
	}
	chain := NewChain()
	buildChainConcurrency(t, config, chain, 1, maxHeight+1, sequenceStampingMaker(seq))
	return chain
}

func buildSpecialChain(t *testing.T, B, maxHeight uint64, heights []uint64) *Chain {
	return buildChainBySequence(t, B, maxHeight, heights)
}

func appendStampingCertificate(t *testing.T, B uint64, chain *Chain, heights []uint64) {
	config := &Config{
		B:           B,
		Probability: 0,
	}

	for _, h := range heights {
		proof := chain.Header(h - config.B)
		stampingCertificate := NewStampingCertificate(h, proof)
		err := chain.addStampingCertificate(stampingCertificate)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func ensureSyncOk(t *testing.T, a *Chain) {
	b := NewChain()
	if err := b.Sync(a); err != nil {
		t.Fatalf("sync error, err:%s", err)
	}

	c := NewChain()
	if err := c.Sync(b); err != nil {
		t.Fatalf("sync error, err:%s", err)
	}

	if equal, err := c.Equal(b); !equal {
		b.Print()
		fmt.Println("---------------------------------after-----------------------------------------------------")
		c.Print()
		t.Fatal(err.Error())
	}

	/*
		a.Print()
		fmt.Println("---------------------------------after b-----------------------------------------------------")
		b.Print()
		fmt.Println("---------------------------------after c-----------------------------------------------------")
		c.Print()
	*/
}

//---------------------------------

func TestNewChain(t *testing.T) {
	const maxHeight = 100000
	buildChainByRandom(t, maxHeight)
}

func TestSyncChain(t *testing.T) {
	rand.Seed(2)

	const maxHeight = 102
	a := buildChainByRandom(t, maxHeight)

	ensureSyncOk(t, a)
}

func TestAutoSyncChain(t *testing.T) {
	const count = 100
	rand.Seed(time.Now().UnixNano())

	for i := 0; i < count; i++ {
		const maxHeight = 10105
		a := buildChainByRandom(t, maxHeight)

		ensureSyncOk(t, a)
	}
}

func TestSyncWhenEmptyChain(t *testing.T) {
	const maxHeight = 20
	const B = 20

	chain := NewChain()

	ensureSyncOk(t, chain)
}

func TestSync0ToB(t *testing.T) {
	const maxHeight = 20
	const B = 20

	// 20
	chain := buildSpecialChain(t, B, maxHeight, nil)

	ensureSyncOk(t, chain)
}

func TestSync0ToBnNLessThanB(t *testing.T) {
	maxHeight := uint64(40)
	const B = 20

	// 20->21->22->23->30->33->40
	scHeights := []uint64{21, 22, 23, 30, 33, 40}
	chain := buildSpecialChain(t, B, maxHeight, scHeights)
	expectedSCStatus := SCStatus{
		Candidate: 40,
		Proof:     20,
		Fz:        20,
	}
	if chain.scStatus != expectedSCStatus {
		t.Fatalf("scstatus error, chain.scstatus:%v, expected:%v", chain.scStatus, expectedSCStatus)
	}
	ensureSyncOk(t, chain)

	maxHeight = 41
	// 20->21->22->23->30->33->40->41
	scHeights = []uint64{21, 22, 23, 30, 33, 40, 41}
	chain = buildSpecialChain(t, B, maxHeight, scHeights)
	expectedSCStatus = SCStatus{
		Candidate: 41,
		Proof:     40,
		Fz:        20,
	}
	if chain.scStatus != expectedSCStatus {
		t.Fatalf("scstatus error, chain.scstatus:%v, expected:%v", chain.scStatus, expectedSCStatus)
	}
	ensureSyncOk(t, chain)
}

func TestSync0ToBnFzEqualB(t *testing.T) {
	const B = 20

	maxHeight := uint64(60)
	// 20->21->22->23->30->33->39->40->41->42--->60
	scHeights := []uint64{21, 22, 23, 30, 33, 39, 40, 41, 42, 60}
	chain := buildSpecialChain(t, B, maxHeight, scHeights)
	expectedSCStatus := SCStatus{
		Candidate: 60,
		Proof:     40,
		Fz:        20,
	}
	if chain.scStatus != expectedSCStatus {
		t.Fatalf("scstatus error, chain.scstatus:%v, expected:%v", chain.scStatus, expectedSCStatus)
	}
	ensureSyncOk(t, chain)

	maxHeight = uint64(61)
	// 20->21->22->23->30->33->39->40->41->42--->60->61
	scHeights = []uint64{21, 22, 23, 30, 33, 39, 40, 41, 42, 60, 61}
	chain = buildSpecialChain(t, B, maxHeight, scHeights)
	expectedSCStatus = SCStatus{
		Candidate: 61,
		Proof:     60,
		Fz:        40,
	}
	if chain.scStatus != expectedSCStatus {
		t.Fatalf("scstatus error, chain.scstatus:%v, expected:%v", chain.scStatus, expectedSCStatus)
	}
	ensureSyncOk(t, chain)
}

func TestSyncWhenFPNearAndPCFurtherThanB(t *testing.T) {
	const maxHeight = 130
	const B = 20

	// 20-->40->41-->60->61------>130
	scHeights := []uint64{40, 41, 60, 61, 130}
	chain := buildSpecialChain(t, B, maxHeight, scHeights)

	ensureSyncOk(t, chain)
}

func TestSyncWhenFPFurtherThanBAndPCFurtherThanB(t *testing.T) {
	const maxHeight = 170
	const B = 20

	// 20-->40->41-->60->61------>130------>165
	scHeights := []uint64{40, 41, 60, 61, 130, 165}
	chain := buildSpecialChain(t, B, maxHeight, scHeights)

	ensureSyncOk(t, chain)
}

func TestSyncWhenFPFurtherThanBAndPCNear(t *testing.T) {
	const maxHeight = 170
	const B = 20

	// 20-->40->41-->60->61------>130->131->132-->150------>165
	scHeights := []uint64{40, 41, 60, 61, 130, 165}
	chain := buildSpecialChain(t, B, maxHeight, scHeights)

	ensureSyncOk(t, chain)
}

func TestSyncWhenFPEqualBAndPCEqualBAdd1(t *testing.T) {
	const maxHeight = 200
	const B = 20

	// 20-->40->41-->60->61------>130->131->132-->150------>165->170->171---->190->191
	scHeights := []uint64{40, 41, 60, 61, 130, 131, 132, 150, 165, 170, 171, 190, 191}
	chain := buildSpecialChain(t, B, maxHeight, scHeights)

	ensureSyncOk(t, chain)
}
