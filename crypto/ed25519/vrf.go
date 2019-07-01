package ed25519

// #cgo CFLAGS: -Wall -std=c99
// #cgo CFLAGS: -I${SRCDIR}/include/
// #cgo LDFLAGS: ${SRCDIR}/lib/libsodium.a
// #include <stdint.h>
// #include "sodium.h"
import "C"
import "crypto/sha512"

func init() {
	if C.sodium_init() < 0 {
		panic("sodium_init() failed")
	}
}

const (
	// VrfProofSize is the size, in bytes, of VRF proofs
	VrfProofSize = 80

	// VrfOutputSize is the size, in bytes, of VRF outputs
	VrfOutputSize = 64

	// VrfOutputSize256 is the size, in bytes, of VRF outputs in 32 bytes
	VrfOutputSize256 = 32
)

// A VrfPrivateKey is a private key used for producing VRF proofs.
// Specifically, we use a 64-byte ed25519 private key.
type VrfPrivateKey PrivateKey

// A VrfPublicKey is a public key that can be used to verify VRF proofs.
type VrfPublicKey PublicKey

// A VrfProof for a message can be generated with a secret key and verified against a public key, like a signature.
// Proofs are malleable, however, for a given message and public key, the VRF output that can be computed from a proof is unique.
type VrfProof [VrfProofSize]byte

// VrfOutput is a 64-byte pseudorandom value that can be computed from a VrfProof.
// The VRF scheme guarantees that such output will be unique
type VrfOutput [VrfOutputSize]byte

// VrfOutput256 is a 32-byte pseudorandom value that can be computed from a VrfProof.
// The VRF scheme guarantees that such output will be unique
type VrfOutput256 [VrfOutputSize256]byte

// VrfGenerateKey generates a new rand VRF key pair.
func VrfGenerateKey() (publicKey VrfPublicKey, privateKey VrfPrivateKey) {
	C.crypto_vrf_keypair((*C.uchar)(&publicKey[0]), (*C.uchar)(&privateKey[0]))
	return
}

// VrfGenerateKeyFromSeed generates a new key pair from seed.
func VrfGenerateKeyFromSeed(seed Seed) (publicKey VrfPublicKey, privateKey VrfPrivateKey) {
	C.crypto_vrf_keypair_from_seed((*C.uchar)(&publicKey[0]), (*C.uchar)(&privateKey[0]), (*C.uchar)(&seed[0]))
	return
}

func VrfPrivateKeyToPublicKey(privateKey *VrfPrivateKey) (publicKey VrfPublicKey) {
	C.crypto_vrf_sk_to_pk((*C.uchar)(&publicKey[0]), (*C.uchar)(&privateKey[0]))
	return
}

// VrfProve generates a proof for a given message.
// ok will be false if the private key is bad.
func VrfProve(privateKey *VrfPrivateKey, msg []byte) (proof VrfProof, ok bool) {
	m := (*C.uchar)(C.NULL)
	if len(msg) > 0 {
		m = (*C.uchar)(&msg[0])
	}
	ret := C.crypto_vrf_prove((*C.uchar)(&proof[0]), (*C.uchar)(&privateKey[0]), m, (C.ulonglong)(len(msg)))
	return proof, ret == 0
}

// VrfProofToHash generates a hash from a proof without verifying the proof.
func VrfProofToHash(proof *VrfProof) (hash VrfOutput, ok bool) {
	ret := C.crypto_vrf_proof_to_hash((*C.uchar)(&hash[0]), (*C.uchar)(&proof[0]))
	return hash, ret == 0
}

// VrfProofToHash256 generates a hash256 from a proof without verifying the proof.
func VrfProofToHash256(proof *VrfProof) (hash256 VrfOutput256, ok bool) {
	var hash VrfOutput
	ret := C.crypto_vrf_proof_to_hash((*C.uchar)(&hash[0]), (*C.uchar)(&proof[0]))
	if ret == 0 {
		hash256 = sha512.Sum512_256(hash[:])
		ok = true
	}
	return
}

// VrfVerify verifies a VRF proof against publicKey and msg, return true and the VrfOutput if the proof is valid.
// ok will be false if the proof is invalid.
//
// For a given public key and message, there are potentially multiple valid proofs.
// However, given a public key and message, all valid proofs will yield the same output.
// Moreover, the output is indistinguishable from random to anyone without the proof or the secret key.
func VrfVerify(publicKey *VrfPublicKey, proof *VrfProof, msg []byte) (ok bool, output VrfOutput) {
	m := (*C.uchar)(C.NULL)
	if len(msg) > 0 {
		m = (*C.uchar)(&msg[0])
	}
	ret := C.crypto_vrf_verify((*C.uchar)(&output[0]), (*C.uchar)(&publicKey[0]), (*C.uchar)(&proof[0]), m, (C.ulonglong)(len(msg)))
	return ret == 0, output
}

func VrfVerify256(publicKey *VrfPublicKey, proof *VrfProof, msg []byte) (ok bool, hash256 VrfOutput256) {
	var hash VrfOutput
	ok, hash = VrfVerify(publicKey, proof, msg)
	if ok {
		hash256 = sha512.Sum512_256(hash[:])
	}
	return
}
