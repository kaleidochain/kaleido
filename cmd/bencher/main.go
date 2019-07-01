package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"flag"
	"io"
	"math/big"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"time"

	"github.com/kaleidochain/kaleido/core/types"
	"github.com/kaleidochain/kaleido/crypto"

	"github.com/kaleidochain/kaleido/common"

	"github.com/ethereum/go-ethereum/log"
	"github.com/kaleidochain/kaleido/ethclient"
)

var (
	hostFlag            = flag.String("host", "http://192.168.0.152:8545", "the RPC kalgo node's url")
	accountFileFlag     = flag.String("accountfile", "account.file", "account file")
	prikeyFileFlag      = flag.String("prikeyfile", "prikey.file", "account file")
	beginAccountsFlag   = flag.Uint64("begin", 0, "Begin of account used to send txs")
	howmuchAccountsFlag = flag.Uint64("howmuchaccount", 500, "Num of accounts used to send txs")
	tpsFlag             = flag.Int("tps", 5000, "target tps")
	chainidFlag         = flag.Uint64("chainid", 889, "Chain id identifies the connected chain")
	logFlag             = flag.Int("loglevel", 3, "Log level")
	senderFlag          = flag.Int("sender", 3, "SenderWorker count")
	producerFlag        = flag.Int("producer", 1, "Producer Signed Tx Worker count")
	transferFlag        = flag.Int("transfer", 0, "transfer some eth to Eth or not(default:0)")
	maxPendingTxs       = flag.Int("maxpending", 0, "keep pending txs below this value")
)

func main() {
	flag.Parse()
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(*logFlag), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	//
	setup()

	abortChan := make(chan os.Signal)
	signal.Notify(abortChan, os.Interrupt)

	//
	b := NewBencher()

	if err := b.loadAccounts(); err != nil {
		log.Error("Failed Load Accounts", "err", err)
		return
	}
	log.Info("Load Accounts Ok", "num", len(b.accounts))

	if err := b.loadPrikeys(); err != nil {
		log.Error("Failed Load Prikeys", "err", err)
		return
	}
	log.Info("Load Prikeys Ok", "num", len(b.prikeys))

	if len(b.accounts) != len(b.prikeys) {
		log.Crit("Failed Count Of Accounts and Prikeys",
			"len(accounts)", len(b.accounts), "len(prikeys)", len(b.prikeys))
		return
	}

	if err := b.loadNonces(); err != nil {
		log.Error("Failed Load Nonce", "err", err)
		return
	}
	log.Info("Load Nonce Ok", "num", len(b.nonces))

	if *transferFlag > 0 {
		if err := b.transfer(); err != nil {
			log.Error("Failed Transfer", "err", err)
			return
		}
		log.Info("Transfer Ok", "num", *howmuchAccountsFlag)
		return
	}

	go b.producer()
	go deliver(b.txsCh, b.targetTps)

	<-abortChan
}

func setup() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func readLines(path string) (lines []string, err error) {
	var (
		file   *os.File
		part   []byte
		prefix bool
	)

	if file, err = os.Open(path); err != nil {
		return
	}

	reader := bufio.NewReader(file)
	buffer := bytes.NewBuffer(make([]byte, 0))

	for {
		if part, prefix, err = reader.ReadLine(); err != nil {
			break
		}
		buffer.Write(part)
		if !prefix {
			lines = append(lines, buffer.String())
			buffer.Reset()
		}
	}
	if err == io.EOF {
		err = nil
	}
	return
}

func NewBencher() *bencher {
	return &bencher{
		begin:           *beginAccountsFlag,
		howmuchAccounts: *howmuchAccountsFlag,
		targetTps:       *tpsFlag,
		txsCh:           make(chan *types.Transaction, *tpsFlag*2),
	}
}

type bencher struct {
	nonces          []uint64
	begin           uint64
	howmuchAccounts uint64
	targetTps       int

	accounts []string
	prikeys  []*ecdsa.PrivateKey

	txsCh chan *types.Transaction
}

func dialClient() *ethclient.Client {
	client, err := ethclient.Dial(*hostFlag)
	if err != nil {
		log.Crit("Failed to dial rpc", "rpc host url", *hostFlag, "err", err)
		return nil
	}

	return client
}

func (b *bencher) transfer() error {
	bank := "0xb2Df2284da2cf81C5FaC17217d69B024B0B99412"
	bankPrikey := "EE51AEB9481CD8924DE1567DA4C19F79466B1E29382D4FC51671D3A16251F3BE" //default:0

	client := dialClient()
	defer client.Close()

	nonce, err := client.NonceAt(context.Background(), common.HexToAddress(bank), nil)
	if err != nil {
		log.Error("Failed Load Nonce", "account", bank, "err", err)
		return err
	}

	for i := 0; i < len(b.accounts) && i < int(b.howmuchAccounts); i++ {
		to := common.HexToAddress(b.accounts[i])
		amount := new(big.Int).SetUint64(1).Mul(new(big.Int).SetUint64(10000), new(big.Int).SetUint64(1e18))
		gasLimit := uint64(21000)
		gasPrice := new(big.Int).SetUint64(1).Mul(new(big.Int).SetUint64(1), new(big.Int).SetUint64(18*1e9))
		tx := types.NewTransaction(nonce, to, amount, gasLimit, gasPrice, nil)
		prikey, err := crypto.HexToECDSA(bankPrikey)
		if err != nil {
			log.Crit("Failed Load prikey", "from", b.accounts[i], "to", strings.ToLower(to.String()), "err", err)
		}
		txWithSigned, err := types.SignTx(tx, types.NewEIP155Signer(new(big.Int).SetUint64(*chainidFlag)), prikey)
		if err != nil {
			log.Crit("Failed Load Nonce", "from", b.accounts[i], "to", strings.ToLower(to.String()),
				"err", err)
			// exit
		}

		if err := client.SendTransaction(context.Background(), txWithSigned); err != nil {
			log.Error("Failed to Transfer", "hash", tx.Hash().String(), "from", strings.ToLower(bank),
				"to", b.accounts[i], "nonce", nonce, "i", i, "err", err)
			return err
		}

		nonce += 1

	}

	return nil
}

func (b *bencher) loadAccounts() error {
	accounts, err := readLines(*accountFileFlag)
	if err != nil {
		log.Error("Failed Read Account File", "file", *accountFileFlag, "err", err)
		return err
	}

	b.accounts = accounts
	return nil
}

func (b *bencher) loadPrikeys() error {
	keys, err := readLines(*prikeyFileFlag)
	if err != nil {
		log.Error("Failed Read Prikey File", "file", *prikeyFileFlag, "err", err)
		return err
	}

	b.prikeys = make([]*ecdsa.PrivateKey, len(keys))
	for index, key := range keys {
		prikey, err := crypto.HexToECDSA(key)
		if err != nil {
			log.Crit("Failed Load prikey", "from", b.accounts[index], "prikey", b.prikeys[index], "err", err)
		}
		b.prikeys[index] = prikey
	}
	return nil
}

func (b *bencher) loadNonces() error {
	out := make(chan int, b.howmuchAccounts)
	b.nonces = make([]uint64, b.howmuchAccounts)
	for i := 0; i < len(b.accounts) && i < int(b.howmuchAccounts); i++ {
		index := i + int(b.begin)

		go func(iIn, indexIn int) {
			client := dialClient()

			addr := common.HexToAddress(b.accounts[indexIn])
			nonce, err := client.NonceAt(context.Background(), addr, nil)
			if err != nil {
				log.Error("Failed Load Nonce", "i", iIn, "i+begin", indexIn, "account", b.accounts[indexIn], "err", err)
			}

			b.nonces[iIn] = nonce

			client.Close()

			out <- indexIn
		}(i, index)
	}

	for i := 0; i < len(b.accounts) && i < int(b.howmuchAccounts); i++ {
		index := <-out
		log.Info("Load Nonce", "i", i, "i+begin", index, "account", b.accounts[index], "nonce", b.nonces[index-int(b.begin)])
	}

	return nil
}

func (b *bencher) producer() {
	b.produceTxWithSigned()
}

type txdata struct {
	from   string
	to     string
	tx     *types.Transaction
	priKey *ecdsa.PrivateKey
}

func signer(in chan txdata, txsCh chan *types.Transaction) {
	for {
		tx := <-in
		txWithSigned, err := types.SignTx(tx.tx, types.NewEIP155Signer(new(big.Int).SetUint64(*chainidFlag)), tx.priKey)
		if err != nil {
			log.Crit("Failed SignTx", "from", tx.from, "to", strings.ToLower(tx.to),
				"err", err)
			// exit
		}

		log.Debug("new tx", "from", tx.from, "to", tx.to,
			"nonce", tx.tx.Nonce(), "hash", txWithSigned.Hash().String())

		txsCh <- txWithSigned
	}
}

func (b *bencher) produceTxWithSigned() {
	factory := make(chan txdata, 2000)
	for i := 0; i < *producerFlag; i++ {
		go signer(factory, b.txsCh)
	}

	for i := 0; ; i++ {
		index := i % int(b.howmuchAccounts)
		index_1 := (i + 1) % int(b.howmuchAccounts)
		indexAccount := index + int(b.begin)
		indexAccount_1 := index_1 + int(b.begin)
		nonce := b.nonces[index]
		to := common.HexToAddress(b.accounts[indexAccount_1])
		amount := new(big.Int).SetUint64(1)
		gasLimit := uint64(21000)
		gasPrice := new(big.Int).SetUint64(18 * 1e9)
		tx := types.NewTransaction(nonce, to, amount, gasLimit, gasPrice, nil)

		in := txdata{
			from:   b.accounts[indexAccount],
			to:     to.String(),
			tx:     tx,
			priKey: b.prikeys[indexAccount],
		}

		factory <- in

		b.nonces[index] += 1

		if i%500 == 0 && len(b.txsCh) < b.targetTps {
			log.Info("cur total txs in chan", "len", len(b.txsCh))
		}
	}
}

func deliver(in chan *types.Transaction, tps int) {
	queue := make(chan *types.Transaction)
	defer close(queue)

	for i := 0; i < *senderFlag; i++ {
		go sender(queue)
	}

	var pendingTxs uint = 0
	go updatePendingTxsCount(&pendingTxs)

	spt := time.Second / time.Duration(tps)
	start := time.Now()

	ptime := time.Now().Add(time.Second)
	counter := 0

	if *maxPendingTxs == 0 {
		*maxPendingTxs = *tpsFlag * 2
	}

	for {
		ts := time.Now()

		if ts.After(ptime) {
			log.Info("deliver progress", "txs", counter, "tps", float32(counter)*float32(time.Second)/float32(ts.Sub(start)))
			ptime = ts.Add(time.Second)
		}

		tx := <-in
		queue <- tx
		counter++

		for pendingTxs >= uint(*maxPendingTxs) {
			log.Info("pending txs too many", "pendingTxs", pendingTxs, "max", *maxPendingTxs)
			time.Sleep(100 * time.Microsecond)
		}
		pendingTxs += 1

		time.Sleep(time.Until(ts.Add(spt)))
	}
}

func updatePendingTxsCount(counter *uint) {
	client := dialClient()
	defer client.Close()

	for {
		*counter, _ = client.GetTxPoolPendingTransactionCount(context.Background())
		time.Sleep(2000 * time.Microsecond)
	}
}

func sender(in chan *types.Transaction) {
	client := dialClient()

	signer := types.NewEIP155Signer(new(big.Int).SetUint64(*chainidFlag))
	for {
		tx := <-in
		if tx == nil {
			break
		}

	FORLOOP:
		for {
			err := client.SendTransaction(context.Background(), tx)

			if err != nil {
				log.Error("Failed to SendTx", "hash", tx.Hash().String(), "err", err)
				client.Close()

				client = dialClient()
			} else {
				from, _ := types.Sender(signer, tx)
				log.Debug("send new tx", "from", from.String(), "to", tx.To().String(),
					"nonce", tx.Nonce(), "hash", tx.Hash().String())
				break FORLOOP
			}
		}
	}

	client.Close()
}
