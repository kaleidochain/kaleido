package stamping

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
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

func blockGenerator(t *testing.T, chain *Chain, maxHeight uint64, eventCh chan<- event) {
	parent := genesisHeader
	for height := uint64(1); height <= maxHeight; height++ {
		header := NewHeader(height, parent)
		fc := NewFinalCertificate(height, parent)
		parent = header

		err := chain.AddBlock(header, fc)
		if err != nil {
			t.Errorf("AddBlock failed, height=%d, err=%v", header.Height, err)
		}

		eventCh <- event{Height: header.Height, Type: newBlockEvent}
	}
	close(eventCh)
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

func buildChain(t *testing.T, maxHeight uint64) *Chain {
	chain := NewChain()

	eventCh := make(chan event, 100)
	go blockGenerator(t, chain, maxHeight, eventCh)
	stampingCh := makeStampingGenerator(defaultConfig, chain, eventCh)

	for s := range stampingCh {
		err := chain.AddStampingCertificate(s)
		if err != nil {
			t.Errorf("AddStampingCertificate failed, height=%d, err=%v", s.Height, err)
			return nil
		}
	}

	return chain
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

func TestNewChain(t *testing.T) {
	const maxHeight = 100000
	buildChain(t, maxHeight)
}

func TestSyncChain(t *testing.T) {
	rand.Seed(2)

	const maxHeight = 102
	a := buildChain(t, maxHeight)

	ensureSyncOk(t, a)
}

func TestAutoSyncChain(t *testing.T) {
	const count = 100
	rand.Seed(time.Now().UnixNano())

	for i := 0; i < count; i++ {
		const maxHeight = 10105
		a := buildChain(t, maxHeight)

		ensureSyncOk(t, a)
	}
}

func buildSpecialChain(t *testing.T, B, maxHeight uint64, heights []uint64) *Chain {
	defaultConfig.B = B

	parent := genesisHeader
	chain := NewChain()
	for height := uint64(1); height <= maxHeight; height++ {
		header := NewHeader(height, parent)
		fc := NewFinalCertificate(height, parent)
		parent = header

		err := chain.AddBlock(header, fc)
		if err != nil {
			t.Fatalf("AddBlock failed, height=%d, err=%v", header.Height, err)
		}
	}

	for _, height := range heights {
		proofHeader := chain.Header(height - defaultConfig.B)
		s := NewStampingCertificate(height, proofHeader)
		err := chain.AddStampingCertificate(s)
		if err != nil {
			t.Fatalf("AddStampingCertificate failed, height=%d, err=%v", s.Height, err)
		}
	}

	return chain
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
	const maxHeight = 39
	const B = 20

	// 20->21->22->23->30->33->39
	scHeights := []uint64{21, 22, 23, 30, 33, 39}
	chain := buildSpecialChain(t, B, maxHeight, scHeights)

	ensureSyncOk(t, chain)
}

func TestSync0ToBnFzEqualB(t *testing.T) {
	const maxHeight = 43
	const B = 20

	// 20->21->22->23->30->33->39
	scHeights := []uint64{21, 22, 23, 30, 33, 39, 40, 41, 42}
	chain := buildSpecialChain(t, B, maxHeight, scHeights)

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
