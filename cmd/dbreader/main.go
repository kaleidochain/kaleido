package main

import (
	"flag"
	"fmt"

	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/core/rawdb"

	"github.com/kaleidochain/kaleido/ethdb"
)

var (
	dbPath  = flag.String("path", "", "DB Path")
	height  = flag.Uint64("height", 0, "height")
	hashStr = flag.String("hash", "", "hash")
)

func main() {
	flag.Parse()

	db, err := ethdb.NewLDBDatabase(*dbPath, 12, 12)
	if err != nil {
		fmt.Printf("open db error: %s\n", err)
		return
	}

	hash := common.HexToHash(*hashStr)
	hasBody := rawdb.HasBody(db, hash, *height)
	hasHeader := rawdb.HasHeader(db, hash, *height)

	fmt.Printf("height:%d, body: %t, hasHeader:%t\n", *height, hasBody, hasHeader)
}
