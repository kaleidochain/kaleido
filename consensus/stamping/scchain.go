package stamping

import (
	"crypto/sha512"
	"fmt"
	"sync"

	"github.com/kaleidochain/kaleido/common"
)

var (
	defaultConfig = &Config{
		B:                  2000,
		FailureProbability: 65,
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
	B                  uint64
	FailureProbability int
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
	Hash       common.Hash
	ParentSeed common.Hash
	ParentRoot common.Hash
}

func NewFinalCertificate(header, parent *Header) *FinalCertificate {
	return &FinalCertificate{
		Height:     header.Height,
		Hash:       header.Hash(),
		ParentSeed: parent.Seed,
		ParentRoot: parent.Root,
	}
}

func (fc *FinalCertificate) Verify(header, parent *Header) bool {
	return fc.Height == header.Height &&
		fc.Hash == header.Hash() &&
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

func (sc *StampingCertificate) Verify(config *Config, header, proofHeader *Header) bool {
	return sc.Height > config.B &&
		sc.Height == header.Height &&
		sc.Height == proofHeader.Height+config.B &&
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
	config      *Config
	headerChain map[uint64]*Header
	mutexChain  sync.RWMutex
	fcChain     map[uint64]*FinalCertificate
	scChain     map[uint64]*StampingCertificate

	currentHeight uint64
	scStatus      SCStatus
}

func NewChain(config *Config) *Chain {
	chain := &Chain{
		config:      config,
		headerChain: make(map[uint64]*Header),
		fcChain:     make(map[uint64]*FinalCertificate),
		scChain:     make(map[uint64]*StampingCertificate),
		scStatus: SCStatus{
			Candidate: config.B,
			Proof:     config.B,
			Fz:        config.B,
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

func (chain *Chain) finalCertificate(height uint64) *FinalCertificate {
	return chain.fcChain[height]
}

func (chain *Chain) HeaderAndStampingCertificate(height uint64) (*Header, *StampingCertificate) {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	return chain.headerChain[height], chain.scChain[height]
}

func (chain *Chain) StampingCertificate(height uint64) *StampingCertificate {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	return chain.stampingCertificate(height)
}

func (chain *Chain) stampingCertificate(height uint64) *StampingCertificate {
	return chain.scChain[height]
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
		return fmt.Errorf("scheader(%d) exists", sc.Height)
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

	proofHeader, ok := chain.headerChain[sc.Height-chain.config.B]
	if !ok {
		return fmt.Errorf("proof header(%d) not exists", sc.Height-chain.config.B)
	}

	if !sc.Verify(chain.config, header, proofHeader) {
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
	start := MaxUint64(height-chain.config.B+1, chain.scStatus.Candidate+1)
	chain.deleteFC(start, height)

	if height-chain.scStatus.Proof <= chain.config.B {
		chain.scStatus.Candidate = height
	} else {
		chain.freezeProof()
		chain.scStatus.Proof = chain.scStatus.Candidate
		chain.scStatus.Candidate = height
	}

	// trim( max(QB - B, Fz), min((C-B), QB)) // 开区间
	start = MaxUint64(chain.scStatus.Proof-chain.config.B, chain.scStatus.Fz)
	end := MinUint64(chain.scStatus.Candidate-chain.config.B, chain.scStatus.Proof)
	n := chain.trim(start, end)
	_ = n
	//fmt.Printf("trim range=[%d, %d] trimmed=%d/%d\n", start, end, n, end-start-1)

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

func (chain *Chain) freezeProof() {
	start := MaxUint64(chain.scStatus.Fz+1, chain.scStatus.Proof-chain.config.B+1)
	end := chain.scStatus.Proof
	for height := start; height < end; height++ {
		delete(chain.scChain, height)
	}

	chain.scStatus.Fz = chain.scStatus.Proof
}

func (chain *Chain) trim(start, end uint64) int {
	count := 0
	for height := end - 1; height > start; height-- {
		if fc := chain.fcChain[height]; fc != nil {
			panic(fmt.Sprintf("fc(%d) should already be deleted", height))
		}

		if chain.headerChain[height] == nil && chain.scChain[height] == nil {
			break
		}

		delete(chain.scChain, height)
		delete(chain.headerChain, height)
		count += 1
	}

	return count
}

func (chain *Chain) Print() {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	count := chain.printRange(1, chain.currentHeight+1)

	fmt.Printf("Status: Fz=%d, Proof=%d, Candidate=%d\n", chain.scStatus.Fz, chain.scStatus.Proof, chain.scStatus.Candidate)
	fmt.Printf("MaxHeight=%d, realLength=%d, percent=%.2f%%\n", chain.currentHeight, count, float64(count*10000/chain.currentHeight)/100)
}

func (chain *Chain) PrintFrozenBreadcrumbs() {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	begin := chain.config.B + 1
	end := chain.scStatus.Fz + 1
	count := chain.printRange(begin, end)

	fmt.Printf("Status: Fz=%d, Proof=%d, Candidate=%d\n", chain.scStatus.Fz, chain.scStatus.Proof, chain.scStatus.Candidate)
	fmt.Printf("RangeLength=%d, realLength=%d, percent=%.2f%%\n", end-begin, count, float64(count*10000/(end-begin))/100)
}

func (chain *Chain) printRange(begin, end uint64) uint64 {
	const perLine = 8

	lastPrinted := uint64(0)
	count := uint64(0)
	for height := begin; height < end; height++ {
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

		hasParent := lastPrinted == height-1
		lastPrinted = height

		fmt.Printf("%s", chain.formatHeader(height, fc, sc, hasParent))
		if count++; count%perLine == 0 {
			fmt.Println()
		}
	}

	if count%perLine != 0 {
		fmt.Println()
	}

	return count
}

func (chain *Chain) formatHeader(height uint64, fc *FinalCertificate, sc *StampingCertificate, hasParent bool) string {
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

		if _, ok := chain.headerChain[height-chain.config.B]; !ok {
			scTag = "s"
		}
	}

	arrow := ""
	if hasParent {
		arrow = "<-"
	}

	zpcTag := ""
	switch height {
	case chain.scStatus.Fz:
		zpcTag = "Z"
	case chain.scStatus.Proof:
		zpcTag = "P"
	case chain.scStatus.Candidate:
		zpcTag = "C"
	}

	return fmt.Sprintf("%2s[%4d(%1s%1s%1s)]", arrow, height, zpcTag, fcTag, scTag)
}

func (chain *Chain) HeaderAndFinalCertificate(height uint64) (*Header, *FinalCertificate) {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	return chain.header(height), chain.finalCertificate(height)
}

func (chain *Chain) canSynchronize(other *Chain) bool {
	return *chain.config == *other.config &&
		chain.headerChain[0].Hash() == other.headerChain[0].Hash()
}

func (chain *Chain) syncRangeByHeaderAndFinalCertificate(peer *Chain, start, end uint64) error {
	for height := start; height <= end && height <= peer.currentHeight; height++ {
		header := peer.Header(height)
		if header == nil {
			return fmt.Errorf("cannt find header(%d)", height)
		}
		fc := peer.FinalCertificate(height)
		if fc == nil {
			return fmt.Errorf("cannt find fc(%d)", height)
		}

		if err := chain.addBlock(header, fc); err != nil {
			return err
		}
	}

	return nil
}

func (chain *Chain) getNextBreadcrumb(begin, end uint64) (*breadcrumb, error) {
	bc := &breadcrumb{}
	for height := end; height >= begin; height-- {
		if sc := chain.stampingCertificate(height); sc != nil {
			header := chain.header(height)
			if header == nil {
				panic(fmt.Sprintf("cannot find header(%d)", height))
			}

			bc.stampingHeader = header
			bc.stampingCertificate = sc

			// rollback tail
			for h := height - 1; h >= begin; h-- {
				header := chain.header(h)
				if header == nil {
					break
				}

				bc.tail = append(bc.tail, header)
			}

			return bc, nil
		}
	}

	for height := begin; height <= chain.currentHeight; height++ {
		header := chain.header(height)
		if header == nil {
			panic(fmt.Sprintf("cannot find header(%d)", height))
		}
		fc := chain.finalCertificate(height)
		if fc == nil {
			panic(fmt.Sprintf("cannot find fc(%d)", height))
		}

		bc.forwardHeader = append(bc.forwardHeader, header)
		bc.forwardFinalCertificate = append(bc.forwardFinalCertificate, fc)

		forwardHeight := height + chain.config.B
		if sc := chain.stampingCertificate(forwardHeight); sc != nil {
			break
		}
	}

	return bc, nil
}

type breadcrumb struct {
	stampingHeader      *Header
	stampingCertificate *StampingCertificate
	tail                []*Header

	forwardHeader           []*Header
	forwardFinalCertificate []*FinalCertificate
}

func (chain *Chain) syncNextBreadcrumb(peer *Chain, begin, end uint64) (nextBegin, nextEnd uint64, err error) {
	breadcrumb, err := peer.getNextBreadcrumb(begin, end)
	if err != nil {
		return
	}

	if breadcrumb.stampingHeader != nil {
		err = chain.addStampingCertificateWithHeader(breadcrumb.stampingHeader, breadcrumb.stampingCertificate)
		if err != nil {
			return
		}

		for _, tailHeader := range breadcrumb.tail {
			err = chain.addHeader(tailHeader)
			if err != nil {
				return
			}
		}

		nextBegin = chain.currentHeight + 1
		nextEnd = (chain.currentHeight - uint64(len(breadcrumb.tail))) + chain.config.B
	} else {
		for i, h := range breadcrumb.forwardHeader {
			fc := breadcrumb.forwardFinalCertificate[i]
			err = chain.addBlock(h, fc)
			if err != nil {
				return
			}
		}

		nextBegin = chain.currentHeight + 1
		nextEnd = chain.currentHeight + chain.config.B
	}

	return
}

func (chain *Chain) Sync(peer *Chain) error {
	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

	if !chain.canSynchronize(peer) {
		return fmt.Errorf("cannot synchronize from this chain")
	}

	// 0 - B
	err := chain.syncRangeByHeaderAndFinalCertificate(peer, 1, chain.config.B)
	if err != nil {
		return fmt.Errorf("synchronize the first b blocks failed: %v", err)
	}

	// B - C
	for begin, end := chain.scStatus.Fz+1, chain.scStatus.Fz+chain.config.B; chain.currentHeight < peer.currentHeight; {
		nextBegin, nextEnd, err := chain.syncNextBreadcrumb(peer, begin, end)
		if err != nil {
			return fmt.Errorf("synchronize frozen breadcrumb in range[%d,%d] failed: %v", begin, end, err)
		}

		begin, end = nextBegin, nextEnd
	}

	return nil
}

func (chain *Chain) SyncBase(peer *Chain) error {
	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

	if !chain.canSynchronize(peer) {
		return fmt.Errorf("cannot synchronize from this chain")
	}

	err := chain.syncRangeByHeaderAndFinalCertificate(peer, 1, chain.config.B)
	if err != nil {
		return fmt.Errorf("synchronize the first b blocks failed: %v", err)
	}

	proofHeight := chain.config.B
	for height := proofHeight + chain.config.B; peer.scStatus.Fz > chain.config.B; {
		//fmt.Printf("process h(%d), proofHeight(%d)\n", height, proofHeight)
		scHeader, sc := peer.HeaderAndStampingCertificate(height)
		if sc != nil {
			if scHeader == nil {
				panic(fmt.Errorf("other chain sc(%d) cannt find header(%d)", sc.Height, height))
			}
			for h := proofHeight - 1; h >= height-chain.config.B && h > proofHeight-chain.config.B; h-- {
				if h <= proofHeight-chain.config.B {
					panic("never here")
				}
				if chain.hasHeader(h) {
					continue
				}
				header := peer.Header(h)
				if header == nil {
					return fmt.Errorf("rollback Header(%d) not exists", h)
				}
				//fmt.Printf("chain.addHeader(%d)\n", header.Height)
				if err := chain.addHeader(header); err != nil {
					return err
				}
			}

			//fmt.Printf("chain.addsc(%d)\n", scHeader.Height)
			if err := chain.addStampingCertificateWithHeader(scHeader, sc); err != nil {
				// TODO: 并发处理还需要更一步处理
				return err
			}

			proofHeight = height
			if height == peer.scStatus.Fz {
				break
			}

			height += chain.config.B
			if height > peer.scStatus.Fz {
				height = peer.scStatus.Fz
			}
		} else {
			height -= 1
			if height > proofHeight {
				continue
			}

			for h := proofHeight + uint64(1); ; h++ {
				header := peer.Header(h)
				fc := peer.FinalCertificate(h)

				if err := chain.addBlock(header, fc); err != nil {
					return err
				}

				//fmt.Printf("chain.addBlock(%d)\n", header.Height)
				if _, sc := peer.HeaderAndStampingCertificate(h + chain.config.B); sc != nil {
					proofHeight = h
					height = proofHeight + chain.config.B
					break
				}

				if h+chain.config.B >= peer.scStatus.Fz {
					return fmt.Errorf("other.SC(Fz) not exists, h:%d, other.Fz:%d", h, peer.scStatus.Fz)
				}
			}
		}
	}
	// fz tail
	for h := peer.scStatus.Fz - 1; h >= peer.scStatus.Proof-chain.config.B && peer.scStatus.Fz > chain.config.B; h-- {
		header := peer.Header(h)
		if header == nil {
			return fmt.Errorf("rollback Header(%d) not exists", h)
		}
		//fmt.Printf("fz tail chain.addHeader(%d)\n", header.Height)
		if err := chain.addHeader(header); err != nil {
			return err
		}
	}

	// fz <-- proof
	for height := peer.scStatus.Fz + 1; height <= peer.scStatus.Proof-chain.config.B; height++ {
		header := peer.Header(height)
		if header == nil {
			return fmt.Errorf("fz <-- proof header(%d) not exists", height)
		}
		fc := peer.FinalCertificate(height)
		if fc == nil {
			return fmt.Errorf("dense tail fc(%d) not exists", height)
		}
		if err := chain.addBlock(header, fc); err != nil {
			return err
		}
	}
	if peer.scStatus.Proof > chain.config.B {
		pHeader, pSc := peer.HeaderAndStampingCertificate(peer.scStatus.Proof)
		if pHeader == nil || pSc == nil {
			return fmt.Errorf("cannt find proof header and sc, height:%d", peer.scStatus.Proof)
		}
		if err := chain.addStampingCertificateWithHeader(pHeader, pSc); err != nil {
			return err
		}
	}
	for height := peer.scStatus.Proof - 1; height >= peer.scStatus.Candidate-chain.config.B && height > peer.scStatus.Proof-chain.config.B && height > peer.scStatus.Fz; height-- {
		//fmt.Printf("fz->proof, h(%d), p(%d)\n", height, other.scStatus.Proof)
		header := peer.Header(height)
		if header == nil {
			return fmt.Errorf("header is nil, height:%d", height)
		}
		if err := chain.addHeader(header); err != nil {
			return err
		}
	}

	// proof <-- C
	for height := peer.scStatus.Proof + 1; height <= peer.scStatus.Candidate-chain.config.B; height++ {
		header := peer.Header(height)
		if header == nil {
			return fmt.Errorf("proof <-- C header(%d) not exists", height)
		}
		fc := peer.FinalCertificate(height)
		if fc == nil {
			return fmt.Errorf("dense tail fc(%d) not exists", height)
		}
		if err := chain.addBlock(header, fc); err != nil {
			return err
		}
	}
	if peer.scStatus.Candidate > chain.config.B {
		cHeader, cSc := peer.HeaderAndStampingCertificate(peer.scStatus.Candidate)
		if cHeader == nil || cSc == nil {
			return fmt.Errorf("cannt find header and sc, height:%d", peer.scStatus.Candidate)
		}
		if err := chain.addStampingCertificateWithHeader(cHeader, cSc); err != nil {
			return err
		}
	}
	for height := peer.scStatus.Candidate - 1; height > peer.scStatus.Candidate-chain.config.B && height > peer.scStatus.Proof; height-- {
		//fmt.Printf("C-->Proof, h(%d)\n", height)
		header := peer.Header(height)
		if err := chain.addHeader(header); err != nil {
			return err
		}
	}

	// dense tail
	for height := peer.scStatus.Candidate + 1; height <= peer.currentHeight; height++ {
		header := peer.Header(height)
		if header == nil {
			return fmt.Errorf("dense tail header(%d) not exists", height)
		}
		fc := peer.FinalCertificate(height)
		if fc == nil {
			return fmt.Errorf("dense tail fc(%d) not exists", height)
		}
		if err := chain.addBlock(header, fc); err != nil {
			return err
		}
	}
	return nil
}

func (chain *Chain) Equal(other *Chain) (bool, error) {
	if chain.scStatus != other.scStatus {
		return false, fmt.Errorf("status not equal, this:%v, other:%v", chain.scStatus, other.scStatus)
	}

	for height := uint64(1); height <= chain.scStatus.Fz; height++ {
		header := chain.header(height)
		oHeader := other.Header(height)
		if !EqualHeader(header, oHeader) {
			return false, fmt.Errorf("header or other not exists, height:%d, this:%v, other:%v", height, header, oHeader)
		}

		fc := chain.finalCertificate(height)
		ofc := other.FinalCertificate(height)
		if !EqualFinalCertificate(fc, ofc) {
			return false, fmt.Errorf("fc not equal, height:%d, this:%v, other:%v", height, fc, ofc)
		}

		sc := chain.stampingCertificate(height)
		osc := other.StampingCertificate(height)
		if !EqualStampingCertificate(sc, osc) {
			return false, fmt.Errorf("sc not equal, height:%d, this:%v, other:%v", height, sc, osc)
		}
	}

	for height := chain.scStatus.Fz + 1; height < chain.currentHeight; height++ {
		if height == chain.scStatus.Proof || height == chain.scStatus.Candidate {
			continue
		}

		header := chain.header(height)
		oHeader := other.Header(height)
		if !EqualHeader(header, oHeader) {
			return false, fmt.Errorf("header or other not exists, height:%d, this:%v, other:%v", height, header, oHeader)
		}

		fc := chain.finalCertificate(height)
		ofc := other.FinalCertificate(height)
		if !EqualFinalCertificate(fc, ofc) {
			return false, fmt.Errorf("fc not equal, height:%d, this:%v, other:%v", height, fc, ofc)
		}
	}
	// proof
	{
		height := chain.scStatus.Proof

		header := chain.header(height)
		oHeader := other.Header(height)
		if !EqualHeader(header, oHeader) {
			return false, fmt.Errorf("header or other not exists, height:%d, this:%v, other:%v", height, header, oHeader)
		}

		fc := chain.finalCertificate(height)
		ofc := other.FinalCertificate(height)
		if !EqualFinalCertificate(fc, ofc) {
			return false, fmt.Errorf("fc not equal, height:%d, this:%v, other:%v", height, fc, ofc)
		}

		sc := chain.stampingCertificate(height)
		osc := other.StampingCertificate(height)
		if !EqualStampingCertificate(sc, osc) {
			return false, fmt.Errorf("sc not equal, height:%d, this:%v, other:%v", height, sc, osc)
		}
	}

	// C
	{
		height := chain.scStatus.Candidate

		header := chain.header(height)
		oHeader := other.Header(height)
		if !EqualHeader(header, oHeader) {
			return false, fmt.Errorf("header or other not exists, height:%d, this:%v, other:%v", height, header, oHeader)
		}

		fc := chain.finalCertificate(height)
		ofc := other.FinalCertificate(height)
		if !EqualFinalCertificate(fc, ofc) {
			return false, fmt.Errorf("fc not equal, height:%d, this:%v, other:%v", height, fc, ofc)
		}

		sc := chain.stampingCertificate(height)
		osc := other.StampingCertificate(height)
		if !EqualStampingCertificate(sc, osc) {
			return false, fmt.Errorf("sc not equal, height:%d, this:%v, other:%v", height, sc, osc)
		}
	}

	return true, nil
}

func EqualHeader(a, b *Header) bool {
	if a == nil && b == nil {
		return true
	}

	if a != nil && b != nil {
		return *a == *b
	}

	return false
}

func EqualFinalCertificate(a, b *FinalCertificate) bool {
	if a == nil && b == nil {
		return true
	}

	if a != nil && b != nil {
		return *a == *b
	}

	return false
}

func EqualStampingCertificate(a, b *StampingCertificate) bool {
	if a == nil && b == nil {
		return true
	}

	if a != nil && b != nil {
		return *a == *b
	}

	return false
}
