// Copyright (c) 2019 The kaleido Authors
// This file is part of kaleido
//
// kaleido is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// kaleido is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with kaleido. If not, see <https://www.gnu.org/licenses/>.

package core

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/contracts"
	"github.com/kaleidochain/kaleido/core/state"
	"github.com/kaleidochain/kaleido/core/types"
	"github.com/kaleidochain/kaleido/crypto"
	"github.com/kaleidochain/kaleido/ethdb"
	"github.com/kaleidochain/kaleido/params"
	"github.com/kaleidochain/kaleido/trie"
)

func proveForAddress(address common.Address, stateDb *state.StateDB, height uint64, minerContract *state.MinerContract, proofDb ethdb.Putter) (err error) {
	err = stateDb.Prove(address, 0, proofDb)
	if err != nil {
		return
	}

	key := minerContract.MakeMinerInfoKey(height, address)
	err = stateDb.StorageProve(contracts.MinerAddress, key, 0, proofDb)
	if err != nil {
		return
	}

	offset := key.Big()
	offset = offset.Add(offset, common.Big1)
	err = stateDb.StorageProve(contracts.MinerAddress, common.BigToHash(offset), 0, proofDb)
	if err != nil {
		return
	}

	offset = offset.Add(offset, common.Big1)
	err = stateDb.StorageProve(contracts.MinerAddress, common.BigToHash(offset), 0, proofDb)
	if err != nil {
		return
	}

	return
}

func BuildProofForStorage(config *params.AlgorandConfig, stateDb *state.StateDB, stateRoot common.Hash, height uint64, leader common.Address, cvs []*types.CertVoteStorage, proofDb *types.NodeSet) (err error) {
	addresses := getCommitteeAddresses(cvs, leader)

	return BuildProof(config, stateDb, height, addresses, proofDb)
}

func BuildProof(config *params.AlgorandConfig, stateDb *state.StateDB, height uint64, addresses []common.Address, proofDb *types.NodeSet) (err error) {
	minerContract := state.NewMinerContract(config)

	err = stateDb.Prove(contracts.MinerAddress, 0, proofDb)
	if err != nil {
		return
	}

	for _, address := range addresses {
		err = proveForAddress(address, stateDb, height, minerContract, proofDb)
		if err != nil {
			return
		}
	}

	return
}

func verifyProofMustHasKey(stateRoot common.Hash, key []byte, proofDb trie.DatabaseReader) (err error) {
	var value []byte
	value, _, err = trie.VerifyProof(stateRoot, key, proofDb)
	if err != nil {
		return
	}
	if len(value) == 0 {
		err = fmt.Errorf("proof node dont contain key %x", key)
		return
	}
	return
}

func verifyProofForAddress(address common.Address, minerContract *state.MinerContract, stateRoot, minerContractRoot common.Hash, height uint64, proofDb trie.DatabaseReader) (err error) {
	err = verifyProofMustHasKey(stateRoot, crypto.Keccak256(address.Bytes()), proofDb)
	if err != nil {
		return
	}

	key := minerContract.MakeMinerInfoKey(height, address)
	err = verifyProofMustHasKey(minerContractRoot, crypto.Keccak256(key.Bytes()), proofDb)
	if err != nil {
		return
	}

	offset := key.Big()
	offset = offset.Add(offset, common.Big1)
	err = verifyProofMustHasKey(minerContractRoot, crypto.Keccak256(offset.Bytes()), proofDb)
	if err != nil {
		return
	}

	offset = offset.Add(offset, common.Big1)
	err = verifyProofMustHasKey(minerContractRoot, crypto.Keccak256(offset.Bytes()), proofDb)
	if err != nil {
		return
	}

	return
}

func VerifyProofForStorage(config *params.AlgorandConfig, stateRoot common.Hash, height uint64, leader common.Address, cvs []*types.CertVoteStorage, proof types.NodeList) error {
	addresses := getCommitteeAddresses(cvs, leader)

	return VerifyProof(config, stateRoot, height, addresses, proof)
}

func getCommitteeAddresses(cvs []*types.CertVoteStorage, leader common.Address) []common.Address {
	var addresses []common.Address

	leaderNotIncluded := true
	for _, cert := range cvs {
		if leaderNotIncluded && cert.Credential.Address == leader {
			leaderNotIncluded = false
		}
		addresses = append(addresses, cert.Credential.Address)
	}

	if leaderNotIncluded {
		addresses = append(addresses, leader)
	}

	return addresses
}

func VerifyProof(config *params.AlgorandConfig, stateRoot common.Hash, height uint64, addresses []common.Address, proof types.NodeList) error {
	nodeSet := proof.NodeSet()

	reads := &readTraceDB{db: nodeSet}

	enc, _, err := trie.VerifyProof(stateRoot, crypto.Keccak256(contracts.MinerAddress.Bytes()), reads)
	if err != nil {
		return err
	}

	var data state.Account
	if err := rlp.DecodeBytes(enc, &data); err != nil {
		log.Error("Failed to decode state object", "addr", contracts.MinerAddress, "err", err)
		return err
	}

	minerContractRoot := data.Root
	minerContract := state.NewMinerContract(config)

	for _, address := range addresses {
		err = verifyProofForAddress(address, minerContract, stateRoot, minerContractRoot, height, reads)
		if err != nil {
			return err
		}
	}

	// check if all nodes have been read by VerifyProofForStorage
	if len(reads.reads) != len(proof) {
		return errors.New("useless nodes in merkle proof nodeset")
	}

	return nil
}

// readTraceDB stores the keys of database reads. We use this to check that received node
// sets contain only the trie nodes necessary to make proofs pass.
type readTraceDB struct {
	db    trie.DatabaseReader
	reads map[string]struct{}
}

// Get returns a stored node
func (db *readTraceDB) Get(k []byte) ([]byte, error) {
	if db.reads == nil {
		db.reads = make(map[string]struct{})
	}
	db.reads[string(k)] = struct{}{}
	return db.db.Get(k)
}

// Has returns true if the node set contains the given key
func (db *readTraceDB) Has(key []byte) (bool, error) {
	_, err := db.Get(key)
	return err == nil, nil
}
