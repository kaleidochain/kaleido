package stamping

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
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

type ChainReadWriter interface {
	Header(height uint64) *Header
	AddBlock(header *Header, fc *FinalCertificate) error
	AddStampingCertificate(sc *StampingCertificate) error
}

type MultiChainWrapper []*Chain

func (c MultiChainWrapper) Header(height uint64) *Header {
	return c[0].Header(height)
}

func (c MultiChainWrapper) AddBlock(header *Header, fc *FinalCertificate) error {
	for _, each := range c {
		err := each.AddBlock(header, fc)
		if err != nil {
			return err
		}
	}
	return nil
}
func (c MultiChainWrapper) AddStampingCertificate(sc *StampingCertificate) error {
	for _, each := range c {
		err := each.AddStampingCertificate(sc)
		if err != nil {
			return err
		}
	}
	return nil
}

func buildChainConcurrency(t *testing.T, config *Config, chain ChainReadWriter, begin, end uint64, maker StampingMaker) {
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
		if height <= config.HeightB() {
			continue
		}

		switch c := chain.(type) {
		case *Chain:
			proofHeader := c.Header(height - config.B)
			if s := maker.Make(height, proofHeader); s != nil {
				err := c.AddStampingCertificate(s)
				if err != nil {
					t.Fatalf("AddStampingCertificate failed, height=%d, err=%v", s.Height, err)
				}
			}
		case MultiChainWrapper:
			for _, each := range c {
				proofHeader := each.Header(height - config.B)
				if s := maker.Make(height, proofHeader); s != nil {
					err := each.AddStampingCertificate(s)
					if err != nil {
						t.Fatalf("AddStampingCertificate failed, height=%d, err=%v", s.Height, err)
					}
				}
			}
		}
	}
}

func buildDefaultChainByRandom(t *testing.T, maxHeight uint64) *Chain {
	chain := NewChain(defaultConfig)
	buildChainConcurrency(t, defaultConfig, chain, 1, maxHeight+1, randomStampingMaker(defaultConfig.FailureProbability))
	return chain
}

func buildChainBySequence(t *testing.T, b uint64, maxHeight uint64, seq []uint64) *Chain {
	config := &Config{
		B:                  b,
		FailureProbability: 0,
	}
	chain := NewChain(config)
	buildChainConcurrency(t, config, chain, 1, maxHeight+1, sequenceStampingMaker(seq))
	return chain
}

func buildSpecialChain(t *testing.T, B, maxHeight uint64, heights []uint64) *Chain {
	return buildChainBySequence(t, B, maxHeight, heights)
}

func ensureSyncOk(t *testing.T, a *Chain) (b, c, d *Chain) {
	b = NewChain(a.config)
	b.AddPeer(a)
	if err := b.Sync(); err != nil {
		a.Print()
		fmt.Println("---------------------------------after-----------------------------------------------------")
		b.Print()
		t.Fatalf("sync error, err:%s", err)
	}

	c = NewChain(a.config)
	c.AddPeer(b)
	if err := c.Sync(); err != nil {
		t.Fatalf("sync error, err:%v", err)
	}

	d = NewChain(a.config)
	if err := d.SyncBase(c); err != nil {
		t.Fatalf("sync error, err:%v", err)
	}

	if equal, err := d.Equal(a); !equal {
		a.Print()
		fmt.Println("---------------------------------after-----------------------------------------------------")
		d.Print()
		t.Fatal(err)
	}

	/*
		a.Print()
		fmt.Println("---------------------------------after b-----------------------------------------------------")
		b.Print()
		fmt.Println("---------------------------------after c-----------------------------------------------------")
		c.Print()
	*/

	return
}

//---------------------------------

func TestNewChain(t *testing.T) {
	const maxHeight = 100000
	buildDefaultChainByRandom(t, maxHeight)
}

func TestSyncChain(t *testing.T) {
	rand.Seed(2)

	const maxHeight = 102
	a := buildDefaultChainByRandom(t, maxHeight)

	ensureSyncOk(t, a)
}

func TestAutoSyncChain(t *testing.T) {
	const count = 100
	rand.Seed(time.Now().UnixNano())

	for i := 0; i < count; i++ {
		const maxHeight = 10105
		a := buildDefaultChainByRandom(t, maxHeight)

		ensureSyncOk(t, a)
	}
}

func TestSyncEmptyChain(t *testing.T) {
	chain := NewChain(defaultConfig)
	ensureSyncOk(t, chain)
}

func TestSync0ToB(t *testing.T) {
	const maxHeight = 20
	const B = 20

	// 20
	chain := buildSpecialChain(t, B, maxHeight, nil)

	ensureSyncOk(t, chain)
}

func TestSyncWithoutSC(t *testing.T) {
	const maxHeight = 101
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

func TestSyncWhenFPAllHeaderNoFC(t *testing.T) {
	const B = 20

	maxHeight := uint64(41)
	// 20-->40->41
	scHeights := []uint64{40, 41}
	chain := buildSpecialChain(t, B, maxHeight, scHeights)
	expectedSCStatus := SCStatus{
		Candidate: 41,
		Proof:     40,
		Fz:        20,
	}
	if chain.scStatus != expectedSCStatus {
		t.Fatalf("scstatus error, chain.scstatus:%v, expected:%v", chain.scStatus, expectedSCStatus)
	}
	ensureSyncOk(t, chain)

	maxHeight = uint64(60)
	// 20-->40->41-->60
	scHeights = []uint64{40, 41, 60}
	chain = buildSpecialChain(t, B, maxHeight, scHeights)
	expectedSCStatus = SCStatus{
		Candidate: 60,
		Proof:     40,
		Fz:        20,
	}
	if chain.scStatus != expectedSCStatus {
		t.Fatalf("scstatus error, chain.scstatus:%v, expected:%v", chain.scStatus, expectedSCStatus)
	}
	ensureSyncOk(t, chain)

	maxHeight = uint64(60)
	// 20-->40->41-->51
	scHeights = []uint64{40, 41, 51}
	chain = buildSpecialChain(t, B, maxHeight, scHeights)
	expectedSCStatus = SCStatus{
		Candidate: 51,
		Proof:     40,
		Fz:        20,
	}
	if chain.scStatus != expectedSCStatus {
		t.Fatalf("scstatus error, chain.scstatus:%v, expected:%v", chain.scStatus, expectedSCStatus)
	}
	ensureSyncOk(t, chain)

	maxHeight = uint64(72)
	// 20-->40->41-->70
	scHeights = []uint64{40, 41, 70}
	chain = buildSpecialChain(t, B, maxHeight, scHeights)
	expectedSCStatus = SCStatus{
		Candidate: 70,
		Proof:     41,
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

// ----------------

func TestBuildMultiChain(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	config := &Config{
		B:                  1000,
		FailureProbability: 65,
	}

	chains := make(MultiChainWrapper, 3)
	for i := range chains {
		chains[i] = NewChain(config)
	}

	buildChainConcurrency(t, config, chains, 1, 50000, randomStampingMaker(config.FailureProbability))

	for _, c := range chains {
		c.PrintFrozenBreadcrumbs()
	}
	for _, c := range chains {
		c.PrintProperty()
	}
}

func TestMultiChainAllSameSync(t *testing.T) {
	rand.Seed(1)

	maxHeight := uint64(2000)
	config := &Config{
		B:                  100,
		FailureProbability: 65,
	}

	archive := buildSpecialChain(t, config.B, maxHeight, nil)

	chain := NewChain(config)
	buildChainConcurrency(t, config, chain, 1, maxHeight, randomStampingMaker(config.FailureProbability))
	b, c, d := ensureSyncOk(t, chain)

	other := NewChain(config)
	other.AddPeer(b)
	other.AddPeer(c)
	other.AddPeer(d)
	other.AddArchivePeer(archive)

	other.SetTroubleMaker(RandomTroubleMaker(10))
	if err := other.Sync(); err != nil || other.currentHeight != b.currentHeight {
		b.Print()
		fmt.Println("---------------------------------after-----------------------------------------------------")
		other.Print()
		t.Fatalf("sync error, my height:%d, other height:%d, err:%s", other.currentHeight, b.currentHeight, err)
	}
}

func TestMultiChain2Same1differentSync(t *testing.T) {
	rand.Seed(1)

	maxHeight := uint64(2000)
	config := &Config{
		B:                  100,
		FailureProbability: 65,
	}

	archive := buildSpecialChain(t, config.B, maxHeight, nil)

	chain := NewChain(config)
	buildChainConcurrency(t, config, chain, 1, maxHeight, randomStampingMaker(config.FailureProbability))
	b, c, _ := ensureSyncOk(t, chain)
	d := NewChain(config)
	buildChainConcurrency(t, config, d, 1, maxHeight, randomStampingMaker(config.FailureProbability))

	other := NewChain(config)
	other.AddPeer(b)
	other.AddPeer(c)
	other.AddPeer(d)
	other.AddArchivePeer(archive)

	other.SetTroubleMaker(RandomTroubleMaker(10))
	if err := other.Sync(); err != nil || other.currentHeight != b.currentHeight {
		b.Print()
		fmt.Println("---------------------------------after-----------------------------------------------------")
		other.Print()
		t.Fatalf("sync error, my height:%d, other height:%d, err:%s", other.currentHeight, b.currentHeight, err)
	}
}

func TestMultiChain3differentSync(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	maxHeight := uint64(2000)
	config := &Config{
		B:                  100,
		FailureProbability: 65,
	}

	archive := buildSpecialChain(t, config.B, maxHeight, nil)

	chains := make(MultiChainWrapper, 3)
	for i := range chains {
		chains[i] = NewChain(config)
	}

	buildChainConcurrency(t, config, chains, 1, maxHeight, randomStampingMaker(config.FailureProbability))

	other := NewChain(config)
	for i := range chains {
		other.AddPeer(chains[i])
	}
	other.AddArchivePeer(archive)

	other.SetTroubleMaker(RandomTroubleMaker(10))
	if err := other.Sync(); err != nil || other.currentHeight != chains[0].currentHeight {
		for i := range chains {
			chains[i].Print()
		}
		fmt.Println("---------------------------------after-----------------------------------------------------")
		other.Print()
		t.Fatalf("sync error, my height:%d, other height:%d, err:%s", other.currentHeight, chains[0].currentHeight, err)
	}
}
