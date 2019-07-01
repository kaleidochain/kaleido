package ed25519

import (
	"math/rand"
	"testing"
)

func TestForwardSecureSignAndVerify(t *testing.T) {
	start := uint64(0)
	max := uint64(10000)
	pk1, sk1 := GenerateForwardSecureKey(start, max)
	_, sk2 := GenerateForwardSecureKey(start, max)

	offset := uint64(rand.Intn(int(max)))

	data1 := randBytes()
	data2 := randBytes()

	sig1 := ForwardSecureSign(&sk1, offset, data1)
	if !ForwardSecureVerify(&pk1, offset, data1, sig1) {
		t.Errorf("verify failed on correct signature")
	}

	if ForwardSecureVerify(&pk1, offset, data2, sig1) {
		t.Errorf("verify passed on wrong data")
	}

	sig2 := ForwardSecureSign(&sk2, offset, data1)
	if ForwardSecureVerify(&pk1, offset, data1, sig2) {
		t.Errorf("verify passed on wrong signature")
	}

	other := uint64(rand.Intn(int(max)))
	for other == offset {
		other = uint64(rand.Intn(int(max)))
	}
	if ForwardSecureVerify(&pk1, other, data1, sig1) {
		t.Errorf("verify passed on wrong offset")
	}

	next := offset + 1
	deleted := ForwardSecureUpdate(&sk1, next)
	if deleted != next-start {
		t.Errorf("the deleted count is incorrect, expected=%d, actual=%d", next-start, deleted)
	}

	sigAfterUpdate := ForwardSecureSign(&sk1, offset, data1)
	if ForwardSecureVerify(&pk1, offset, data1, sigAfterUpdate) {
		t.Errorf("verify passed on invalid signature which generated after update")
	}

	sig3 := ForwardSecureSign(&sk1, next, data1)
	if !ForwardSecureVerify(&pk1, next, data1, sig3) {
		t.Errorf("verify failed on correct signature after update")
	}
}
