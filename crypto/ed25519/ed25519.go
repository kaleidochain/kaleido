package ed25519

// #cgo CFLAGS: -Wall -std=c99
// #cgo CFLAGS: -I${SRCDIR}/include
// #cgo LDFLAGS: ${SRCDIR}/lib/libsodium.a
// #include <stdint.h>
// #include "sodium.h"
import "C"
import "fmt"

func init() {
	if C.sodium_init() < 0 {
		panic("sodium_init() failed")
	}
}

const (
	// PublicKeySize is the size, in bytes, of public keys as used in this package.
	PublicKeySize = 32

	// PrivateKeySize is the size, in bytes, of private keys as used in this package.
	PrivateKeySize = 64

	// SignatureSize is the size, in bytes, of signatures generated and verified by this package.
	SignatureSize = 64

	// SeedSize is the size, in bytes, of private key seeds. These are the private key representations used by RFC 8032.
	SeedSize = 32
)

type Signature [SignatureSize]byte
type PublicKey [PublicKeySize]byte
type PrivateKey [PrivateKeySize]byte
type Seed [SeedSize]byte

// GenerateKey generates a public/private key pair using rand in libsodium.
func GenerateKey() (publicKey PublicKey, privateKey PrivateKey) {
	C.crypto_sign_ed25519_keypair((*C.uchar)(&publicKey[0]), (*C.uchar)(&privateKey[0]))
	return
}

// GenerateKeyFromSeed generates a public/private key pair using seed.
func GenerateKeyFromSeed(seed Seed) (publicKey PublicKey, privateKey PrivateKey) {
	C.crypto_sign_ed25519_seed_keypair((*C.uchar)(&publicKey[0]), (*C.uchar)(&privateKey[0]), (*C.uchar)(&seed[0]))
	return
}

// PrivateKeyToPublicKey derives a public key from a private key. This is very
// efficient since ed25519 private keys literally contain their public key
func PrivateKeyToPublicKey(privateKey *PrivateKey) (PublicKey, error) {
	var pk PublicKey
	result := C.crypto_sign_ed25519_sk_to_pk((*C.uchar)(&pk[0]), (*C.uchar)(&privateKey[0]))
	if result != 0 {
		return pk, fmt.Errorf("failed to extract public key: %d", int(result))
	}
	return pk, nil
}

// PrivateKeyToSeed derives the seed from a private key. This is very efficient
// since ed25519 private keys literally contain their seed
func PrivateKeyToSeed(privateKey *PrivateKey) (Seed, error) {
	var seed Seed
	result := C.crypto_sign_ed25519_sk_to_seed((*C.uchar)(&seed[0]), (*C.uchar)(&privateKey[0]))
	if result != 0 {
		return seed, fmt.Errorf("failed to extract seed: %d", int(result))
	}
	return seed, nil
}

// Sign signs the data with privateKey and returns a signature. It will
// panic if privateKey is nil
func Sign(privateKey *PrivateKey, data []byte) (sig Signature) {
	// data maybe zero length
	d := (*C.uchar)(C.NULL)
	if len(data) > 0 {
		d = (*C.uchar)(&data[0])
	}
	// https://download.libsodium.org/doc/public-key_cryptography/public-key_signatures#detached-mode
	C.crypto_sign_ed25519_detached((*C.uchar)(&sig[0]), (*C.ulonglong)(C.NULL), d, C.ulonglong(len(data)), (*C.uchar)(&privateKey[0]))
	return
}

// Verify reports whether sig is a valid signature of data by publicKey.
func Verify(publicKey *PublicKey, data []byte, sig *Signature) bool {
	// data maybe zero length
	d := (*C.uchar)(C.NULL)
	if len(data) > 0 {
		d = (*C.uchar)(&data[0])
	}
	// https://download.libsodium.org/doc/public-key_cryptography/public-key_signatures#detached-mode
	result := C.crypto_sign_ed25519_verify_detached((*C.uchar)(&sig[0]), d, C.ulonglong(len(data)), (*C.uchar)(&publicKey[0]))
	return result == 0
}
