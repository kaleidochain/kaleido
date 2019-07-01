package ed25519

import (
	"crypto/rand"
	"testing"
)

func randBytes() []byte {
	s := make([]byte, 64)
	n, err := rand.Read(s)
	if err != nil {
		panic("read rand failed: " + err.Error())
	}
	return s[:n]
}

func TestSignVerifyEmptyMessage(t *testing.T) {
	pk, sk := GenerateKey()
	sig := Sign(&sk, []byte{})
	if !Verify(&pk, []byte{}, &sig) {
		t.Errorf("sig of an empty message failed to verify")
	}
}

func TestGeneratePrivateKeyFromSeed(t *testing.T) {
	var seed Seed
	_, err := rand.Read(seed[:])
	if err != nil {
		t.Errorf("read rand failed: %s", err.Error())
	}

	pkA, skA := GenerateKeyFromSeed(seed)
	for i := 0; i < 10; i++ {
		pkB, skB := GenerateKeyFromSeed(seed)

		if pkA != pkB {
			t.Errorf("different public key generated for the same seed")
		}
		if skA != skB {
			t.Errorf("different private key generated for the same seed")
		}

		pkC, err := PrivateKeyToPublicKey(&skA)
		if err != nil {
			t.Errorf("get public key from private key failed")
		}
		if pkC != pkA {
			t.Errorf("PrivateKeyToPublicKey does not match the public key")
		}

		seedB, err := PrivateKeyToSeed(&skA)
		if err != nil {
			t.Errorf("get seed from private key failed")
		}
		if seedB != seed {
			t.Errorf("PrivateKeyToSeed does not match the origin seed")
		}
	}
}

func signAndVerify(t *testing.T, pkA PublicKey, skA PrivateKey, _ PublicKey, skB PrivateKey) {
	s1 := randBytes()

	sig := Sign(&skA, s1)
	if !Verify(&pkA, s1, &sig) {
		t.Errorf("correct signature failed to verify")
	}

	s2 := randBytes()

	sig2 := Sign(&skA, s2)
	if Verify(&pkA, s1, &sig2) {
		t.Errorf("wrong message incorrectly verified")
	}

	sig3 := Sign(&skB, s1)
	if Verify(&pkA, s1, &sig3) {
		t.Errorf("wrong key incorrectly verified")
	}

	if Verify(&pkA, s2, &sig3) {
		t.Errorf("wrong message+key incorrectly verified")
	}
}

func TestSignAndVerify(t *testing.T) {
	pk1, sk1 := GenerateKey()
	pk2, sk2 := GenerateKey()
	signAndVerify(t, pk1, sk1, pk2, sk2)
}

func BenchmarkSign(b *testing.B) {
	_, sk := GenerateKey()
	s := randBytes()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = Sign(&sk, s)
	}
}

func BenchmarkVerify(b *testing.B) {
	pk, sk := GenerateKey()
	s := randBytes()
	sig := Sign(&sk, s)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = Verify(&pk, s, &sig)
	}
}

func BenchmarkSignAndVerify(b *testing.B) {
	pk, sk := GenerateKey()
	s := randBytes()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sig := Sign(&sk, s)
		_ = Verify(&pk, s, &sig)
	}
}
