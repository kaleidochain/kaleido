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
	"io"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
)

type JournalType uint8

const (
	VoteJournalEntry    JournalType = 0
	TimeoutJournalEntry JournalType = 1
)

type rlpEncoder struct {
	Ts   uint64
	Code uint64
	From string
	Data []byte
}

type VoteJournal struct {
	path string

	file *os.File
}

func NewVoteJournal(path string) *VoteJournal {
	return &VoteJournal{
		path: path,
	}
}

func (j *VoteJournal) OpenFile() error {
	file, err := os.OpenFile(j.path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0755)
	if err != nil {
		log.Error("voteJournal openFile error", "err", err)
		return err
	}

	j.file = file
	return nil
}

func (j *VoteJournal) CloseFile() error {
	if j.file != nil {
		err := j.file.Close()
		j.file = nil
		return err
	}
	return nil
}

func (j *VoteJournal) Write(code uint64, data interface{}, from string) error {
	if j.file == nil {
		j.OpenFile()
	}

	bs, err := rlp.EncodeToBytes(data)
	if err != nil {
		return err
	}
	entry := &rlpEncoder{
		Ts:   uint64(time.Now().UnixNano()),
		Code: code,
		From: from,
		Data: bs,
	}

	if err := rlp.Encode(j.file, entry); err != nil {
		return err
	}

	return nil
}

func (j *VoteJournal) Read(load func(ts, code uint64, data interface{}, from string) error) error {
	j.OpenFile()
	defer j.CloseFile()

	stream := rlp.NewStream(j.file, 0)
	total, dropped := 0, 0

	var failure error
	for {
		var entry rlpEncoder
		if err := stream.Decode(&entry); err != nil {
			if err == rlp.EOL {
				break
			} else if err == io.EOF {
				break
			}
			failure = err

			break
		}

		total++

		if err := load(entry.Ts, entry.Code, entry.Data, entry.From); err != nil {
			log.Debug("Failed to add journal", "err", err)
			dropped++
			continue
		}
	}
	log.Info("Loaded local recover journal", "success", total, "dropped", dropped)

	return failure
}
