package stamping

import (
	"crypto/sha512"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/log"
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
	BaseHeight         uint64      // from the height Vault starts, for a new chain, it's 0
	BaseHash           common.Hash // hash of the BaseHeight header
	StampingThreshold  uint64
	Address            common.Address
}

func (c Config) HeightB() uint64 {
	return c.BaseHeight + c.B
}

func (c Config) InitialStampingStatus() SCStatus {
	return SCStatus{
		Candidate: c.HeightB(),
		Proof:     c.HeightB(),
		Fz:        c.HeightB(),
	}
}

type TroubleMaker interface {
	Trouble() bool
}

var ErrRandomTrouble = fmt.Errorf("random trouble hit")

type RandomTroubleMaker int

func (r RandomTroubleMaker) Trouble() bool {
	return rand.Intn(100) <= int(r)
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

const checkNewSCInterval = 30 * time.Second
const stampingVoteCandidateTerm = 30
const gossipMaxHeightDiff = 100

type StatusMsg struct {
	SCStatus
	Height uint64
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

	name string

	id           string
	chains       []*Chain
	archive      *Chain
	troubleMaker TroubleMaker

	messageChan                chan message
	buildingStampingVoteWindow map[uint64]*StampingVotes
	maxVoteHeight              uint64
	checkNewInterval           uint64
	checkNewTicker             *time.Ticker
	counter                    *HeightVoteSet

	peers []*peer
}

func NewChain(config *Config) *Chain {
	chain := &Chain{
		config:      config,
		headerChain: make(map[uint64]*Header),
		fcChain:     make(map[uint64]*FinalCertificate),
		scChain:     make(map[uint64]*StampingCertificate),
		scStatus:    config.InitialStampingStatus(),
	}
	chain.headerChain[0] = genesisHeader
	chain.messageChan = make(chan message, msgChanSize)
	chain.buildingStampingVoteWindow = make(map[uint64]*StampingVotes)
	chain.checkNewTicker = time.NewTicker(checkNewSCInterval)
	chain.counter = NewHeightVoteSet()

	return chain
}

func (chain *Chain) SetTroubleMaker(t TroubleMaker) {
	chain.troubleMaker = t
}

func (chain *Chain) SetName(name string) {
	chain.name = name
}

func (chain *Chain) AddPeer(peer *peer) {
	chain.peers = append(chain.peers, peer)

	go chain.gossipVote(peer)
	go peer.handleMsg()
}

func (chain *Chain) AddPeerChain(peer *Chain) {
	peer.id = fmt.Sprintf("%d", len(chain.chains))
	chain.chains = append(chain.chains, peer)
}

func (chain *Chain) AddArchiveChain(archivePeer *Chain) {
	archivePeer.id = "archive"
	chain.archive = archivePeer
}

func (chain *Chain) getPeer() *Chain {
	return chain.chains[rand.Intn(len(chain.chains))]
}
func (chain *Chain) getArchivePeer() *Chain {
	return chain.archive
}

func (chain *Chain) Log() log.Logger {
	return log.New("chain", chain.name)
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

func (chain *Chain) addHeaderWithHash(header *Header, hash common.Hash) error {
	if header.Hash() != hash {
		return fmt.Errorf("invalid header(%d): hash not matched, expect %s but got %s",
			header.Height, hash, header.Hash())
	}
	if _, ok := chain.headerChain[header.Height]; ok {
		return fmt.Errorf("header(%d) already exists", header.Height)
	}

	chain.headerChain[header.Height] = header
	return nil
}

func (chain *Chain) addHeader(header *Header) error {
	if _, ok := chain.headerChain[header.Height]; ok {
		return fmt.Errorf("header(%d) already exists", header.Height)
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
	headerEnd := MinUint64(chain.scStatus.Candidate-chain.config.B, chain.scStatus.Proof)
	end := chain.scStatus.Proof

	//trim the tail of P to keep its length minimal
	for height := start; height < headerEnd; height++ {
		delete(chain.scChain, height)
		delete(chain.headerChain, height)
	}

	// delete sc from the minimal tail
	for height := headerEnd; height < end; height++ {
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

	fmt.Printf("Miner=%s\n", chain.config.Address.String())
	fmt.Printf("Status: Fz=%d, Proof=%d, Candidate=%d\n", chain.scStatus.Fz, chain.scStatus.Proof, chain.scStatus.Candidate)
	fmt.Printf("MaxHeight=%d, realLength=%d, percent=%.2f%%\n", chain.currentHeight, count, float64(count*10000/chain.currentHeight)/100)
}

func (chain *Chain) PrintFrozenBreadcrumbs() {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	begin := chain.config.HeightB() + 1
	end := chain.scStatus.Fz + 1
	count := chain.printRange(begin, end)

	fmt.Printf("Status: Fz=%d, Proof=%d, Candidate=%d\n", chain.scStatus.Fz, chain.scStatus.Proof, chain.scStatus.Candidate)
	fmt.Printf("RangeLength=%d, realLength=%d, percent=%.2f%%\n", end-begin, count, float64(count*10000/(end-begin))/100)
}

func (chain *Chain) PrintProperty() {
	countBreadcrumb := 0
	countTail := 0
	sumTailLength := 0
	countForward := 0
	sumForwardLength := 0

	for begin, end := chain.config.HeightB()+1, chain.config.HeightB()+chain.config.B; end <= chain.scStatus.Fz; {
		breadcrumb, err := chain.getNextBreadcrumb(begin, end)
		if err != nil {
			panic("invalid chain")
		}

		countBreadcrumb++
		if breadcrumb.stampingHeader != nil {
			if n := len(breadcrumb.tail); n > 0 {
				countTail++
				sumTailLength += n
			}

			begin = breadcrumb.stampingHeader.Height + 1
			end = (breadcrumb.stampingHeader.Height - uint64(len(breadcrumb.tail))) + chain.config.B
		} else {
			countForward++
			sumForwardLength += len(breadcrumb.forwardHeader)

			lastOne := breadcrumb.forwardHeader[len(breadcrumb.forwardHeader)-1]
			begin = lastOne.Height + 1
			end = lastOne.Height + chain.config.B
		}
	}

	avgTailLen := 0.0
	if countTail > 0 {
		avgTailLen = float64(sumTailLength) / float64(countTail)
	}
	avgForwardLen := 0.0
	if countForward > 0 {
		avgForwardLen = float64(sumForwardLength) / float64(countForward)
	}
	fmt.Printf("#Breadcrum=%d, #Tail=%d, TailLenTotal=%d, avgTailLen=%f, #Forward=%d, ForwardLenTotal=%d, avgForwardLen=%f\n",
		countBreadcrumb, countTail, sumTailLength, avgTailLen, countForward, sumForwardLength, avgForwardLen)
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

func (chain *Chain) StatusString() string {
	return fmt.Sprintf("%d/%d/%d/%d", chain.scStatus.Fz, chain.scStatus.Proof, chain.scStatus.Candidate, chain.currentHeight)
}

func (chain *Chain) ChainStatus() StatusMsg {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	return StatusMsg{
		SCStatus: chain.scStatus,
		Height:   chain.currentHeight,
	}
}

func (chain *Chain) canSynchronize(other *Chain) bool {
	return *chain.config == *other.config &&
		chain.headerChain[0].Hash() == other.headerChain[0].Hash()
}

func (chain *Chain) forwardSyncRangeByHeaderAndFinalCertificate(peer *Chain, start, end uint64) error {
	for height := start; height <= end && height <= peer.currentHeight; height++ {
		if chain.hasHeader(height) {
			continue
		}

		if chain.troubleMaker != nil && chain.troubleMaker.Trouble() {
			return ErrRandomTrouble
		}

		header, fc := peer.HeaderAndFinalCertificate(height)
		if header == nil || fc == nil {
			return fmt.Errorf("peer has no header or fc at height(%d)", height)
		}

		if err := chain.addBlock(header, fc); err != nil {
			return err
		}
	}

	return nil
}

func (chain *Chain) backwardSyncRangeOnlyByHeader(peer *Chain, start, end uint64, endHash common.Hash) error {
	for height := end; height >= start; height-- {
		if chain.hasHeader(height) {
			continue
		}

		header := peer.Header(height)
		if header == nil {
			return fmt.Errorf("peer has no header(%d)", height)
		}

		var err error
		if height == end {
			err = chain.addHeaderWithHash(header, endHash)
		} else {
			err = chain.addHeader(header)
		}
		if err != nil {
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

func (chain *Chain) getAllHeaders(begin, end uint64) (headers []*Header) {
	for height := end; height >= begin; height-- {
		header := chain.header(height)
		if header == nil {
			headers = nil
			return
		}

		headers = append(headers, header)
	}

	return
}

type breadcrumb struct {
	stampingHeader      *Header
	stampingCertificate *StampingCertificate
	tail                []*Header

	forwardHeader           []*Header
	forwardFinalCertificate []*FinalCertificate
}

func (chain *Chain) syncNextBreadcrumb(peer *Chain, begin, end uint64) (nextBegin, nextEnd uint64, err error) {
	var bc *breadcrumb
	bc, err = peer.getNextBreadcrumb(begin, end)
	if err != nil {
		return
	}

	if bc.stampingHeader != nil {
		if proofHeader := chain.header(bc.stampingHeader.Height - chain.config.B); proofHeader == nil {
			tailLength := chain.getTailLength(begin)
			neededBegin := bc.stampingHeader.Height - chain.config.B
			neededEnd := begin - 1 - tailLength
			if err = chain.syncAllHeaders(peer, neededBegin, neededEnd); err != nil {
				// TODO: switch to full node
				archive := chain.getArchivePeer()
				if err = chain.syncAllHeaders(archive, neededBegin, neededEnd); err != nil {
					/*fmt.Printf("syncAllHeaders, peer:%s, begin:end=[%d, %d] [%d, %d], proof:%d, tailLength:%d\n",
					peer.id, begin, end, neededBegin, neededEnd, bc.stampingHeader.Height-chain.config.B, tailLength)*/
					return
				}
			}
		}

		err = chain.addStampingCertificateWithHeader(bc.stampingHeader, bc.stampingCertificate)
		if err != nil {
			return
		}

		for _, tailHeader := range bc.tail {
			err = chain.addHeader(tailHeader)
			if err != nil {
				return
			}
		}

		nextBegin = chain.currentHeight + 1
		nextEnd = (chain.currentHeight - uint64(len(bc.tail))) + chain.config.B
	} else {
		for i, h := range bc.forwardHeader {
			fc := bc.forwardFinalCertificate[i]
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

func (chain *Chain) getTailLength(height uint64) (length uint64) {
	for h := height - 1; h > height-chain.config.B; h-- {
		if _, ok := chain.headerChain[h]; !ok {
			return
		}
		length += 1
	}
	return
}

func (chain *Chain) syncAllHeaders(peer *Chain, begin, end uint64) (err error) {
	headers := peer.getAllHeaders(begin, end)
	if len(headers) == 0 || headers[len(headers)-1].Height != begin {
		err = fmt.Errorf("peer do not have 'begin(%d)'", begin)
		return
	}

	for _, tailHeader := range headers {
		err = chain.addHeader(tailHeader)
		if err != nil {
			return
		}
	}
	return
}

func (chain *Chain) AutoBuildSCVote(buildVote bool) {
	generateSCInterval := 5 * time.Second
	generateSCTicker := time.NewTicker(generateSCInterval)
	go func() {
		for {

			select {
			case <-generateSCTicker.C:
				status := chain.ChainStatus()
				newHeight := status.Height + 1

				parent := chain.Header(status.Height)
				header := NewHeader(newHeight, parent)
				finalCertificate := NewFinalCertificate(header, parent)

				err := chain.AddBlock(header, finalCertificate)
				if err != nil {
					panic(fmt.Sprintf("AddBlock failed, height=%d, err=%v", newHeight, err))
				}
				if !buildVote {
					continue
				}

				weight := uint64(rand.Int63n(int64(chain.config.StampingThreshold))) + 30
				vote := NewStampingVote(newHeight, chain.config.Address, weight)

				chain.Log().Trace("Timer SendSCVote", "Status", chain.StatusString(), "vote", vote.String())

				chain.sendToMessageChan(message{
					code: StampingVoteMsg,
					data: vote,
					from: chain.name,
				})

				if err := chain.broadcastMessage(message{
					code: StampingVoteMsg,
					data: vote,
					from: chain.name,
				}); err != nil {
					chain.Log().Error("broadcast err", "err", err)
				}
			}
		}
	}()
}

func (chain *Chain) sendToMessageChan(msg message) {
	select {
	case chain.messageChan <- msg:
		return
	default:
		chain.Log().Error("message chan full", "size", len(chain.messageChan))
		return
	}
}

func (chain *Chain) broadcastMessage(msg message) error {
	switch msg.code {
	case StampingStatusMsg:
		for _, peer := range chain.peers {
			status := msg.data.(*StatusMsg)
			peer.SendStatus(*status)
		}
	case StampingVoteMsg:
		for _, peer := range chain.peers {
			vote := msg.data.(*StampingVote)
			peer.SendSCVote(vote)
		}
	}

	return nil
}

func (chain *Chain) SendMsgToPeer(msg message) {
	chain.sendToMessageChan(msg)
}

func (chain *Chain) Start() {
	go chain.handleMsg()
}

func sendToMessageChan(ch chan<- message, msg message) {
	select {
	case ch <- msg:
		return
	default:
		log.Error("message chan full", "size", len(ch))
		return
	}
}

func (chain *Chain) OnReceive(code uint64, data interface{}, from string) {
	sendToMessageChan(chain.messageChan, message{code, data, from})
}

func (chain *Chain) handleMsg() {
	for {
		select {
		case msg := <-chain.messageChan:
			switch msg.code {
			case StampingVoteMsg:
				vote := msg.data.(*StampingVote)
				if err := chain.handleStampingVote(vote); err != nil {
					chain.Log().Error("handle vote failed", "vote", vote, "from", msg.from, "err", err)
				}
			}
		case <-chain.checkNewTicker.C:
			if err := chain.checkEnoughVotesAndAddToSCChain(); err != nil {
				chain.Log().Error("handle check new stampingcertificate failed", "err", err)
			}
		}
	}
}

func (chain *Chain) handleStampingVote(vote *StampingVote) error {
	// verify
	chain.Log().Trace("handleStampingVote", "vote", vote)

	scStatus := chain.ChainStatus()
	if vote.Height <= scStatus.Candidate {
		return fmt.Errorf("recv low sc vote, drop it, sc:%v", vote)
	}

	_, _, err := chain.addVoteAndCount(vote, chain.config.StampingThreshold)
	if err != nil {
		chain.Log().Trace("AddVoteAndCount failed", "vote", vote, "err", err)
		return err
	}

	return nil
}

func (chain *Chain) checkEnoughVotesAndAddToSCChain() (err error) {
	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

	maxEnoughVotesHeight := uint64(0)

	var enoughHeights []uint64
	now := time.Now().Unix()
	for height, votes := range chain.buildingStampingVoteWindow {
		chain.Log().Trace("check enough", "height", height, "now", now, "votes", votes)
		if votes.weight >= chain.config.StampingThreshold && (now-votes.ts >= int64(stampingVoteCandidateTerm)) {
			if maxEnoughVotesHeight < height {
				maxEnoughVotesHeight = height
			}

			if height <= maxEnoughVotesHeight {
				enoughHeights = append(enoughHeights, height)
			}
		}
	}

	if maxEnoughVotesHeight != 0 {
		sort.Slice(enoughHeights, func(i, j int) bool {
			return enoughHeights[i] < enoughHeights[j]
		})

		for _, height := range enoughHeights {
			//
			var scVotes []*StampingVote
			votes := chain.buildingStampingVoteWindow[height]
			for _, vote := range votes.votes {
				scVotes = append(scVotes, vote)
			}
			delete(chain.buildingStampingVoteWindow, height)

			proofHeader := chain.header(height - chain.config.B)
			sc := NewStampingCertificate(height, proofHeader, scVotes)
			if sc == nil {
				return fmt.Errorf("new sc(%d) failed\n", height)
			}
			if err := chain.addStampingCertificate(sc); err != nil {
				return fmt.Errorf("add sc(%d) failed, err:%s\n", height, err)
			}
			chain.Log().Trace("add sc done", "height", height)
		}

		for height := range chain.buildingStampingVoteWindow {
			if height <= maxEnoughVotesHeight {
				delete(chain.buildingStampingVoteWindow, height)
			}
		}

		statusMsg := StatusMsg{
			SCStatus: chain.scStatus,
			Height:   chain.currentHeight,
		}
		if err := chain.broadcastMessage(message{
			code: StampingStatusMsg,
			data: &statusMsg,
			from: chain.name,
		}); err != nil {
			chain.Log().Error("broadcast status", "err", err)
		}
	}
	chain.Log().Trace("check enough done", "max height:", maxEnoughVotesHeight)
	return nil
}

func (chain *Chain) addVoteAndCount(vote *StampingVote, threshold uint64) (added, enough bool, err error) {
	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

	if vote.Height <= chain.scStatus.Candidate {
		return false, false, fmt.Errorf("vote.height too low, height:%d, C:%d\n",
			vote.Height, chain.scStatus.Candidate)
	}

	if vote.Height > chain.currentHeight {
		return false, false, fmt.Errorf("vote.height too high, height:%d, current:%d\n",
			vote.Height, chain.currentHeight)
	}

	votes, ok := chain.buildingStampingVoteWindow[vote.Height]
	if !ok {
		votes = NewStampingVotes()
		chain.buildingStampingVoteWindow[vote.Height] = votes
	}
	if chain.maxVoteHeight < vote.Height {
		chain.maxVoteHeight = vote.Height
	}

	if votes.hasVote(vote.Address) {
		return false, false, errors.New("duplicate vote")
	}

	if votes.weight < threshold {
		votes.addVote(vote)
		votes.weight += vote.Weight

		added = true
	}

	if votes.weight >= threshold {
		enough = true
	}

	if added && enough {
		votes.setEnoughTs()
	}

	chain.Log().Info("AddVoteAndCount OK", "miner", chain.config.Address.String(), "Added", added, "Enough", enough,
		"Weight", fmt.Sprintf("(%d/%d)", votes.weight, threshold), "vote", vote)
	return
}

func (chain *Chain) pickFrozenSCVoteToPeer(begin, end uint64, p *peer) (sent bool) {
	var startLog, endLog uint64
	for height := begin; height <= end; height++ {
		sc := chain.StampingCertificate(height)
		if sc == nil {
			chain.Log().Error("sc not exist", "height", height)
			continue
		}

		if err := p.PickAndSend(sc.Votes); err == nil {
			sent = true

			p.Log().Info("gossipVoteData vote below C", "chain", chain.name, "status", chain.StatusString(),
				"send", height, "not send", fmt.Sprintf("%d-%d", startLog, endLog))
			break
		} else {
			if startLog == 0 {
				startLog = height
			}
			endLog = height
			//p.Log().Info("gossipVoteData vote below C, err,", "chain", chain.name, "status", chain.StatusString(), "send", height, "err", err)
		}
	}
	return
}

func (chain *Chain) pickBuildingSCVoteToPeer(begin, end uint64, p *peer) (sent bool) {
	var startLog, endLog uint64
	for height := begin + 1; height <= end; height++ {
		votes := chain.buildingStampingVoteWindow[height]
		if err := p.PickBuildingAndSend(votes); err == nil {
			sent = true

			p.Log().Info("gossipVoteData vote between C and H", "chain", chain.name, "status", chain.StatusString(),
				"send", height, "not send", fmt.Sprintf("%d-%d", startLog, endLog))
			break
		} else {
			if startLog == 0 {
				startLog = height
			}
			endLog = height
			//p.Log().Info("gossipVoteData vote between C and H, err,", "chain", chain.name, "status", chain.StatusString(), "send", height, "err", err)
		}
	}

	return
}

func (chain *Chain) gossipVote(p *peer) {
	needSleep := false
	for {
		if needSleep {
			time.Sleep(500 * time.Millisecond)
		}
		needSleep = false

		scStatus := chain.ChainStatus()
		peerScStatus := p.ChainStatus()

		if scStatus.Height < peerScStatus.Candidate || scStatus.Candidate > peerScStatus.Height {
			needSleep = true
			continue
		}

		if peerScStatus.Candidate < scStatus.Candidate {
			if chain.pickFrozenSCVoteToPeer(peerScStatus.Candidate, scStatus.Candidate, p) {
				needSleep = true
				continue
			}
		}

		windowFloor := MaxUint64(scStatus.Candidate, peerScStatus.Candidate)
		windowCeil := MinUint64(scStatus.Height, peerScStatus.Height)
		if chain.pickBuildingSCVoteToPeer(windowFloor, windowCeil, p) {
			needSleep = true
			continue
		}

		needSleep = true
	}
}

func (chain *Chain) Sync() error {
	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

	peer := chain.getPeer()
	return chain.sync(peer)
}

func (chain *Chain) sync(peer *Chain) error {
	if !chain.canSynchronize(peer) {
		return fmt.Errorf("cannot synchronize from this chain")
	}

	// make BaseHeader exist as the genesis block header for stamping certificate
	if !chain.hasHeader(chain.config.BaseHeight) {
		if common.EmptyHash(chain.config.BaseHash) {
			// sync [1, Base] to get BaseHeader
			err := chain.forwardSyncRangeByHeaderAndFinalCertificate(peer, 1, chain.config.BaseHeight)
			if err == ErrRandomTrouble {
				return ErrRandomTrouble
			}
			if err != nil {
				return fmt.Errorf("forward synchronize [1, Base] failed: %v", err)
			}
		} else {
			// TODO: 也许应该将BaseHeader和BaseHash一起写到代码里面来，就不用下载了
			base := peer.Header(chain.config.BaseHeight)
			if err := chain.addHeaderWithHash(base, chain.config.BaseHash); err != nil {
				return err
			}
		}
	}

	// sync the first b range [Base+1, Base+B] if needed
	baseHeight := chain.config.BaseHeight
	if start, end := baseHeight+1, baseHeight+chain.config.B; chain.currentHeight < end && chain.currentHeight < peer.currentHeight {
		err := chain.forwardSyncRangeByHeaderAndFinalCertificate(peer, start, end)
		if err == ErrRandomTrouble {
			return ErrRandomTrouble
		}
		if err != nil {
			return fmt.Errorf("forward synchronize the first b blocks failed: %v", err)
		}
	}

	// C+1 - peer.currentHeight
	for begin, end := chain.scStatus.Candidate+1, chain.scStatus.Candidate+chain.config.B; chain.currentHeight < peer.currentHeight; {
		if chain.troubleMaker != nil && chain.troubleMaker.Trouble() {
			fmt.Printf("peer changed, [%d, %d]\n", begin, end)
			return ErrRandomTrouble
		}

		//fmt.Printf("process begin:[%d, %d]\n", begin, end)
		nextBegin, nextEnd, err := chain.syncNextBreadcrumb(peer, begin, end)
		if err != nil {
			return fmt.Errorf("synchronize breadcrumb in range[%d,%d] failed: %v", begin, end, err)
		}
		//fmt.Printf("process end:[%d, %d]\n", nextBegin, nextEnd)
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

	start := chain.config.BaseHeight + 1
	end := chain.config.BaseHeight + chain.config.B
	err := chain.forwardSyncRangeByHeaderAndFinalCertificate(peer, start, end)
	if err != nil {
		return fmt.Errorf("synchronize the first b blocks failed: %v", err)
	}

	proofHeight := end
	for height := proofHeight + chain.config.B; peer.scStatus.Fz > end; {
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
	if peer.scStatus.Proof > chain.config.HeightB() {
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
	if peer.scStatus.Candidate > chain.config.HeightB() {
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
		// TODO: compare votes?
		return a.Height == b.Height && a.Seed == b.Seed && a.Root == b.Root
	}

	return false
}
