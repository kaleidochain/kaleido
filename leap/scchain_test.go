package leap

import (
	"crypto/sha512"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/log"

	"github.com/kaleidochain/kaleido/common"
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
				s.AddVote(NewStampingVote(height, c.config.Address, c.config.StampingThreshold))
				err := c.AddStampingCertificate(s)
				if err != nil {
					t.Fatalf("AddStampingCertificate failed, height=%d, err=%v", s.Height, err)
				}
			}
		case MultiChainWrapper:
			for _, each := range c {
				proofHeader := each.Header(height - config.B)
				if s := maker.Make(height, proofHeader); s != nil {
					s.AddVote(NewStampingVote(height, each.config.Address, each.config.StampingThreshold))
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
	b.AddPeerChain(a)
	if err := b.Sync(); err != nil {
		a.Print()
		fmt.Println("---------------------------------after-----------------------------------------------------")
		b.Print()
		t.Fatalf("sync error, err:%s", err)
	}

	c = NewChain(a.config)
	c.AddPeerChain(b)
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

func TestSyncAllSameMultiChain(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

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
	other.AddPeerChain(b)
	other.AddPeerChain(c)
	other.AddPeerChain(d)
	other.AddArchiveChain(archive)

	other.SetTroubleMaker(RandomTroubleMaker(10))
	for {
		err := other.Sync()
		if err == nil {
			other.Print()
			return
		}

		if err == ErrRandomTrouble {
			continue
		}

		if err != nil || other.currentHeight != b.currentHeight {
			b.Print()
			fmt.Println("---------------------------------after-----------------------------------------------------")
			other.Print()
			t.Fatalf("sync error, my height:%d, other height:%d, err:%s", other.currentHeight, b.currentHeight, err)
		}
	}
}

func TestSync2Same1DifferentMultiChain(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

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
	other.AddPeerChain(b)
	other.AddPeerChain(c)
	other.AddPeerChain(d)
	other.AddArchiveChain(archive)

	other.SetTroubleMaker(RandomTroubleMaker(10))
	for {
		err := other.Sync()
		if err == nil {
			other.Print()
			return
		}

		if err == ErrRandomTrouble {
			continue
		}

		if err != nil || other.currentHeight != b.currentHeight {
			b.Print()
			fmt.Println("---------------------------------after-----------------------------------------------------")
			other.Print()
			t.Fatalf("sync error, my height:%d, other height:%d, err:%s", other.currentHeight, b.currentHeight, err)
		}
	}
}

func TestSync3DifferentMultiChain(t *testing.T) {
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
		other.AddPeerChain(chains[i])
	}
	other.AddArchiveChain(archive)

	other.SetTroubleMaker(RandomTroubleMaker(10))
	for {
		err := other.Sync()
		if err == nil {
			other.Print()
			return
		}

		if err == ErrRandomTrouble {
			continue
		}

		if err != nil || other.currentHeight != chains[0].currentHeight {
			for i := range chains {
				chains[i].Print()
			}
			fmt.Println("---------------------------------after-----------------------------------------------------")
			other.Print()
			t.Fatalf("sync error, my height:%d, other height:%d, err:%s", other.currentHeight, chains[0].currentHeight, err)
		}
	}
}

func TestSyncAllSameMultiChainAndContinueGrow(t *testing.T) {
	rand.Seed(1)

	maxHeight := uint64(2000)
	finalMaxHeight := uint64(2500)
	config := &Config{
		B:                  100,
		FailureProbability: 65,
	}

	archive := buildSpecialChain(t, config.B, finalMaxHeight, nil)

	chain := NewChain(config)
	buildChainConcurrency(t, config, chain, 1, maxHeight, randomStampingMaker(config.FailureProbability))
	b, c, d := ensureSyncOk(t, chain)

	other := NewChain(config)
	other.AddPeerChain(b)
	other.AddPeerChain(c)
	other.AddPeerChain(d)
	other.AddArchiveChain(archive)

	other.SetTroubleMaker(RandomTroubleMaker(10))
	for {
		err := other.Sync()
		if err == nil {
			return
		}

		if err == ErrRandomTrouble {
			continue
		}

		if err != nil || other.currentHeight != b.currentHeight {
			b.Print()
			fmt.Println("---------------------------------after-----------------------------------------------------")
			other.Print()
			t.Fatalf("sync error, my height:%d, other height:%d, err:%s", other.currentHeight, b.currentHeight, err)
		}
	}
}

func TestSync3DifferentMultiChainWithContinueGrow(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	maxHeight := uint64(2000)
	finalMaxHeight := uint64(2500)
	config := &Config{
		B:                  100,
		FailureProbability: 65,
	}

	archive := buildSpecialChain(t, config.B, finalMaxHeight, nil)

	chains := make(MultiChainWrapper, 3)
	for i := range chains {
		chains[i] = NewChain(config)
	}

	buildChainConcurrency(t, config, chains, 1, maxHeight, randomStampingMaker(config.FailureProbability))

	other := NewChain(config)
	for i := range chains {
		other.AddPeerChain(chains[i])
	}
	other.AddArchiveChain(archive)

	other.SetTroubleMaker(RandomTroubleMaker(10))
	hasGrowUp := false
	for {
		err := other.Sync()
		if err == nil {
			for i := range chains {
				chains[i].Print()
			}
			fmt.Println("---------------------------------after-----------------------------------------------------")
			other.Print()
			return
		}

		if err == ErrRandomTrouble {
			if !hasGrowUp {
				buildChainConcurrency(t, config, chains, maxHeight, finalMaxHeight, randomStampingMaker(config.FailureProbability))
				hasGrowUp = true
			}
			continue
		}

		if err != nil || other.currentHeight != chains[0].currentHeight {
			for i := range chains {
				chains[i].Print()
			}
			fmt.Println("---------------------------------after-----------------------------------------------------")
			other.Print()
			t.Fatalf("sync error, my height:%d, other height:%d, err:%s", other.currentHeight, chains[0].currentHeight, err)
		}
	}
}

func TestSync3DifferentMultiChainWithContinueGrowAndConfigHeight(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	maxHeight := uint64(2000)
	finalMaxHeight := uint64(2500)
	config := &Config{
		B:                  100,
		FailureProbability: 65,
		BaseHeight:         834,
	}

	archive := buildSpecialChain(t, config.B, finalMaxHeight, nil)

	chains := make(MultiChainWrapper, 3)
	for i := range chains {
		chains[i] = NewChain(config)
	}

	buildChainConcurrency(t, config, chains, 1, maxHeight, randomStampingMaker(config.FailureProbability))

	other := NewChain(config)
	for i := range chains {
		other.AddPeerChain(chains[i])
	}
	other.AddArchiveChain(archive)

	other.SetTroubleMaker(RandomTroubleMaker(30))
	hasGrowUp := false
	for {
		err := other.Sync()
		if err == nil {
			for i := range chains {
				chains[i].Print()
			}
			fmt.Println("---------------------------------after-----------------------------------------------------")
			other.Print()
			return
		}

		if err == ErrRandomTrouble {
			if !hasGrowUp {
				buildChainConcurrency(t, config, chains, maxHeight, finalMaxHeight, randomStampingMaker(config.FailureProbability))
				hasGrowUp = true
			}
			continue
		}

		if err != nil || other.currentHeight != chains[0].currentHeight {
			for i := range chains {
				chains[i].Print()
			}
			fmt.Println("---------------------------------after-----------------------------------------------------")
			other.Print()
			t.Fatalf("sync error, my height:%d, other height:%d, err:%s", other.currentHeight, chains[0].currentHeight, err)
		}
	}
}

func TestSync3DifferentMultiChainWithContinueGrowAndConfigHash(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	maxHeight := uint64(2000)
	finalMaxHeight := uint64(2500)
	config := &Config{
		B:                  100,
		FailureProbability: 65,
		BaseHeight:         834,
	}
	bytes := common.Uint64ToHash(config.BaseHeight)
	config.BaseHash = sha512.Sum512_256(bytes[:])

	archive := buildSpecialChain(t, config.B, finalMaxHeight, nil)

	chains := make(MultiChainWrapper, 3)
	for i := range chains {
		chains[i] = NewChain(config)
	}

	buildChainConcurrency(t, config, chains, 1, maxHeight, randomStampingMaker(config.FailureProbability))

	other := NewChain(config)
	for i := range chains {
		other.AddPeerChain(chains[i])
	}
	other.AddArchiveChain(archive)

	other.SetTroubleMaker(RandomTroubleMaker(30))
	hasGrowUp := false
	for {
		err := other.Sync()
		if err == nil {
			for i := range chains {
				chains[i].Print()
			}
			fmt.Println("---------------------------------after-----------------------------------------------------")
			other.Print()
			return
		}

		if err == ErrRandomTrouble {
			if !hasGrowUp {
				buildChainConcurrency(t, config, chains, maxHeight, finalMaxHeight, randomStampingMaker(config.FailureProbability))
				hasGrowUp = true
			}
			continue
		}

		if err != nil || other.currentHeight != chains[0].currentHeight {
			for i := range chains {
				chains[i].Print()
			}
			fmt.Println("---------------------------------after-----------------------------------------------------")
			other.Print()
			t.Fatalf("sync error, my height:%d, other height:%d, err:%s", other.currentHeight, chains[0].currentHeight, err)
		}
	}
}

func TestChainSCVote(t *testing.T) {
	rand.Seed(6)

	maxHeight := uint64(200)
	var configs []*Config
	for i := 1; i <= 3; i++ {
		config := Config{
			B:                  20,
			FailureProbability: 65,
			StampingThreshold:  100,
		}
		config.Address = common.HexToAddress(fmt.Sprintf("0x%d00000000000000000000000000000000000000%d", i, i))
		configs = append(configs, &config)
	}

	chains := make(MultiChainWrapper, 3)
	for i := range chains {
		chains[i] = NewChain(configs[i])
		chains[i].AutoBuildSCVote(true)
		chains[i].Start()
	}

	buildChainConcurrency(t, configs[0], chains, 1, maxHeight, randomStampingMaker(configs[0].FailureProbability))

	chains[0].AddPeerChain(chains[1])
	chains[0].AddPeerChain(chains[2])
	chains[1].AddPeerChain(chains[0])
	chains[1].AddPeerChain(chains[2])
	chains[2].AddPeerChain(chains[0])
	chains[2].AddPeerChain(chains[1])

	go func() {
		for {
			for i := range chains {
				chains[i].Print()
			}

			time.Sleep(60 * time.Second)
		}
	}()

	time.Sleep(50 * 60 * time.Second)
}

func TestChainGossipP1SCP2NoSC(t *testing.T) {
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(5), log.StreamHandler(os.Stdout, log.TerminalFormat(false))))
	rand.Seed(3)

	chainNum := 2
	chains := makeMultiChain(t, chainNum)
	for i := range chains {
		if i+1 == chainNum {
			chains[i].AutoBuildSCVote(false)
		} else {
			chains[i].AutoBuildSCVote(true)
		}
	}

	makePairPeer(chains[0], chains[1])
	//makePairPeer(chains[0], chains[2])
	//makePairPeer(chains[0], chains[3])

	go func() {
		for {
			for i := range chains {
				chains[i].Print()
			}

			time.Sleep(60 * time.Second)
		}
	}()

	time.Sleep(150 * 60 * time.Second)
}

func makeMultiChain(t *testing.T, chainNum int) []*Chain {
	maxHeight := uint64(20)
	var configs []*Config
	for i := 1; i <= chainNum; i++ {
		config := Config{
			B:                  20,
			FailureProbability: 65,
			StampingThreshold:  100,
		}
		config.Address = common.HexToAddress(fmt.Sprintf("0x%d00000000000000000000000000000000000000%d", i, i))
		configs = append(configs, &config)
	}

	chains := make(MultiChainWrapper, chainNum)
	for i := range chains {
		chains[i] = NewChain(configs[i])
		chains[i].SetName(fmt.Sprintf("chain%d", i+1))
		chains[i].Start()
	}

	buildChainConcurrency(t, configs[0], chains, 1, maxHeight, randomStampingMaker(configs[0].FailureProbability))

	return chains
}

func TestChainGossipP1SCP2SC(t *testing.T) {
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(5), log.StreamHandler(os.Stdout, log.TerminalFormat(false))))
	rand.Seed(3)

	chains := makeMultiChain(t, 2)
	for i := range chains {
		chains[i].AutoBuildSCVote(true)
	}

	makePairPeer(chains[0], chains[1])

	go func() {
		for {
			for i := range chains {
				chains[i].Print()
			}

			time.Sleep(60 * time.Second)
		}
	}()

	time.Sleep(150 * 60 * time.Second)
}

func TestChainGossipP1P2P3(t *testing.T) {
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(5), log.StreamHandler(os.Stdout, log.TerminalFormat(false))))
	rand.Seed(3)

	chainNum := 3
	chains := makeMultiChain(t, chainNum)
	for i := range chains {
		if i+1 == chainNum {
			chains[i].AutoBuildSCVote(false)
		} else {
			chains[i].AutoBuildSCVote(true)
		}
	}

	makePairPeer(chains[0], chains[1])
	makePairPeer(chains[1], chains[2])

	/*
		go func() {
			for {
				for i := range chains {
					chains[i].Print()
				}

				time.Sleep(60 * time.Second)
			}
		}()
	*/

	time.Sleep(150 * 60 * time.Second)

	scStatus := chains[0].ChainStatus()
	equal, err := chains[1].EqualRange(chains[2], 1, scStatus.Fz)
	if !equal {
		t.Errorf(fmt.Sprintf("not equal, err:%s", err))

		chains[1].Print()
		fmt.Println("---------------------------------other-----------------------------------------------------")
		chains[2].Print()
	}
}

func TestChainGossipP1P2P3P4(t *testing.T) {
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(5), log.StreamHandler(os.Stdout, log.TerminalFormat(false))))
	rand.Seed(4)

	chainNum := 4
	chains := makeMultiChain(t, chainNum)
	for i := range chains {
		chains[i].AutoBuildSCVote(i < 2)
	}

	makePairPeer(chains[0], chains[1])
	makePairPeer(chains[1], chains[2])
	makePairPeer(chains[2], chains[3])

	time.Sleep(10 * 60 * time.Second)

	scStatus := chains[0].ChainStatus()
	equal, err := chains[0].EqualRange(chains[3], 1, scStatus.Fz)
	if !equal {
		t.Errorf(fmt.Sprintf("not equal, err:%s", err))

		chains[0].Print()
		fmt.Println("---------------------------------other-----------------------------------------------------")
		chains[3].Print()
	} else {
		chains[0].Print()
		fmt.Println("---------------------------------other-----------------------------------------------------")
		chains[3].Print()
		t.Log("equal")
	}

}

func TestChainGossipP1P2P3P4P1(t *testing.T) {
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(5), log.StreamHandler(os.Stdout, log.TerminalFormat(false))))
	rand.Seed(9)

	chainNum := 4
	chains := makeMultiChain(t, chainNum)
	for i := range chains {
		chains[i].AutoBuildSCVote(true)

	}

	makePairPeer(chains[0], chains[1])
	makePairPeer(chains[1], chains[2])
	makePairPeer(chains[2], chains[3])
	makePairPeer(chains[0], chains[3])

	time.Sleep(20 * 60 * time.Second)

	scStatus := chains[0].ChainStatus()
	equal, err := chains[0].EqualRange(chains[3], 1, scStatus.Fz)
	if !equal {
		t.Errorf(fmt.Sprintf("not equal, err:%s", err))

		chains[0].Print()
		fmt.Println("---------------------------------other-----------------------------------------------------")
		chains[3].Print()
	} else {
		chains[0].Print()
		fmt.Println("---------------------------------other-----------------------------------------------------")
		chains[3].Print()
		t.Log("equal")
	}
}

func TestChainSyncGossip(t *testing.T) {
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(5), log.StreamHandler(os.Stdout, log.TerminalFormat(false))))
	rand.Seed(9)

	maxHeight := uint64(2000)
	config := &Config{
		B:                  30,
		FailureProbability: 65,
		StampingThreshold:  100,
	}

	a := NewChain(config)
	buildChainConcurrency(t, config, a, 1, maxHeight, randomStampingMaker(config.FailureProbability))
	b, c, d := ensureSyncOk(t, a)

	var chains []*Chain
	chains = append(chains, a, b, c, d)

	for i := range chains {
		index := i + 1
		chains[i].config = &Config{
			B:                  30,
			FailureProbability: 65,
			StampingThreshold:  100,
		}
		chains[i].config.Address = common.HexToAddress(fmt.Sprintf("0x%d00000000000000000000000000000000000000%d", index, index))
		chains[i].SetName(fmt.Sprintf("chain%d", index))
		chains[i].Start()
	}
	for i := range chains {

		chains[i].AutoBuildSCVote(true)
	}

	makePairPeer(chains[0], chains[1])
	makePairPeer(chains[1], chains[2])
	makePairPeer(chains[2], chains[3])
	makePairPeer(chains[0], chains[3])

	go func() {
		for {
			for i := range chains {
				chains[i].Print()
			}

			time.Sleep(60 * time.Second)
		}
	}()

	time.Sleep(20 * 60 * time.Second)
}
