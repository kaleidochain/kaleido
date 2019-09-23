package leap

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/kaleidochain/kaleido/p2p"

	"github.com/kaleidochain/kaleido/core/state"
	"github.com/kaleidochain/kaleido/ethdb"

	"github.com/kaleidochain/kaleido/consensus"
	"github.com/kaleidochain/kaleido/core/types"

	"github.com/ethereum/go-ethereum/log"
	"github.com/kaleidochain/kaleido/common"
	algorand "github.com/kaleidochain/kaleido/consensus/algorand/core"
	"github.com/kaleidochain/kaleido/core"
	"github.com/kaleidochain/kaleido/params"
)

const (
	chainStampingChanSize     = 10
	checkNewSCInterval        = 30 * time.Second
	stampingVoteCandidateTerm = 30
	gossipMaxHeightDiff       = 20
	belowCHeight              = 100
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

type MapStampingVotes map[uint64]*StampingVotes

type StampingChain struct {
	config *params.ChainConfig
	eth    Backend

	mutexChain sync.RWMutex
	fcChain    map[uint64]*FinalCertificate

	stampingStatus types.StampingStatus

	messageChan                chan message
	buildingStampingVoteWindow MapStampingVotes
	belowStampingVoteWindow    MapStampingVotes
	checkNewInterval           uint64
	checkNewTicker             *time.Ticker
	counter                    *HeightVoteSet

	pm *ProtocolManager
}

func NewChain(eth Backend, config *params.ChainConfig, engine consensus.Engine, networkId uint64) *StampingChain {
	chain := &StampingChain{
		config: config,
		eth:    eth,

		fcChain: make(map[uint64]*FinalCertificate),
	}
	chain.messageChan = make(chan message, msgChanSize)
	chain.buildingStampingVoteWindow = make(MapStampingVotes)
	chain.belowStampingVoteWindow = make(MapStampingVotes)
	chain.checkNewTicker = time.NewTicker(checkNewSCInterval)
	chain.counter = NewHeightVoteSet()
	chain.stampingStatus = chain.readStampingStatus()

	chain.pm = NewProtocolManager(eth, chain, config, engine, networkId)

	chain.Start()

	return chain
}

func (chain *StampingChain) Start() {
	go chain.handleLoop()
}

func (chain *StampingChain) readStampingStatus() types.StampingStatus {
	status := chain.eth.BlockChain().GetStampingStatus()
	if status != nil {
		return *status
	}

	if chain.eth.BlockChain().CurrentBlock().NumberU64() > chain.config.Stamping.HeightB() {
		log.Error("cant read stamping status")
	}

	return types.StampingStatus{
		Height:    chain.eth.BlockChain().CurrentBlock().NumberU64(),
		Candidate: chain.config.Stamping.HeightB(),
		Proof:     chain.config.Stamping.HeightB(),
		Fz:        chain.config.Stamping.HeightB(),
	}
}

func (chain *StampingChain) FinalCertificate(height uint64) *FinalCertificate {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	return chain.fcChain[height]
}

func (chain *StampingChain) finalCertificate(height uint64) *FinalCertificate {
	return chain.fcChain[height]
}

func (chain *StampingChain) HeaderAndStampingCertificate(height uint64) (*types.Header, *types.StampingCertificate) {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	head := chain.header(height)
	sc := chain.stampingCertificate(height)

	return head, sc
}

func (chain *StampingChain) StampingCertificate(height uint64) *types.StampingCertificate {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	return chain.stampingCertificate(height)
}

func (chain *StampingChain) stampingCertificate(height uint64) *types.StampingCertificate {
	return chain.eth.BlockChain().GetStampingCertificate(height)
}

func (chain *StampingChain) Header(height uint64) *types.Header {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	return chain.header(height)
}

func (chain *StampingChain) header(height uint64) *types.Header {
	return chain.eth.BlockChain().GetHeaderByNumber(height)
}

func (chain *StampingChain) hasHeader(height uint64) bool {
	return chain.header(height) != nil
}

func (chain *StampingChain) AddHeader(header *types.Header) error {
	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

	return chain.addHeader(header)
}

func (chain *StampingChain) addHeaderWithHash(header *types.Header, hash common.Hash) error {
	if header.Hash() != hash {
		return fmt.Errorf("invalid header(%d): hash not matched, expect %s but got %s",
			header.NumberU64(), hash, header.Hash())
	}
	if chain.header(header.NumberU64()) != nil {
		return fmt.Errorf("header(%d) already exists", header.NumberU64())
	}

	// TODO: header validation needs to be refactored
	n, err := chain.eth.BlockChain().InsertHeaderChain([]*types.Header{header}, 1)
	if n != 1 {
		return fmt.Errorf("header insert error, n = 0")
	}
	return err
}

func (chain *StampingChain) addHeader(header *types.Header) error {
	if chain.header(header.NumberU64()) != nil {
		return fmt.Errorf("header(%d) already exists", header.NumberU64())
	}
	nextHeader := chain.header(header.NumberU64() + 1)
	if nextHeader == nil {
		return fmt.Errorf("next header(%d) not exists", header.NumberU64()+1)
	}
	if nextHeader.ParentHash != header.Hash() {
		return fmt.Errorf("next header(%d) ParentHash != header(%d).Hash", header.NumberU64()+1, header.NumberU64())
	}
	// TODO: header validation needs to be refactored
	n, err := chain.eth.BlockChain().InsertHeaderChain([]*types.Header{header}, 1)
	if n != 1 {
		return fmt.Errorf("header insert error, n = 0")
	}

	return err
}

func (chain *StampingChain) AddBlock(header *types.Header, fc *FinalCertificate) error {
	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

	return chain.addBlock(header, fc)
}

func (chain *StampingChain) addBlock(header *types.Header, fc *FinalCertificate) error {
	if header.NumberU64() <= chain.stampingStatus.Height {
		return fmt.Errorf("block(%d) lower than stamping status(%s)", header.NumberU64(), chain.StatusString())
	}
	if h := chain.header(header.NumberU64()); h != nil {
		return fmt.Errorf("block(%d) exists", header.NumberU64())
	}
	if _, ok := chain.fcChain[fc.Height]; ok {
		return fmt.Errorf("finalCertificate(%d) exists", fc.Height)
	}

	parent := chain.header(header.NumberU64() - 1)
	if parent == nil {
		return fmt.Errorf("parent block(%d) not exists", header.NumberU64()-1)
	}

	if !fc.Verify(header, parent) {
		return fmt.Errorf("block invalid")
	}

	if err := chain.addHeader(header); err != nil {
		return err
	}

	chain.fcChain[fc.Height] = fc
	chain.updateStatusHeight(header.NumberU64())
	chain.writeStatusToDb()
	return nil
}

func (chain *StampingChain) addStampingCertificateWithHeader(header *types.Header, sc *types.StampingCertificate) error {
	if sc.Height <= chain.stampingStatus.Candidate {
		return fmt.Errorf("sc(%d) lower than stamping status(%s)", sc.Height, chain.StatusString())
	}

	if chain.header(sc.Height) != nil {
		return fmt.Errorf("scheader(%d) exists", sc.Height)
	}

	if err := chain.verifyStampingCertificate(header, sc); err != nil {
		return err
	}

	if err := chain.eth.BlockChain().WriteStampingCertificate(sc); err != nil {
		return err
	}
	chain.updateStatusHeight(header.NumberU64())
	chain.updateStampingCertificate(sc.Height)
	chain.writeStatusToDb()
	return nil
}

func (chain *StampingChain) updateStatusHeight(height uint64) {
	chain.stampingStatus.Height = height
}

func (chain *StampingChain) writeStatusToDb() {
	chain.eth.BlockChain().WriteStampingCertificateStatus(&chain.stampingStatus)
}

func (chain *StampingChain) verifyStampingCertificate(header *types.Header, sc *types.StampingCertificate) error {
	if chain.stampingCertificate(sc.Height) != nil {
		return fmt.Errorf("stampingCertificate(%d) exists", sc.Height)
	}

	proofHeader := chain.header(sc.Height - chain.config.Stamping.B)
	if proofHeader == nil {
		return fmt.Errorf("proof header(%d) not exists", sc.Height-chain.config.Stamping.B)
	}

	if err := sc.Verify(chain.config, header, proofHeader); err != nil {
		return err
	}

	return nil
}

func (chain *StampingChain) addStampingCertificate(sc *types.StampingCertificate) error {
	header := chain.header(sc.Height)
	if header == nil {
		return fmt.Errorf("header(%d) not exists", sc.Height)
	}

	if err := chain.verifyStampingCertificate(header, sc); err != nil {
		return err
	}

	if err := chain.eth.BlockChain().WriteStampingCertificate(sc); err != nil {
		return err
	}

	chain.updateStampingCertificate(sc.Height)
	return nil
}

func (chain *StampingChain) updateStampingCertificate(height uint64) {
	if height <= chain.stampingStatus.Candidate {
		return
	}

	// delete fc
	// max(N-B, C+1, B+1)
	start := MaxUint64(height-chain.config.Stamping.B+1, chain.stampingStatus.Candidate+1)
	chain.deleteFC(start, height)

	if height-chain.stampingStatus.Proof <= chain.config.Stamping.B {
		chain.stampingStatus.Candidate = height
	} else {
		chain.freezeProof()
		chain.stampingStatus.Proof = chain.stampingStatus.Candidate
		chain.stampingStatus.Candidate = height
	}

	// trim( max(QB - B, Fz), min((C-B), QB)) // 开区间
	start = MaxUint64(chain.stampingStatus.Proof-chain.config.Stamping.B, chain.stampingStatus.Fz)
	end := MinUint64(chain.stampingStatus.Candidate-chain.config.Stamping.B, chain.stampingStatus.Proof)
	n := chain.trim(start, end)
	_ = n
	//fmt.Printf("trim range=[%d, %d] trimmed=%d/%d\n", start, end, n, end-start-1)

	return
}

//Keeping proof-objects up-to-date
func (chain *StampingChain) AddStampingCertificate(sc *types.StampingCertificate) error {
	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

	return chain.addStampingCertificate(sc)
}

func (chain *StampingChain) deleteFC(start, end uint64) int {
	count := 0
	for i := start; i <= end; i++ {
		if _, ok := chain.fcChain[i]; ok {
			delete(chain.fcChain, i)
			count += 1
		}
	}

	return count
}

func (chain *StampingChain) freezeProof() {
	start := MaxUint64(chain.stampingStatus.Fz+1, chain.stampingStatus.Proof-chain.config.Stamping.B+1)
	headerEnd := MinUint64(chain.stampingStatus.Candidate-chain.config.Stamping.B, chain.stampingStatus.Proof)
	end := chain.stampingStatus.Proof

	//trim the tail of P to keep its length minimal
	for height := start; height < headerEnd; height++ {
		//delete(chain.scChain, height)
		chain.eth.BlockChain().DeleteStampingCertificate(height)
		// TODO: delete headerchain
		//delete(chain.headerChain, height)
	}

	// delete sc from the minimal tail
	for height := headerEnd; height < end; height++ {
		//delete(chain.scChain, height)
		chain.eth.BlockChain().DeleteStampingCertificate(height)
	}

	chain.stampingStatus.Fz = chain.stampingStatus.Proof
}

func (chain *StampingChain) trim(start, end uint64) int {
	count := 0
	for height := end - 1; height > start; height-- {
		if fc := chain.fcChain[height]; fc != nil {
			panic(fmt.Sprintf("fc(%d) should already be deleted", height))
		}

		if chain.header(height) == nil && chain.stampingCertificate(height) == nil {
			break
		}

		//delete(chain.scChain, height)
		chain.eth.BlockChain().DeleteStampingCertificate(height)
		// TODO: delete headerchain
		//delete(chain.headerChain, height)
		count += 1
	}

	return count
}

func (chain *StampingChain) print() {
	if chain.stampingStatus.Height <= chain.config.Stamping.B {
		return
	}
	count := chain.printRange(1, chain.stampingStatus.Height+1)

	fmt.Printf("Status: Fz=%d, Proof=%d, Candidate=%d\n", chain.stampingStatus.Fz, chain.stampingStatus.Proof, chain.stampingStatus.Candidate)
	fmt.Printf("MaxHeight=%d, realLength=%d, percent=%.2f%%\n", chain.stampingStatus.Height, count, float64(count*10000/chain.stampingStatus.Height)/100)
}

func (chain *StampingChain) Print() {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	chain.print()
}

func (chain *StampingChain) PrintFrozenBreadcrumbs() {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	begin := chain.config.Stamping.HeightB() + 1
	end := chain.stampingStatus.Fz + 1
	count := chain.printRange(begin, end)

	fmt.Printf("Status: Fz=%d, Proof=%d, Candidate=%d\n", chain.stampingStatus.Fz, chain.stampingStatus.Proof, chain.stampingStatus.Candidate)
	fmt.Printf("RangeLength=%d, realLength=%d, percent=%.2f%%\n", end-begin, count, float64(count*10000/(end-begin))/100)
}

func (chain *StampingChain) PrintProperty() {
	countBreadcrumb := 0
	countTail := 0
	sumTailLength := 0
	countForward := 0
	sumForwardLength := 0

	for begin, end := chain.config.Stamping.HeightB()+1, chain.config.Stamping.HeightB()+chain.config.Stamping.B; end <= chain.stampingStatus.Fz; {
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

			begin = breadcrumb.stampingHeader.NumberU64() + 1
			end = (breadcrumb.stampingHeader.NumberU64() - uint64(len(breadcrumb.tail))) + chain.config.Stamping.B
		} else {
			countForward++
			sumForwardLength += len(breadcrumb.forwardHeader)

			lastOne := breadcrumb.forwardHeader[len(breadcrumb.forwardHeader)-1]
			begin = lastOne.NumberU64() + 1
			end = lastOne.NumberU64() + chain.config.Stamping.B
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

func (chain *StampingChain) printRange(begin, end uint64) uint64 {
	const perLine = 8

	lastPrinted := uint64(0)
	count := uint64(0)
	for height := begin; height < end; height++ {
		header := chain.header(height)
		fc := chain.fcChain[height]
		sc := chain.stampingCertificate(height)

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

func (chain *StampingChain) formatHeader(height uint64, fc *FinalCertificate, sc *types.StampingCertificate, hasParent bool) string {
	fcTag := ""
	if fc != nil {
		fcTag = "F"

		if chain.header(height-1) == nil {
			fcTag = "f"
		}
	}

	scTag := ""
	if sc != nil {
		scTag = "S"

		if chain.header(height-chain.config.Stamping.B) == nil {
			scTag = "s"
		}
	}

	arrow := ""
	if hasParent {
		arrow = "<-"
	}

	zpcTag := ""
	switch height {
	case chain.stampingStatus.Fz:
		zpcTag = "Z"
	case chain.stampingStatus.Proof:
		zpcTag = "P"
	case chain.stampingStatus.Candidate:
		zpcTag = "C"
	}

	return fmt.Sprintf("%2s[%4d(%1s%1s%1s)]", arrow, height, zpcTag, fcTag, scTag)
}

func (chain *StampingChain) HeaderAndFinalCertificate(height uint64) (*types.Header, *FinalCertificate) {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	return chain.header(height), chain.finalCertificate(height)
}

func (chain *StampingChain) StatusString() string {
	return fmt.Sprintf("%d/%d/%d/%d", chain.stampingStatus.Fz, chain.stampingStatus.Proof, chain.stampingStatus.Candidate, chain.stampingStatus.Height)
}

func (chain *StampingChain) ChainStatus() types.StampingStatus {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	return chain.stampingStatus
}

func (chain *StampingChain) forwardSyncRangeByHeaderAndFinalCertificate(p *peer, start, end uint64) error {
	for height := start; height <= end && height <= p.scStatus.Height; height++ {
		if chain.hasHeader(height) {
			continue
		}

		header, fc := p.HeaderAndFinalCertificate(height)
		if header == nil || fc == nil {
			return fmt.Errorf("p has no header or fc at height(%d)", height)
		}

		if err := chain.addBlock(header, fc); err != nil {
			return err
		}
	}

	return nil
}

func (chain *StampingChain) backwardSyncRangeOnlyByHeader(p *peer, start, end uint64, endHash common.Hash) error {
	for height := end; height >= start; height-- {
		if chain.hasHeader(height) {
			continue
		}

		header := p.Header(height)
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

func (chain *StampingChain) getNextBreadcrumb(begin, end uint64) (*breadcrumb, error) {
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

	for height := begin; height <= chain.stampingStatus.Height; height++ {
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

		forwardHeight := height + chain.config.Stamping.B
		if sc := chain.stampingCertificate(forwardHeight); sc != nil {
			break
		}
	}

	return bc, nil
}

func (chain *StampingChain) getAllHeaders(begin, end uint64) (headers []*types.Header) {
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
	stampingHeader      *types.Header
	stampingCertificate *types.StampingCertificate
	tail                []*types.Header

	forwardHeader           []*types.Header
	forwardFinalCertificate []*FinalCertificate
}

func (chain *StampingChain) syncNextBreadcrumb(p *peer, begin, end uint64) (nextBegin, nextEnd uint64, err error) {
	var bc *breadcrumb
	bc, err = p.GetNextBreadcrumb(begin, end)
	if err != nil {
		return
	}

	if bc.stampingHeader != nil {
		if proofHeader := chain.header(bc.stampingHeader.Number.Uint64() - chain.config.Stamping.B); proofHeader == nil {
			tailLength := chain.getTailLength(begin)
			neededBegin := bc.stampingHeader.Number.Uint64() - chain.config.Stamping.B
			neededEnd := begin - 1 - tailLength
			if err = chain.syncAllHeaders(p, neededBegin, neededEnd); err != nil {
				// TODO: switch to full node
				archive := chain.pm.GetArchivePeer()
				if err = chain.syncAllHeaders(archive, neededBegin, neededEnd); err != nil {
					/*fmt.Printf("syncAllHeaders, peer:%s, begin:end=[%d, %d] [%d, %d], proof:%d, tailLength:%d\n",
					peer.id, begin, end, neededBegin, neededEnd, bc.stampingHeader.Height-chain.config.Stamping.B, tailLength)*/
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

		nextBegin = chain.stampingStatus.Height + 1
		nextEnd = (chain.stampingStatus.Height - uint64(len(bc.tail))) + chain.config.Stamping.B
	} else {
		for i, h := range bc.forwardHeader {
			fc := bc.forwardFinalCertificate[i]
			err = chain.addBlock(h, fc)
			if err != nil {
				return
			}
		}

		nextBegin = chain.stampingStatus.Height + 1
		nextEnd = chain.stampingStatus.Height + chain.config.Stamping.B
	}

	return
}

func (chain *StampingChain) getTailLength(height uint64) (length uint64) {
	for h := height - 1; h > height-chain.config.Stamping.B; h-- {
		if chain.header(h) == nil {
			return
		}
		length += 1
	}
	return
}

func (chain *StampingChain) syncAllHeaders(p *peer, begin, end uint64) (err error) {
	headers := p.GetHeaders(begin, end)
	if len(headers) == 0 || headers[len(headers)-1].NumberU64() != begin {
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

func (chain *StampingChain) sendToMessageChan(msg message) {
	select {
	case chain.messageChan <- msg:
		return
	default:
		log.Error("message chan full", "size", len(chain.messageChan))
		return
	}
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

func (chain *StampingChain) OnReceive(code uint64, data interface{}, from string) {
	sendToMessageChan(chain.messageChan, message{code, data, from})
}

func (chain *StampingChain) handleLoop() {
	chainStampingCh := make(chan core.ChainStampingEvent, chainStampingChanSize)
	chainStampingSub := chain.eth.BlockChain().SubscribeStampingEvent(chainStampingCh)
	defer chainStampingSub.Unsubscribe()

	for {
		select {
		case msg := <-chain.messageChan:
			chain.handleMsg(msg)
		case newStamping := <-chainStampingCh:
			chain.handleStampingEvent(newStamping)

		case <-chain.checkNewTicker.C:
			if err := chain.checkEnoughVotesAndAddToSCChain(); err != nil {
				log.Error("handle check new stampingcertificate failed", "err", err)
			}

			// for statistic
			chain.checkBelowCEnoughVotesAndCount()
		case err := <-chainStampingSub.Err():
			log.Error("handleLoop chainStampingSub error", "err", err)
			return
		}
	}
}

func (chain *StampingChain) handleStampingEvent(stampingEvent core.ChainStampingEvent) {
	chain.handleUpdateStatus()

	if err := chain.handleStampingVote(stampingEvent.Vote); err != nil {
		log.Error("handleStampingVote error", "err", err)
		return
	}

	log.Trace("handleStampingEvent done", "Status", chain.StatusString(), "vote", stampingEvent.Vote.String())

	//chain.pm.Broadcast(StampingVoteMsg, stampingEvent.Vote)
}

func (chain *StampingChain) handleMsg(msg message) {
	switch msg.code {
	case StampingVoteMsg:
		vote := msg.data.(*types.StampingVote)
		if err := chain.handleStampingVote(vote); err != nil {
			log.Error("handle vote failed", "vote", vote, "from", msg.from, "err", err)
		}
	}
}

func (chain *StampingChain) handleUpdateStatus() {
	log.Trace("handleUpdateStatus", "Status", chain.StatusString())

	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

	currentBlock := chain.eth.BlockChain().CurrentBlock()
	chain.stampingStatus.Height = currentBlock.NumberU64()

	chain.pm.Broadcast(StampingStatusMsg, &chain.stampingStatus)
}

func (chain *StampingChain) handleStampingVote(vote *types.StampingVote) error {
	log.Trace("handleStampingVote", "vote", vote)
	if vote == nil {
		return fmt.Errorf("vote is nil")
	}

	// verify
	proofHeight := vote.Height - chain.config.Stamping.B
	proofHeader := chain.Header(proofHeight)
	if proofHeader == nil {
		return fmt.Errorf("proof header(%d) is not exist", proofHeight)
	}

	err := core.VerifyProof(chain.config.Algorand, proofHeader.Root, vote.Height, []common.Address{vote.Address}, vote.TrieProof)
	if err != nil {
		return err
	}

	db := ethdb.NewMemDatabase()
	vote.TrieProof.Store(db)
	database := state.NewDatabase(db)
	stateDb, err := state.New(proofHeader.Root, database)
	if err != nil {
		return err
	}

	mv := algorand.GetMinerVerifier(chain.config.Algorand, stateDb, vote.Address, vote.Height)
	err = algorand.VerifyStampingSignatureAndCredential(mv, vote.SignBytes(), vote.ESignValue, &vote.Credential, stateDb, proofHeader.Seed(), proofHeader.TotalBalanceOfMiners)
	if err != nil {
		return err
	}

	_, _, err = chain.addStampingVoteAndCount(vote, params.CommitteeConfigv1.StampingCommitteeThreshold)
	if err != nil {
		log.Trace("AddVoteAndCount failed", "vote", vote, "err", err)
		return err
	}

	chain.pm.Broadcast(HasSCVoteMsg, ToHasSCVoteData(vote))

	return nil
}

func (chain *StampingChain) checkEnoughVotesAndAddToSCChain() (err error) {
	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

	maxEnoughVotesHeight, enoughHeights := findEnoughHeights(chain.buildingStampingVoteWindow, params.CommitteeConfigv1.StampingCommitteeThreshold)

	if maxEnoughVotesHeight != 0 {
		sort.Slice(enoughHeights, func(i, j int) bool {
			return enoughHeights[i] < enoughHeights[j]
		})

		for _, height := range enoughHeights {
			//
			var scVotes []*types.StampingVote
			votes := chain.buildingStampingVoteWindow[height]
			for _, vote := range votes.votes {
				scVotes = append(scVotes, vote)
			}
			delete(chain.buildingStampingVoteWindow, height)

			header := chain.header(height)
			if header == nil {
				panic(fmt.Sprintf("header not exist:%d", height))
			}
			sc := types.NewStampingCertificate(header, scVotes)
			if sc == nil {
				return fmt.Errorf("new sc(%d) failed\n", height)
			}
			if err := chain.addStampingCertificate(sc); err != nil {
				return fmt.Errorf("add sc(%d) failed, err:%s\n", height, err)
			}
			log.Trace("add sc done", "height", height)
		}

		for height := range chain.buildingStampingVoteWindow {
			if height <= maxEnoughVotesHeight {
				delete(chain.buildingStampingVoteWindow, height)
			}
		}

		chain.pm.Broadcast(StampingStatusMsg, &chain.stampingStatus)
	}
	log.Trace("check enough done", "max height", maxEnoughVotesHeight)

	chain.print()
	return nil
}

func findEnoughHeights(window MapStampingVotes, threshold uint64) (uint64, []uint64) {
	maxEnoughVotesHeight := uint64(0)

	var enoughHeights []uint64
	now := time.Now().Unix()
	for height, votes := range window {
		log.Trace("check vote enough", "height", height, "vote", votes)
		if votes.weight >= threshold && (now-votes.ts >= int64(stampingVoteCandidateTerm)) {
			if maxEnoughVotesHeight < height {
				maxEnoughVotesHeight = height
			}

		}
	}
	for height, votes := range window {
		if votes.weight >= threshold {
			if height <= maxEnoughVotesHeight {
				enoughHeights = append(enoughHeights, height)
			}
		}
	}

	return maxEnoughVotesHeight, enoughHeights
}

func (chain *StampingChain) checkBelowCEnoughVotesAndCount() {
	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

	maxEnoughVotesHeight, enoughHeights := findEnoughHeights(chain.belowStampingVoteWindow, params.CommitteeConfigv1.StampingCommitteeThreshold)

	for height := range chain.belowStampingVoteWindow {
		if height <= maxEnoughVotesHeight || ((height > belowCHeight) && (height <= chain.stampingStatus.Candidate-100)) {
			delete(chain.buildingStampingVoteWindow, height)
		}
	}

	log.Trace("check below C enough done", "max height", maxEnoughVotesHeight, "enough", len(enoughHeights))
}

func (chain *StampingChain) addStampingVoteAndCount(vote *types.StampingVote, threshold uint64) (added, enough bool, err error) {
	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

	if vote.Height <= chain.stampingStatus.Candidate {
		chain.processStampingVoteWindow(vote, chain.belowStampingVoteWindow, threshold)
		return false, false, fmt.Errorf("vote.height too low, height:%d, C:%d\n",
			vote.Height, chain.stampingStatus.Candidate)
	}

	if vote.Height > chain.stampingStatus.Height {
		return false, false, fmt.Errorf("vote.height too high, height:%d, current:%d\n",
			vote.Height, chain.stampingStatus.Height)
	}

	added, enough, err = chain.processStampingVoteWindow(vote, chain.buildingStampingVoteWindow, threshold)

	log.Info("addStampingVoteAndCount OK", "Added", added, "Enough", enough,
		"Weight", fmt.Sprintf("(%d/%d)", chain.buildingStampingVoteWindow[vote.Height].weight, threshold), "vote", vote)
	return
}

func (chain *StampingChain) processStampingVoteWindow(vote *types.StampingVote, window MapStampingVotes, threshold uint64) (added, enough bool, err error) {
	votes, ok := window[vote.Height]
	if !ok {
		votes = NewStampingVotes()
		window[vote.Height] = votes
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

	return
}

func (chain *StampingChain) pickFrozenSCVoteToPeer(begin, end uint64, p *peer) (sent bool) {
	var startLog, endLog uint64
	for height := begin; height <= end; height++ {
		sc := chain.StampingCertificate(height)
		if sc == nil {
			log.Error("sc not exist", "height", height)
			continue
		}

		if err := p.PickAndSend(sc.Votes); err == nil {
			sent = true

			p.Log().Info("gossipVoteData vote below F", "status", chain.StatusString(),
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

func (chain *StampingChain) PickBuildingSCVoteToPeer(begin, end uint64, p *peer) (sent bool) {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	var startLog, endLog uint64
	for height := begin + 1; height <= end; height++ {
		votes := chain.buildingStampingVoteWindow[height]
		if err := p.PickBuildingAndSend(votes); err == nil {
			sent = true

			p.Log().Info("gossipVoteData vote between C and H", "status", chain.StatusString(),
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

func (chain *StampingChain) Sync() error {
	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

	peer := chain.pm.GetBestPeer()
	return chain.sync(peer)
}

func (chain *StampingChain) sync(peer *peer) error {
	// make BaseHeader exist as the genesis block header for stamping certificate
	if !chain.hasHeader(chain.config.Stamping.BaseHeight) {
		if common.EmptyHash(chain.config.Stamping.BaseHash) {
			// sync [1, Base] to get BaseHeader
			err := chain.forwardSyncRangeByHeaderAndFinalCertificate(peer, 1, chain.config.Stamping.BaseHeight)
			if err != nil {
				return fmt.Errorf("forward synchronize [1, Base] failed: %v", err)
			}
		} else {
			// TODO: 也许应该将BaseHeader和BaseHash一起写到代码里面来，就不用下载了
			base := peer.Header(chain.config.Stamping.BaseHeight)
			if err := chain.addHeaderWithHash(base, chain.config.Stamping.BaseHash); err != nil {
				return err
			}
		}
	}

	// sync the first b range [Base+1, Base+B] if needed
	baseHeight := chain.config.Stamping.BaseHeight
	if start, end := baseHeight+1, baseHeight+chain.config.Stamping.B; chain.stampingStatus.Height < end && chain.stampingStatus.Height < peer.scStatus.Height {
		err := chain.forwardSyncRangeByHeaderAndFinalCertificate(peer, start, end)
		if err != nil {
			return fmt.Errorf("forward synchronize the first b blocks failed: %v", err)
		}
	}

	// C+1 - peer.currentHeight
	for begin, end := chain.stampingStatus.Candidate+1, chain.stampingStatus.Candidate+chain.config.Stamping.B; chain.stampingStatus.Height < peer.scStatus.Height; {
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

func (chain *StampingChain) Protocols() []p2p.Protocol {
	return chain.pm.SubProtocols
}

func (chain *StampingChain) EqualRange(other *StampingChain, begin, end uint64) (bool, error) {
	for height := begin; height <= end; height++ {
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

func (chain *StampingChain) Equal(other *StampingChain) (bool, error) {
	if chain.stampingStatus != other.stampingStatus {
		return false, fmt.Errorf("status not equal, this:%v, other:%v", chain.stampingStatus, other.stampingStatus)
	}

	for height := uint64(1); height <= chain.stampingStatus.Fz; height++ {
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

	for height := chain.stampingStatus.Fz + 1; height < chain.stampingStatus.Height; height++ {
		if height == chain.stampingStatus.Proof || height == chain.stampingStatus.Candidate {
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
		height := chain.stampingStatus.Proof

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
		height := chain.stampingStatus.Candidate

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

func EqualHeader(a, b *types.Header) bool {
	if a == nil && b == nil {
		return true
	}

	if a != nil && b != nil {
		return a.Hash() == b.Hash()
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

func EqualStampingCertificate(a, b *types.StampingCertificate) bool {
	if a == nil && b == nil {
		return true
	}

	if a != nil && b != nil {
		// TODO: compare votes?
		return a.Height == b.Height && a.Hash == b.Hash
	}

	return false
}
