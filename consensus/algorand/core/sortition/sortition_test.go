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

package sortition

import (
	"crypto/rand"
	"testing"

	"github.com/kaleidochain/kaleido/crypto/ed25519"
)

func TestSortition(t *testing.T) {
	hitcount := uint64(0)
	const nTimes = 10000

	const threshold = 2
	const totalNumber = 20
	const ownWeight = 100
	const totalWeight = 200
	for i := 0; i < nTimes; i++ {
		var vrfOutput ed25519.VrfOutput256
		_, _ = rand.Read(vrfOutput[:])
		selected := Choose(vrfOutput, ownWeight, threshold, totalNumber, totalWeight)
		hitcount += selected
	}

	expected := uint64(nTimes * totalNumber * ownWeight / totalWeight)
	var diff uint64
	if expected > hitcount {
		diff = expected - hitcount
	} else {
		diff = hitcount - expected
	}

	// within 2% good enough
	maxDiff := expected / 50
	if diff > maxDiff {
		t.Errorf("wanted %d weight but got %d, diff=%d, maxDiff=%d", expected, hitcount, diff, maxDiff)
	}
}

func BenchmarkSortition(b *testing.B) {
	b.StopTimer()
	keys := make([]ed25519.VrfOutput256, b.N)
	for i := 0; i < b.N; i++ {
		_, _ = rand.Read(keys[i][:])
	}
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		Choose(keys[i], 1000000, 2, 2500, 1000000000000)
	}
}

func TestStampingSortition(t *testing.T) {
	hitcount := uint64(0)
	const nTimes = 10000

	const threshold = 100
	const totalNumber = 120
	const ownWeight = 1600
	const totalWeight = 2000
	for i := 0; i < nTimes; i++ {
		var vrfOutput ed25519.VrfOutput256
		_, _ = rand.Read(vrfOutput[:])
		selected := Choose(vrfOutput, ownWeight, threshold, totalNumber, totalWeight)
		if selected >= threshold {
			hitcount += 1
		}
	}

	const expected = 35.0
	fs := float64(hitcount) / nTimes * 100
	if fs <= expected-1 || fs >= expected+1 {
		t.Errorf("wanted %f%% weight but got %f%%, hit=%d", expected, fs, hitcount)
	}
}
