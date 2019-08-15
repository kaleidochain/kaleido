package stamping

import (
	"crypto/sha512"
	"fmt"
	"sync"

	"github.com/kaleidochain/kaleido/common"
)

var (
	defaultConfig = &Config{
		B:           8,
		Probability: 65,
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
	return sc.Height == header.Height &&
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

func (chain *Chain) Header(height uint64) *Header {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	return chain.headerChain[height]
}

func (chain *Chain) header(height uint64) *Header {
	return chain.headerChain[height]
}

func (chain *Chain) AddBlock(header *Header, fc *FinalCertificate) error {
	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

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

func (chain *Chain) AddStampingCertificate(sc *StampingCertificate) error {
	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

	header := chain.header(sc.Height)
	if header == nil {
		return fmt.Errorf("header(%d) not exists", sc.Height)
	}
	if _, ok := chain.scChain[sc.Height]; ok {
		return fmt.Errorf("stampingCertificate(%d) exists", sc.Height)
	}
	if sc.Height <= defaultConfig.B {
		return fmt.Errorf("stampingCertificate(%d) is lower than B(%d)", sc.Height, defaultConfig.B)
	}

	proofHeader := chain.header(sc.Height - defaultConfig.B)
	if proofHeader == nil {
		return fmt.Errorf("proof header(%d) not exists", sc.Height-defaultConfig.B)
	}

	if !sc.Verify(header, proofHeader) {
		return fmt.Errorf("sc(%d) invalid", sc.Height)
	}

	return chain.addStampingCertificate(sc)
}

//Keeping proof-objects up-to-date
func (chain *Chain) addStampingCertificate(sc *StampingCertificate) error {
	if sc.Height <= chain.scStatus.Candidate {
		return fmt.Errorf("sc(%d) lower than Candidate(%d)", sc.Height, chain.scStatus.Candidate)
	}

	chain.scChain[sc.Height] = sc

	// delete fc
	// max(N-B, C+1, B+1)
	start := MaxUint64(sc.Height-chain.scStatus.Proof, chain.scStatus.Candidate+1)
	n := chain.deleteFC(start, sc.Height)
	fmt.Printf("deleteFC range=[%d, %d] deleted=%d\n", start, sc.Height, n)

	if sc.Height-chain.scStatus.Proof <= defaultConfig.B {
		chain.scStatus.Candidate = sc.Height
	} else {
		chain.freeze(chain.scStatus.Proof)
		chain.scStatus.Proof = chain.scStatus.Candidate
		chain.scStatus.Candidate = sc.Height
	}

	// trim( max(QB - B, Fz), min((C-B), QB)) // 开区间
	start = MaxUint64(chain.scStatus.Proof-defaultConfig.B, chain.scStatus.Fz)
	end := MinUint64(chain.scStatus.Candidate-defaultConfig.B, chain.scStatus.Proof)
	n = chain.trim(start, end)
	fmt.Printf("trim range=[%d, %d] deleted=%d\n", start, end, n)

	return nil
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
	for height := uint64(0); height <= chain.currentHeight; height++ {
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
