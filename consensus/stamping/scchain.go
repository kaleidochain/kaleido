package stamping

import (
	"crypto/sha512"
	"fmt"
	"sync"

	"github.com/kaleidochain/kaleido/common"
)

var (
	defaultConfig = &Config{
		B:           24,
		Probability: 35,
	}

	genesisHeader = &Header{
		Height: 0,
	}
)

func MaxUint64(a, b uint64) uint64 {
	if a > b {
		return a
	}

	return b
}
func MinUint64(a, b uint64) uint64 {
	if a < b {
		return a
	}

	return b
}

type Config struct {
	B           uint64
	Probability int
}

type Header struct {
	Height     uint64
	ParentHash common.Hash
	Root       common.Hash
	Seed       common.Hash
}

func (h *Header) Hash() common.Hash {
	bytes := common.Uint64ToHash(h.Height)
	return sha512.Sum512_256(bytes[:])
}
func NewHeader(height uint64, parent *Header) *Header {
	return &Header{
		Height:     height,
		ParentHash: parent.Hash(),
		Root:       parent.Hash(), //
		Seed:       parent.Hash(),
	}

}

type FinalCertificate struct {
	Height     uint64
	ParentSeed common.Hash
	ParentRoot common.Hash
}

func NewFinalCertificate(height uint64, parent *Header) *FinalCertificate {
	return &FinalCertificate{
		Height:     height,
		ParentSeed: parent.Seed,
		ParentRoot: parent.Root,
	}
}

func (fc *FinalCertificate) Verify(header, parent *Header) bool {
	return fc.Height == header.Height &&
		fc.Height == parent.Height+1 &&
		fc.ParentSeed == parent.Seed &&
		fc.ParentRoot == parent.Root
}

type StampingCertificate struct {
	Height uint64
	Seed   common.Hash
	Root   common.Hash
}

func NewStampingCertificate(height uint64, proofHeader *Header) *StampingCertificate {
	return &StampingCertificate{
		Height: height,
		Seed:   proofHeader.Seed,
		Root:   proofHeader.Root,
	}
}

func (sc *StampingCertificate) Verify(header, proofHeader *Header) bool {
	return sc.Height > defaultConfig.B &&
		sc.Height == header.Height &&
		sc.Height == proofHeader.Height+defaultConfig.B &&
		sc.Seed == proofHeader.Seed &&
		sc.Root == proofHeader.Root
}

type Node struct {
	header              *Header
	finalCertificate    *FinalCertificate
	stampingCertificate *StampingCertificate
}

func NewNode(header *Header) *Node {
	return &Node{
		header: header,
	}
}

type SCStatus struct {
	Candidate uint64
	Proof     uint64
	Fz        uint64
}

type Chain struct {
	headerChain map[uint64]*Header
	mutexChain  sync.RWMutex
	fcChain     map[uint64]*FinalCertificate
	scChain     map[uint64]*StampingCertificate

	currentHeight uint64
	scStatus      SCStatus
}

func NewChain() *Chain {
	chain := &Chain{
		headerChain: make(map[uint64]*Header),
		fcChain:     make(map[uint64]*FinalCertificate),
		scChain:     make(map[uint64]*StampingCertificate),
		scStatus: SCStatus{
			Candidate: defaultConfig.B,
			Proof:     defaultConfig.B,
			Fz:        defaultConfig.B,
		},
	}
	chain.headerChain[0] = genesisHeader

	return chain
}

func (chain *Chain) FinalCertificate(height uint64) *FinalCertificate {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	return chain.fcChain[height]
}

func (chain *Chain) HeaderAndStampingCertificate(height uint64) (*Header, *StampingCertificate) {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	return chain.headerChain[height], chain.scChain[height]
}

func (chain *Chain) Header(height uint64) *Header {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	return chain.headerChain[height]
}

func (chain *Chain) header(height uint64) *Header {
	return chain.headerChain[height]
}

func (chain *Chain) hasHeader(height uint64) bool {
	_, ok := chain.headerChain[height]
	return ok
}

func (chain *Chain) AddHeader(header *Header) error {
	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

	return chain.addHeader(header)
}

func (chain *Chain) addHeader(header *Header) error {
	if _, ok := chain.headerChain[header.Height]; ok {
		return fmt.Errorf("header(%d) exists", header.Height)
	}
	nextHeader, ok := chain.headerChain[header.Height+1]
	if !ok {
		fmt.Printf("header(%d)\n", header.Height)
		return fmt.Errorf("next header(%d) not exists", header.Height+1)
	}
	if nextHeader.ParentHash != header.Hash() {
		return fmt.Errorf("next header(%d) ParentHash != header(%d).Hash", header.Height+1, header.Height)
	}
	chain.headerChain[header.Height] = header

	return nil
}

func (chain *Chain) AddBlock(header *Header, fc *FinalCertificate) error {
	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

	return chain.addBlock(header, fc)
}

func (chain *Chain) addBlock(header *Header, fc *FinalCertificate) error {
	if header.Height <= chain.currentHeight {
		return fmt.Errorf("block(%d) lower than currentHeight(%d)", header.Height, chain.currentHeight)
	}
	if h := chain.header(header.Height); h != nil {
		return fmt.Errorf("block(%d) exists", header.Height)
	}
	if _, ok := chain.fcChain[fc.Height]; ok {
		return fmt.Errorf("finalCertificate(%d) exists", fc.Height)
	}

	parent := chain.header(header.Height - 1)
	if parent == nil {
		return fmt.Errorf("parent block(%d) not exists", header.Height-1)
	}

	if !fc.Verify(header, parent) {
		return fmt.Errorf("block invalid")
	}

	chain.headerChain[header.Height] = header
	chain.fcChain[fc.Height] = fc
	chain.currentHeight = header.Height

	return nil
}

func (chain *Chain) addStampingCertificateWithHeader(header *Header, sc *StampingCertificate) error {
	_, ok := chain.headerChain[sc.Height]
	if ok {
		return fmt.Errorf("header(%d) exists", sc.Height)
	}

	if err := chain.verifyStampingCertificate(header, sc); err != nil {
		return err
	}

	chain.headerChain[header.Height] = header
	chain.scChain[sc.Height] = sc
	chain.currentHeight = header.Height // TODO: check currentHeight < height

	chain.updateStampingCertificate(sc.Height)
	return nil
}

func (chain *Chain) verifyStampingCertificate(header *Header, sc *StampingCertificate) error {
	if _, ok := chain.scChain[sc.Height]; ok {
		return fmt.Errorf("stampingCertificate(%d) exists", sc.Height)
	}

	proofHeader, ok := chain.headerChain[sc.Height-defaultConfig.B]
	if !ok {
		return fmt.Errorf("proof header(%d) not exists", sc.Height-defaultConfig.B)
	}

	if !sc.Verify(header, proofHeader) {
		return fmt.Errorf("sc(%d) invalid", sc.Height)
	}

	return nil
}

func (chain *Chain) addStampingCertificate(sc *StampingCertificate) error {
	header, ok := chain.headerChain[sc.Height]
	if !ok {
		return fmt.Errorf("header(%d) not exists", sc.Height)
	}

	if err := chain.verifyStampingCertificate(header, sc); err != nil {
		return err
	}

	chain.scChain[sc.Height] = sc

	chain.updateStampingCertificate(sc.Height)
	return nil
}

func (chain *Chain) updateStampingCertificate(height uint64) {
	if height <= chain.scStatus.Candidate {
		return
	}

	// delete fc
	// max(N-B, C+1, B+1)
	start := MaxUint64(height-defaultConfig.B+1, chain.scStatus.Candidate+1)
	n := chain.deleteFC(start, height)
	_ = n
	fmt.Printf("%d deleteFC range=[%d, %d] deleted=%d\n", height, start, height, n)

	if height-chain.scStatus.Proof <= defaultConfig.B {
		chain.scStatus.Candidate = height
	} else {
		chain.freeze(chain.scStatus.Proof)
		chain.scStatus.Proof = chain.scStatus.Candidate
		chain.scStatus.Candidate = height
	}

	// trim( max(QB - B, Fz), min((C-B), QB)) // 开区间
	start = MaxUint64(chain.scStatus.Proof-defaultConfig.B, chain.scStatus.Fz)
	end := MinUint64(chain.scStatus.Candidate-defaultConfig.B, chain.scStatus.Proof)
	n = chain.trim(start, end)
	fmt.Printf("trim range=[%d, %d] deleted=%d\n", start, end, n)

	return
}

//Keeping proof-objects up-to-date
func (chain *Chain) AddStampingCertificate(sc *StampingCertificate) error {
	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

	return chain.addStampingCertificate(sc)
}

func (chain *Chain) deleteFC(start, end uint64) int {
	count := 0
	for i := start; i <= end; i++ {
		if _, ok := chain.fcChain[i]; ok {
			delete(chain.fcChain, i)

			count += 1
		}
	}

	return count
}

func (chain *Chain) freeze(proof uint64) {
	chain.scStatus.Fz = proof
}

func (chain *Chain) trim(start, end uint64) int {
	count := 0
	for height := start + 1; height < end; height++ {
		if _, ok := chain.fcChain[height]; ok {
			panic(fmt.Sprintf("fc(%d) exist", height))
		}

		delete(chain.scChain, height)

		if h := chain.header(height); h != nil {
			count += 1
		}

		//chain.headerChain[height].Root = common.Hash{}
		delete(chain.headerChain, height)
	}

	return count
}

func (chain *Chain) Print() {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	realLength := uint64(0)
	prev := uint64(0)
	line := 0
	for height := uint64(1); height <= chain.currentHeight; height++ {
		header := chain.headerChain[height]
		fc := chain.fcChain[height]
		sc := chain.scChain[height]

		if header == nil {
			if fc != nil {
				panic(fmt.Sprintf("Unexpected! No header, but has FC, height=%d", height))
			}
			if sc != nil {
				panic(fmt.Sprintf("Unexpected! No header, but has SC, height=%d", height))
			}
			continue
		}

		realLength++

		fcTag := ""
		if fc != nil {
			fcTag = "F"

			if _, ok := chain.headerChain[height-1]; !ok {
				fcTag = "f"
			}
		}

		scTag := ""
		if sc != nil {
			scTag = "S"

			if _, ok := chain.headerChain[height-defaultConfig.B]; !ok {
				scTag = "s"
			}
		}

		arrow := ""
		if height > 0 && height-1 == prev {
			arrow = "<-"
		}
		prev = height

		zpcTag := ""
		switch height {
		case chain.scStatus.Fz:
			zpcTag = "Z"
		case chain.scStatus.Proof:
			zpcTag = "P"
		case chain.scStatus.Candidate:
			zpcTag = "C"
		}

		line += 1
		fmt.Printf("%2s[%4d(%1s%1s%1s)]", arrow, height, zpcTag, fcTag, scTag)
		if line >= 8 {
			fmt.Println()

			line = 0
		}
	}
	fmt.Println()

	fmt.Printf("Status: Fz: %d, Proof:%d, Candidate:%d\n", chain.scStatus.Fz, chain.scStatus.Proof, chain.scStatus.Candidate)
	fmt.Printf("MaxHeight: %d, realLength: %d, percent:%.2f%%\n", chain.currentHeight, realLength, float64(realLength*10000/chain.currentHeight)/100)
}

func (chain *Chain) Sync(other *Chain) error {
	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

	for height := uint64(1); height <= defaultConfig.B; height++ {
		header := other.Header(height)
		fc := other.FinalCertificate(height)

		if err := chain.addBlock(header, fc); err != nil {
			return err
		}
	}

	proofHeight := defaultConfig.B
	for height := proofHeight + defaultConfig.B; height <= other.scStatus.Candidate; {
		fmt.Printf("process h(%d), p(%d), ch:%d\n", height, proofHeight, other.currentHeight)
		scHeader, sc := other.HeaderAndStampingCertificate(height)
		if sc != nil {
			if scHeader == nil {
				panic(fmt.Errorf("other chain sc(%d) cannt find header(%d)", sc.Height, height))
			}
			for h := proofHeight - 1; h >= height-defaultConfig.B && h > proofHeight-defaultConfig.B; h-- {
				if h <= proofHeight-defaultConfig.B {
					panic("never here")
				}
				if chain.hasHeader(h) {
					continue
				}
				header := other.Header(h)
				if header == nil {
					return fmt.Errorf("rollback Header(%d) not exists", h)
				}
				if err := chain.addHeader(header); err != nil {
					return err
				}
				// fetch state
			}

			if err := chain.addStampingCertificateWithHeader(scHeader, sc); err != nil {
				// TODO: 并发处理还需要更一步处理
				return err
			}

			proofHeight = height
			height += defaultConfig.B
		} else {
			height -= 1
			if height > proofHeight {
				continue
			}

			for h := proofHeight + uint64(1); h <= other.scStatus.Fz; h++ {
				header := other.Header(h)
				fc := other.FinalCertificate(h)

				if err := chain.addBlock(header, fc); err != nil {
					return err
				}

				if _, sc := other.HeaderAndStampingCertificate(h + defaultConfig.B); sc != nil {
					proofHeight = h
					height = proofHeight + defaultConfig.B
					break
				}
			}

		}
	}

	// fz --> proof
	for height := other.scStatus.Proof - 1; height > other.scStatus.Fz && height >= other.scStatus.Candidate-defaultConfig.B; height-- {
		//fmt.Printf("fz->proof, h(%d)\n", height)
		header := other.Header(height)
		if err := chain.addHeader(header); err != nil {
			return err
		}
	}

	// proof --> C
	cHeader, cSc := other.HeaderAndStampingCertificate(other.scStatus.Candidate)
	if cHeader == nil || cSc == nil {
		return fmt.Errorf("cannt find header and sc, height:%d", other.scStatus.Candidate)
	}
	if err := chain.addStampingCertificateWithHeader(cHeader, cSc); err != nil {
		return err
	}
	for height := other.scStatus.Candidate - 1; height > other.scStatus.Proof; height-- {
		//fmt.Printf("C-->Proof, h(%d)\n", height)
		header := other.Header(height)
		if err := chain.addHeader(header); err != nil {
			return err
		}
	}

	// dense tail
	for height := other.scStatus.Candidate + 1; height <= other.currentHeight; height++ {
		header := other.Header(height)
		if header == nil {
			return fmt.Errorf("dense tail header(%d) not exists", height)
		}
		fc := other.FinalCertificate(height)
		if fc == nil {
			return fmt.Errorf("dense tail fc(%d) not exists", height)
		}
		if err := chain.addBlock(header, fc); err != nil {
			return err
		}
	}
	return nil
}
