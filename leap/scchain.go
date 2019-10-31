package leap

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kaleidochain/kaleido/eth/downloader"

	"github.com/ethereum/go-ethereum/log"
	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/consensus"
	"github.com/kaleidochain/kaleido/consensus/algorand"
	algorandCore "github.com/kaleidochain/kaleido/consensus/algorand/core"
	"github.com/kaleidochain/kaleido/core"
	"github.com/kaleidochain/kaleido/core/types"
	"github.com/kaleidochain/kaleido/p2p"
	"github.com/kaleidochain/kaleido/params"
)

var (
	errLowerThanHeight = fmt.Errorf("block lower than stamping height")
	errExists          = fmt.Errorf("header exists")
)

const (
	chainStampingChanSize     = 10
	checkNewSCInterval        = 6 * time.Second
	stampingVoteCandidateTerm = 60 * 10
	gossipMaxHeightDiff       = 20
	belowCHeight              = 100

	forceSyncCycle      = 10 * time.Second // Time interval to force syncs, even if few peers are available
	minDesiredPeerCount = 5                // Amount of peers desired to start syncing
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
	config     *params.ChainConfig
	eth        Backend
	downloader *downloader.Downloader

	mutexChain sync.RWMutex

	stampingStatus             types.StampingStatus
	messageChan                chan message
	buildingStampingVoteWindow MapStampingVotes
	belowStampingVoteWindow    MapStampingVotes
	checkNewInterval           uint64
	checkNewTicker             *time.Ticker
	counter                    *HeightVoteSet

	pm            *ProtocolManager
	synchronising int32
}

func NewChain(eth Backend, config *params.ChainConfig, engine consensus.Engine, networkId uint64, downloader *downloader.Downloader) *StampingChain {
	chain := &StampingChain{
		config:     config,
		eth:        eth,
		downloader: downloader,
	}
	chain.messageChan = make(chan message, msgChanSize)
	chain.buildingStampingVoteWindow = make(MapStampingVotes)
	chain.belowStampingVoteWindow = make(MapStampingVotes)
	chain.checkNewTicker = time.NewTicker(checkNewSCInterval)
	chain.counter = NewHeightVoteSet()

	chain.pm = NewProtocolManager(eth, chain, config, engine, networkId)

	chain.Start()

	return chain
}

func (chain *StampingChain) Start() {
	chain.processStatusAndChainConsistence()
	chain.Sync()

	go chain.syncer()
	go chain.handleLoop()
}

func (chain *StampingChain) processStatusAndChainConsistence() {
	status := chain.eth.BlockChain().GetStampingStatus()

	if status == nil {
		if chain.eth.BlockChain().CurrentBlock().NumberU64() < chain.config.Stamping.BaseHeight {
			log.Error("cant read stamping status, check stamping chain status")
			panic(fmt.Sprintf("BaseHeight > CurrentHeight, BaseHeight:%d, CurrentHeight:%d\n",
				chain.config.Stamping.BaseHeight, chain.eth.BlockChain().CurrentBlock().NumberU64()))
		}
		chain.stampingStatus = types.StampingStatus{
			Height:    chain.eth.BlockChain().CurrentBlock().NumberU64(),
			Candidate: chain.config.Stamping.HeightB(),
			Proof:     chain.config.Stamping.HeightB(),
			Fz:        chain.config.Stamping.HeightB(),
		}
		return
	}
	chain.stampingStatus = *status
	log.Trace("read stamping", "stamping status", status.String())

	futureStampingStatus := chain.eth.BlockChain().GetFutureStampingStatus()
	if futureStampingStatus == nil {
		return
	}

	log.Trace("read stamping", "future stamping status", futureStampingStatus.String())

	if futureStampingStatus.Candidate > status.Candidate ||
		futureStampingStatus.Proof > status.Proof ||
		futureStampingStatus.Fz > status.Fz {
		chain.doKeepStampingStatusUptoDate(futureStampingStatus.Candidate)
	}
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
	has, err := chain.eth.BlockChain().HeaderHasBeenDeleted(height)
	if err != nil {
		log.Error("db error", "height", height, "err", err)
		return nil
	}
	if has {
		return nil
	}

	return chain.eth.BlockChain().GetHeaderByNumber(height)
}

func (chain *StampingChain) hasHeader(height uint64) bool {
	return chain.header(height) != nil
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
	return chain.writeHeader(header)
}

func (chain *StampingChain) addBackwardHeader(header *types.Header) error {
	if chain.header(header.NumberU64()) != nil {
		log.Warn("header exist", "height", header.NumberU64())
		//return fmt.Errorf("header(%d) already exists", header.NumberU64())
	}
	nextHeader := chain.header(header.NumberU64() + 1)
	if nextHeader == nil {
		return fmt.Errorf("next header(%d) not exists", header.NumberU64()+1)
	}
	if nextHeader.ParentHash != header.Hash() {
		return fmt.Errorf("next header(%d) ParentHash != header(%d).Hash", header.NumberU64()+1, header.NumberU64())
	}
	// TODO: header validation needs to be refactored
	return chain.writeBackwardHeader(header)
}

func (chain *StampingChain) addForwardHeader(header *types.Header) error {
	// TODO: header validation needs to be refactored
	return chain.writeHeader(header)
}

func (chain *StampingChain) writeHeader(header *types.Header) error {
	return chain.eth.BlockChain().InsertStampingCertificateHeader(header)
}

func (chain *StampingChain) writeBackwardHeader(header *types.Header) error {
	return chain.eth.BlockChain().InsertBackwardHeader([]*types.Header{header})
}

func (chain *StampingChain) writeNonCertificateHeader(header *types.Header) error {
	return chain.eth.BlockChain().WriteNonCertificateHeader(header)
}

func (chain *StampingChain) addForwardBlock(header *types.Header) error {
	height := header.NumberU64()
	if height <= chain.stampingStatus.Height {
		log.Warn("block lower than stamping status", "height", height, "status", chain.stampingStatus.String())
		return errLowerThanHeight
	}
	if h := chain.header(height); h != nil {
		log.Warn("block exist", "height", height)
		//return errExists
	}

	parent := chain.header(height - 1)
	if parent == nil {
		return fmt.Errorf("parent block(%d) not exists", height-1)
	}

	stateDb, err := algorand.GetStateDbFromProof(header.Certificate.TrieProof, parent.Root)
	if err != nil {
		log.Warn("statedb error", "height", height, "err", err)
		return err
	}
	if err := algorand.DoVerifySeal(chain.config, stateDb, header, parent); err != nil {
		return fmt.Errorf("DoVerifySeal failed, height:%d, err:%s", height, err)
	}

	if err := chain.addForwardHeader(header); err != nil {
		return err
	}

	chain.updateStatusHeight(height)
	chain.writeStatusToDb()
	chain.broadcastStampingStatusMsg()
	return nil
}

func (chain *StampingChain) addStampingCertificateWithSync(header *types.Header, sc *types.StampingCertificate) error {
	log.Trace("addStampingCertificateWithSync", "header.Height", header.NumberU64(), "sc.Height", sc.Height)
	if sc.Height <= chain.stampingStatus.Candidate {
		return fmt.Errorf("sc(%d) lower than stamping status(%s)", sc.Height, chain.stampingStatus.String())
	}

	if err := chain.verifyStampingCertificate(header, sc); err != nil {
		return err
	}

	if err := chain.writeHeader(header); err != nil {
		return err
	}

	if err := chain.writeStampingCertificate(sc); err != nil {
		return err
	}
	chain.updateStatusHeight(header.NumberU64())
	chain.keepStampingChainUptoDate(sc.Height)
	return nil
}

func (chain *StampingChain) updateStatusHeight(height uint64) {
	chain.stampingStatus.Height = height
}

func (chain *StampingChain) writeFutureStatusToDb(futureStatus *types.StampingStatus) {
	chain.eth.BlockChain().WriteFutureStampingCertificateStatus(futureStatus)
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

	e := chain.writeStampingCertificate(sc)
	if e != nil {
		return e
	}

	chain.keepStampingChainUptoDate(sc.Height)
	return nil
}

func (chain *StampingChain) writeStampingCertificate(sc *types.StampingCertificate) error {
	if err := chain.eth.BlockChain().WriteStampingCertificate(sc); err != nil {
		return err
	}
	return nil
}

func (chain *StampingChain) keepStampingChainUptoDate(height uint64) {
	futureStampingStatus := chain.precomputedFutureStampingStatus(height)
	chain.writeFutureStatusToDb(&futureStampingStatus)
	if chain.doKeepStampingStatusUptoDate(height) {
		chain.writeStatusToDb()
		chain.broadcastStampingStatusMsg()
	}
}

func (chain *StampingChain) precomputedFutureStampingStatus(height uint64) (stampingStatus types.StampingStatus) {
	stampingStatus = chain.stampingStatus

	if height-chain.stampingStatus.Proof <= chain.config.Stamping.B {
		stampingStatus.Candidate = height
	} else {
		stampingStatus.Fz = chain.stampingStatus.Proof
		stampingStatus.Proof = chain.stampingStatus.Candidate
		stampingStatus.Candidate = height
	}
	return
}

func (chain *StampingChain) doKeepStampingStatusUptoDate(height uint64) bool {
	if height <= chain.stampingStatus.Candidate {
		return false
	}

	// delete fc
	// max(N-B, C+1, B+1)
	start := MaxUint64(height-chain.config.Stamping.B+1, chain.stampingStatus.Candidate+1)
	chain.deleteFC(start, height)

	if height-chain.stampingStatus.Proof <= chain.config.Stamping.B {
		chain.stampingStatus.Candidate = height
	} else {
		chain.freezeProof()
		chain.stampingStatus.Fz = chain.stampingStatus.Proof
		chain.stampingStatus.Proof = chain.stampingStatus.Candidate
		chain.stampingStatus.Candidate = height
	}

	// trim( max(QB - B, Fz), min((C-B), QB)) // 开区间
	start = MaxUint64(chain.stampingStatus.Proof-chain.config.Stamping.B, chain.stampingStatus.Fz)
	end := MinUint64(chain.stampingStatus.Candidate-chain.config.Stamping.B, chain.stampingStatus.Proof)
	n := chain.trim(start, end)
	_ = n
	//fmt.Printf("trim range=[%d, %d] trimmed=%d/%d\n", start, end, n, end-start-1)

	return true
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
		header := chain.header(i)
		if header != nil {
			headerNoCert := types.CopyNonCertHeader(header)
			if err := chain.writeNonCertificateHeader(headerNoCert); err != nil {
				log.Error("write non certificate header failed", "height", i, "err", err)
			}
		}
	}

	return count
}

func (chain *StampingChain) freezeProof() {
	// freeze( max(Fz, QB-B), min(C-B, QB))
	start := MaxUint64(chain.stampingStatus.Fz+1, chain.stampingStatus.Proof-chain.config.Stamping.B+1)
	headerEnd := MinUint64(chain.stampingStatus.Candidate-chain.config.Stamping.B, chain.stampingStatus.Proof)
	end := chain.stampingStatus.Proof

	//trim the tail of P to keep its length minimal
	for height := start; height < headerEnd; height++ {
		//delete(chain.stampingChain, height)
		chain.eth.BlockChain().DeleteStampingCertificate(height)
		// TODO: delete headerchain
		//delete(chain.headerChain, height)
		chain.eth.BlockChain().WriteDeleteHeaderTag(height)
	}

	// delete sc from the minimal tail
	for height := headerEnd; height < end; height++ {
		//delete(chain.stampingChain, height)
		chain.eth.BlockChain().DeleteStampingCertificate(height)
	}
}

func (chain *StampingChain) trim(start, end uint64) int {
	count := 0
	for height := end - 1; height > start; height-- {
		if chain.header(height) == nil && chain.stampingCertificate(height) == nil {
			break
		}

		//delete(chain.stampingChain, height)
		chain.eth.BlockChain().DeleteStampingCertificate(height)
		// TODO: delete headerchain
		//delete(chain.headerChain, height)
		chain.eth.BlockChain().WriteDeleteHeaderTag(height)
		count += 1
	}

	return count
}

func (chain *StampingChain) print() string {
	if chain.stampingStatus.Height <= chain.config.Stamping.HeightB() {
		return ""
	}
	result, count := chain.printRange(chain.config.Stamping.HeightB(), chain.stampingStatus.Height+1)

	result += fmt.Sprintf("Status: Fz=%d, Proof=%d, Candidate=%d\n", chain.stampingStatus.Fz, chain.stampingStatus.Proof, chain.stampingStatus.Candidate)
	result += fmt.Sprintf("MaxHeight=%d, realLength=%d, percent=%.2f%%\n", chain.stampingStatus.Height, count, float64(count*10000/chain.stampingStatus.Height)/100)

	return result
}

func (chain *StampingChain) Print() string {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	return chain.print()
}

func (chain *StampingChain) PrintProperty() {
	countBreadcrumb := 0
	countTail := 0
	sumTailLength := 0
	countForward := 0
	sumForwardLength := 0

	for begin, end := chain.config.Stamping.HeightB()+1, chain.config.Stamping.HeightB()+chain.config.Stamping.B; end <= chain.stampingStatus.Fz; {
		breadcrumb, err := chain.getNextBreadcrumb(begin, end, chain.stampingStatus)
		if err != nil {
			panic("invalid chain")
		}

		countBreadcrumb++
		if breadcrumb.StampingHeader != nil {
			if n := len(breadcrumb.Tail); n > 0 {
				countTail++
				sumTailLength += n
			}

			begin = breadcrumb.StampingHeader.NumberU64() + 1
			end = (breadcrumb.StampingHeader.NumberU64() - uint64(len(breadcrumb.Tail))) + chain.config.Stamping.B
		} else {
			countForward++
			sumForwardLength += len(breadcrumb.ForwardHeader)

			lastOne := breadcrumb.ForwardHeader[len(breadcrumb.ForwardHeader)-1]
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

func (chain *StampingChain) printRange(begin, end uint64) (string, uint64) {
	var result string
	const perLine = 8

	lastPrinted := uint64(0)
	count := uint64(0)
	for height := begin; height < end; height++ {
		header := chain.header(height)
		sc := chain.stampingCertificate(height)

		if header == nil {
			if sc != nil {
				panic(fmt.Sprintf("Unexpected! No header, but has SC, height=%d", height))
			}
			continue
		}

		hasParent := lastPrinted == height-1
		lastPrinted = height

		result += fmt.Sprintf("%s", chain.formatHeader(header, sc, hasParent))
		if count++; count%perLine == 0 {
			result += fmt.Sprintln()
		}
	}

	if count%perLine != 0 {
		result += fmt.Sprintln()
	}

	return result, count
}

func (chain *StampingChain) formatHeader(header *types.Header, sc *types.StampingCertificate, hasParent bool) string {
	height := header.NumberU64()
	fcTag := ""
	if header.Certificate != nil {
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

	return fmt.Sprintf("%2s[%5d(%1s%1s%1s)]", arrow, height, zpcTag, fcTag, scTag)
}

func (chain *StampingChain) HeaderAndFinalCertificate(height uint64) *types.Header {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	return chain.header(height)
}

func (chain *StampingChain) StatusString() string {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	return chain.stampingStatus.String()
}

func (chain *StampingChain) ChainStatus() types.StampingStatus {
	chain.mutexChain.RLock()
	defer chain.mutexChain.RUnlock()

	return chain.stampingStatus
}

func (chain *StampingChain) forwardSyncRangeByHeaderAndFinalCertificate(p *peer, start, end uint64) error {
	log.Debug("forwardSyncRangeByHeaderAndFinalCertificate", "peer", p.ID().String(), "start", start, "end", end)
	for height := start; height <= end && height <= p.ChainStatus().Height; height++ {
		if chain.hasHeader(height) {
			continue
		}

		headers := p.GetHeaders(height, height, true, true)
		if headers == nil || headers[0] == nil {
			return fmt.Errorf("p has no header or fc at height(%d)", height)
		}

		if err := chain.addForwardBlock(headers[0]); err != nil {
			return err
		}
	}

	return nil
}

func (chain *StampingChain) backwardSyncRangeOnlyByHeader(p *peer, start, end uint64, endHash common.Hash) error {
	headers := p.GetHeaders(start, end, false, false)
	if len(headers) == 0 {
		return fmt.Errorf("peer has no header(%d-%d)", start, end)
	}

	for _, header := range headers {
		height := header.NumberU64()
		if chain.hasHeader(height) {
			continue
		}

		var err error
		if height == end {
			err = chain.addHeaderWithHash(header, endHash)
		} else {
			err = chain.addBackwardHeader(header)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func (chain *StampingChain) getNextBreadcrumb(begin, end uint64, status types.StampingStatus) (bc *breadcrumb, err error) {
	defer func() {
		log.Debug("getNextBreadcrumb return", "begin", begin, "end", end, "peer.status", status.String(), "bc", bc.String())
	}()

	bc = &breadcrumb{}
	if begin < status.Fz || begin > end {
		return bc, fmt.Errorf("param err, begin:%d, end:%d, status:%s", begin, end, status.String())
	}

	start := MinUint64(end, chain.stampingStatus.Candidate)
	for height := start; height >= begin; height-- {
		if sc := chain.stampingCertificate(height); sc != nil {
			header := chain.header(height)
			if header == nil {
				panic(fmt.Sprintf("cannot find header(%d)", height))
			}

			bc.StampingHeader = header
			bc.StampingCertificate = sc

			// rollback tail
			for h := height - 1; h >= begin; h-- {
				header := chain.header(h)
				if header == nil {
					break
				}

				bc.Tail = append(bc.Tail, types.CopyNonCertHeader(header))
			}

			return bc, nil
		}
	}

	start = MaxUint64(begin, status.Height+1)
	for height := start; height <= chain.stampingStatus.Height; height++ {
		header := chain.header(height)
		if header == nil {
			//panic(fmt.Sprintf("cannot find header(%d)", height))
			break
		}

		bc.ForwardHeader = append(bc.ForwardHeader, header)

		forwardHeight := height + chain.config.Stamping.B
		if sc := chain.stampingCertificate(forwardHeight); sc != nil {
			break
		}
	}

	return bc, nil
}

func (chain *StampingChain) getHeaders(begin, end uint64, forward, includeFc bool) (headers []*types.Header) {
	if forward {
		for height := begin; height <= end; height++ {
			header := chain.header(height)
			if header == nil {
				headers = nil
				return
			}
			if !includeFc {
				header.Certificate.TrieProof = nil
			}

			headers = append(headers, header)
		}
	} else {
		for height := end; height >= begin; height-- {
			header := chain.header(height)
			if header == nil {
				headers = nil
				return
			}
			if !includeFc {
				header.Certificate.TrieProof = nil
			}

			headers = append(headers, header)
		}
	}

	return
}

type breadcrumb struct {
	StampingHeader      *types.Header              `rlp:"nil"`
	StampingCertificate *types.StampingCertificate `rlp:"nil"`
	Tail                []*types.Header            `rlp:"nil"`

	ForwardHeader []*types.Header `rlp:"nil"`
	//forwardFinalCertificate []*FinalCertificate
}

func (bc *breadcrumb) String() string {
	if bc.StampingCertificate == nil {
		return fmt.Sprintf("sc:nil, tail len:%d, forward len:%d", len(bc.Tail), len(bc.ForwardHeader))
	}
	return fmt.Sprintf("sc:%d, tail len:%d, forward len:%d", bc.StampingCertificate.Height, len(bc.Tail), len(bc.ForwardHeader))
}

func (chain *StampingChain) syncNextBreadcrumb(p *peer, begin, end uint64) (nextBegin, nextEnd uint64, err error) {
	var bc *breadcrumb
	bc, err = p.GetNextBreadcrumb(begin, end, chain.stampingStatus)
	if err != nil {
		return
	}

	if bc.StampingHeader == nil && len(bc.ForwardHeader) < 1 {
		err = fmt.Errorf("get no data from peer:%s", p.ID())
		return
	}

	if bc.StampingHeader != nil {
		if proofHeader := chain.header(bc.StampingHeader.Number.Uint64() - chain.config.Stamping.B); proofHeader == nil {
			tailLength := chain.getTailLength(begin)
			neededBegin := bc.StampingHeader.Number.Uint64() - chain.config.Stamping.B
			neededEnd := begin - 1 - tailLength
			if err = chain.syncAllHeaders(p, neededBegin, neededEnd); err != nil {
				// TODO: switch to full node
				archive := chain.pm.GetArchivePeer()
				if err = chain.syncAllHeaders(archive, neededBegin, neededEnd); err != nil {
					/*fmt.Printf("syncAllHeaders, peer:%s, begin:end=[%d, %d] [%d, %d], proof:%d, tailLength:%d\n",
					peer.id, begin, end, neededBegin, neededEnd, bc.StampingHeader.Height-chain.config.Stamping.B, tailLength)*/
					return
				}
			}
		}

		if err = chain.syncBreadcrumbWithTailHeader(bc.StampingHeader, bc.StampingCertificate, bc.Tail); err != nil {
			return
		}

		nextBegin = chain.stampingStatus.Height + 1
		nextEnd = (chain.stampingStatus.Height - uint64(len(bc.Tail))) + chain.config.Stamping.B
	} else {
		for _, h := range bc.ForwardHeader {
			err = chain.addForwardBlock(h)
			if err == errExists || err == errLowerThanHeight {
				continue
			}
			if err != nil {
				return
			}
		}

		nextBegin = chain.stampingStatus.Height + 1
		nextEnd = chain.stampingStatus.Height + chain.config.Stamping.B
	}

	return
}

func (chain *StampingChain) syncBreadcrumbWithTailHeader(header *types.Header, sc *types.StampingCertificate, tail []*types.Header) error {
	height := header.NumberU64()
	log.Trace("syncBreadcrumbWithTailHeader", "header.Height", height, "sc.Height", sc.Height)
	if sc.Height <= chain.stampingStatus.Candidate {
		return fmt.Errorf("sc(%d) lower than stamping status(%s)", sc.Height, chain.stampingStatus.String())
	}

	if err := chain.verifyStampingCertificate(header, sc); err != nil {
		return err
	}

	if err := chain.writeHeader(header); err != nil {
		return err
	}

	if err := chain.writeStampingCertificate(sc); err != nil {
		return err
	}
	chain.updateStatusHeight(height)
	if chain.doKeepStampingStatusUptoDate(height) {
		for _, tailHeader := range tail {
			err := chain.addBackwardHeader(tailHeader)
			if err != nil {
				return err
			}
		}

		chain.writeStatusToDb()
		chain.broadcastStampingStatusMsg()
	}
	return nil
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
	headers := p.GetHeaders(begin, end, false, false)
	if len(headers) == 0 || headers[len(headers)-1].NumberU64() != begin {
		err = fmt.Errorf("peer do not have 'begin(%d)'", begin)
		return
	}

	for _, tailHeader := range headers {
		err = chain.addBackwardHeader(tailHeader)
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
	chain.writeStatusToDb()
	chain.broadcastStampingStatusMsg()

	if err := chain.handleStampingVote(stampingEvent.Vote); err != nil {
		log.Error("handleStampingVote error", "status", chain.stampingStatus, "err", err)
		return
	}

	log.Trace("handleStampingEvent done", "status", chain.stampingStatus, "vote", stampingEvent.Vote.String())

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
	log.Trace("handleUpdateStatus", "Status", chain.stampingStatus)

	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

	currentBlock := chain.eth.BlockChain().CurrentBlock()
	chain.stampingStatus.Height = currentBlock.NumberU64()

	log.Trace("handleUpdateStatus done", "Status", chain.stampingStatus)
}

func (chain *StampingChain) broadcastStampingStatusMsg() {
	chain.pm.Broadcast(StampingStatusMsg, &chain.stampingStatus)
}

func (chain *StampingChain) handleStampingVote(vote *types.StampingVote) error {
	log.Trace("handleStampingVote", "vote", vote)
	if vote == nil {
		return fmt.Errorf("vote is nil")
	}
	if vote.Height <= chain.config.Stamping.HeightB() {
		return fmt.Errorf("vote is invalid, vote.Height(%d) is lower than B(%d)", vote.Height, chain.config.Stamping.HeightB())
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

	stateDb, err := algorand.GetStateDbFromProof(vote.TrieProof, proofHeader.Root)
	if err != nil {
		return err
	}

	mv := algorandCore.GetMinerVerifier(chain.config.Algorand, stateDb, vote.Address, vote.Height)
	err = algorandCore.VerifyStampingSignatureAndCredential(mv, vote.SignBytes(), vote.ESignValue, &vote.Credential, stateDb, proofHeader.Seed(), proofHeader.TotalBalanceOfMiners)
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
			if height <= chain.stampingStatus.Candidate {
				continue
			}
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
			if height <= maxEnoughVotesHeight || height <= chain.stampingStatus.Candidate {
				delete(chain.buildingStampingVoteWindow, height)
			}
		}

		chain.pm.Broadcast(StampingStatusMsg, &chain.stampingStatus)
	}
	log.Trace("check enough done", "max height", maxEnoughVotesHeight)

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
			delete(chain.belowStampingVoteWindow, height)
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

			p.Log().Info("gossipVoteData vote below F", "status", chain.stampingStatus,
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
	for height := begin; height <= end; height++ {
		votes := chain.buildingStampingVoteWindow[height]
		if err := p.PickBuildingAndSend(votes); err == nil {
			sent = true

			p.Log().Info("gossipVoteData vote between C and H", "status", chain.stampingStatus,
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

func (chain *StampingChain) syncer() {
	forceSync := time.NewTicker(forceSyncCycle)
	defer forceSync.Stop()

	for {
		select {
		case <-chain.pm.newPeerCh:
			// Make sure we have peers to select from, then sync
			if chain.pm.peers.Len() < minDesiredPeerCount {
				break
			}
			go chain.Sync()

		case <-forceSync.C:
			// Force a sync even if not enough peers are present
			go chain.Sync()

		case <-chain.pm.quitSync:
			return
		}
	}
}

func (chain *StampingChain) Sync() {
	if !atomic.CompareAndSwapInt32(&chain.synchronising, 0, 1) {
		log.Trace("sync is busy.")
	}
	defer atomic.StoreInt32(&chain.synchronising, 0)

	if err := chain.syncWithPeer(); err != nil {
		log.Error("sync failed", "err", err)
		//panic(fmt.Sprintf("sync failed, err:%s", err))
		return
	}
}

func (chain *StampingChain) syncWithPeer() error {
	peer := chain.pm.GetBestPeer()
	if peer == nil {
		return nil
	}

	chain.mutexChain.Lock()
	defer chain.mutexChain.Unlock()

	log.Trace("begin sync", "peer", peer.ID().String(), "chain", chain.stampingStatus.String(), "peer", peer.statusString())

	err := chain.sync(peer)
	log.Trace("end sync", "peer", peer.ID().String(), "err", err)
	//chain.print()
	return err
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

	peerStatus := peer.ChainStatus()
	// sync the first b range [Base+1, Base+B] if needed
	baseHeight := chain.config.Stamping.BaseHeight
	if start, end := baseHeight+1, baseHeight+chain.config.Stamping.B; chain.stampingStatus.Height < end && chain.stampingStatus.Height < peerStatus.Height {
		err := chain.forwardSyncRangeByHeaderAndFinalCertificate(peer, start, end)
		if err != nil {
			return fmt.Errorf("forward synchronize the first b blocks failed: %v", err)
		}
	}

	// H+1 - peer.currentHeight
	for begin, end := chain.stampingStatus.Candidate+1, chain.stampingStatus.Candidate+chain.config.Stamping.B; chain.stampingStatus.Height < peerStatus.Height; {
		//fmt.Printf("process begin:[%d, %d]\n", begin, end)
		nextBegin, nextEnd, err := chain.syncNextBreadcrumb(peer, begin, end)
		if err != nil {
			return fmt.Errorf("synchronize breadcrumb in range[%d,%d] failed: %v", begin, end, err)
		}
		//fmt.Printf("process end:[%d, %d]\n", nextBegin, nextEnd)
		begin, end = nextBegin, nextEnd
	}

	if chain.stampingStatus.Height < peerStatus.Height {
		log.Warn("dont reach latest status, return", "chain", chain.stampingStatus, "peer", peerStatus.String())
		return fmt.Errorf("donot reach latest status, chain:%s, peer:%s", chain.stampingStatus.String(), peerStatus.String())
	}

	return chain.fetchLatestBlock(peer)
}

func (chain *StampingChain) fetchLatestBlock(p *peer) error {
	if chain.stampingStatus.Height <= 1 {
		return fmt.Errorf("sync failed, height(%d) <= 1", chain.stampingStatus.Height)
	}

	latest := chain.header(chain.stampingStatus.Height)
	if latest == nil {
		return fmt.Errorf("laster header not exist, height:%d", chain.stampingStatus.Height)
	}

	var block *blockData
	var err error
	if !chain.eth.BlockChain().HasFastBlock(latest.Hash(), latest.NumberU64()) {
		block, err = p.GetBlockAndReceipts(latest.Hash())
		if err != nil {
			p.Log().Error("GetBlockAndReceipts failed", "chain", chain.stampingStatus.String())
			return err
		}
	}

	if !chain.eth.BlockChain().HasState(latest.Root) {
		if err := chain.downloader.FetchNodeData(latest.Root); err != nil {
			log.Error("fetch state failed", "height", latest.NumberU64(), "err", err)
			return err
		}
	}

	if block != nil {
		// insert block and receipts
		return chain.commitPivotBlock(block.Block, block.Receipts)
	}

	return nil
}

func (chain *StampingChain) commitPivotBlock(block *types.Block, receipts types.Receipts) error {
	if err := chain.eth.BlockChain().InsertBlockAndReceipt(block, receipts); err != nil {
		log.Error("insert block and receipt failed", "height", block.NumberU64(), "err", err)
		return err
	}

	if err := chain.eth.BlockChain().FastSyncCommitHead(block.Hash()); err != nil {
		log.Error("update currentBlock failed", "height", block.NumberU64(), "err", err)
		return err
	}

	events := make([]interface{}, 0, 1)
	events = append(events, core.ChainHeadEvent{Block: block})
	chain.eth.BlockChain().PostChainEvents(events, nil)

	log.Info("commitPivotBlock success", "height", block.NumberU64())
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

		if !EqualFinalCertificate(header.Certificate, oHeader.Certificate) {
			return false, fmt.Errorf("fc not equal, height:%d", height)
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

		if !EqualFinalCertificate(header.Certificate, oHeader.Certificate) {
			return false, fmt.Errorf("fc not equal, height:%d", height)
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

		if !EqualFinalCertificate(header.Certificate, oHeader.Certificate) {
			return false, fmt.Errorf("fc not equal, height:%d", height)
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

		if !EqualFinalCertificate(header.Certificate, oHeader.Certificate) {
			return false, fmt.Errorf("fc not equal, height:%d", height)
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

		if !EqualFinalCertificate(header.Certificate, oHeader.Certificate) {
			return false, fmt.Errorf("fc not equal, height:%d", height)
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

func EqualFinalCertificate(a, b *types.Certificate) bool {
	if a == nil && b == nil {
		return true
	}

	if a != nil && b != nil {
		return a.Height == b.Height && a.Round == b.Round && a.Value == b.Value
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
