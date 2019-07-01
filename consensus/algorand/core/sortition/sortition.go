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

// #cgo CFLAGS: -O3
// #include "sortition.h"
// #include "stdlib.h"
import "C"
import (
	"unsafe"

	"github.com/kaleidochain/kaleido/crypto/ed25519"
)

func Choose(hash ed25519.VrfOutput256, ownedWeight, expectedThreshold, expectedWeight, totalWeight uint64) (j uint64) {
	chash := unsafe.Pointer(&hash[0])
	if expectedThreshold == 1 {
		r := C.SortitionIsZeroByStandardSearch(chash, C.ulonglong(ownedWeight), C.ulonglong(expectedWeight), C.ulonglong(totalWeight))
		if C.int(r) == 0 {
			j = 1
		}
	} else {
		r := C.SortitionByBinarySearch(chash, C.ulonglong(ownedWeight), C.ulonglong(expectedWeight), C.ulonglong(totalWeight))
		j = uint64(r)
	}
	return
}
