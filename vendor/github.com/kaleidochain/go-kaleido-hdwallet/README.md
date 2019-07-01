# go-kaleido-hdwallet

> Kaleido HD Wallet is forked from [Ethereum HD Wallet](https://github.com/miguelmota/go-ethereum-hdwallet), which derivations from [mnemonic] seed in Go (golang). Implements the [kaleido](https://github.com/kaleidochain/kaleido) [`accounts.Wallet`](https://github.com/kaleidochain/kaleido/blob/master/accounts/accounts.go) interface by renaming `ethereum/go-ethereum` to `kaleidochain/kaleido` mainly in imports([see details](https://github.com/kaleidochain/go-kaleido-hdwallet/commit/0b0e8151540affa3c184d960f081db1e48c991d6)).

[![License](http://img.shields.io/badge/license-MIT-blue.svg)](https://raw.githubusercontent.com/miguelmota/go-ethereum-hdwallet/master/LICENSE) [![GoDoc](https://godoc.org/github.com/kaleidochain/go-kaleido-hdwallet?status.svg)](https://godoc.org/github.com/kaleidochain/go-kaleido-hdwallet)

## Install

```bash
go get -u github.com/kaleidochain/go-kaleido-hdwallet
```

## Documenation

[https://godoc.org/github.com/kaleidochain/go-kaleido-hdwallet](https://godoc.org/github.com/kaleidochain/go-kaleido-hdwallet)

## Getting started

```go
package main

import (
	"fmt"
	"log"

	"github.com/kaleidochain/go-kaleido-hdwallet"
)

func main() {
	mnemonic := "tag volcano eight thank tide danger coast health above argue embrace heavy"
	wallet, err := hdwallet.NewFromMnemonic(mnemonic)
	if err != nil {
		log.Fatal(err)
	}

	path := hdwallet.MustParseDerivationPath("m/44'/60'/0'/0/0")
	account, err := wallet.Derive(path, false)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(account.Address.Hex()) // 0xC49926C4124cEe1cbA0Ea94Ea31a6c12318df947

	path = hdwallet.MustParseDerivationPath("m/44'/60'/0'/0/1")
	account, err = wallet.Derive(path, false)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(account.Address.Hex()) // 0x8230645aC28A4EdD1b0B53E7Cd8019744E9dD559
}
```

### Signing transaction

```go
package main

import (
	"log"
	"math/big"

	"github.com/davecgh/go-spew/spew"
	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/core/types"
	"github.com/kaleidochain/go-kaleido-hdwallet"
)

func main() {
	mnemonic := "tag volcano eight thank tide danger coast health above argue embrace heavy"
	wallet, err := hdwallet.NewFromMnemonic(mnemonic)
	if err != nil {
		log.Fatal(err)
	}

	path := hdwallet.MustParseDerivationPath("m/44'/60'/0'/0/0")
	account, err := wallet.Derive(path, true)
	if err != nil {
		log.Fatal(err)
	}

	nonce := uint64(0)
	value := big.NewInt(1000000000000000000)
	toAddress := common.HexToAddress("0x0")
	gasLimit := uint64(21000)
	gasPrice := big.NewInt(21000000000)
	var data []byte

	tx := types.NewTransaction(nonce, toAddress, value, gasLimit, gasPrice, data)
	signedTx, err := wallet.SignTx(account, tx, nil)
	if err != nil {
		log.Fatal(err)
	}

	spew.Dump(signedTx)
}
```

## Test

```bash
make test
```

## License

[MIT](LICENSE)
