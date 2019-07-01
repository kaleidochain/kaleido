package ed25519

import (
	"encoding/hex"
	"testing"
)

func unhex(s string) []byte {
	data := make([]byte, hex.DecodedLen(len(s)))
	n, err := hex.Decode(data, []byte(s))
	if err != nil {
		panic("not hex string")
	}
	return data[:n]
}

func testVector(t *testing.T, seedHex, pkHex, alphaHex, piHex, betaHex string) {
	var seed [32]byte
	var pk VrfPublicKey
	var alpha []byte
	var pi VrfProof
	var beta VrfOutput

	alpha = make([]byte, hex.DecodedLen(len(alphaHex)))

	copy(seed[:], unhex(seedHex))
	copy(pk[:], unhex(pkHex))
	copy(pi[:], unhex(piHex))
	copy(beta[:], unhex(betaHex))
	copy(alpha, unhex(alphaHex))

	pkTest, sk := VrfGenerateKeyFromSeed(seed)
	if pkTest != pk {
		t.Errorf("Computed public key does from seed not match the test vector")
	}

	pkTest2 := VrfPrivateKeyToPublicKey(&sk)
	if pkTest2 != pk {
		t.Errorf("Computed public key from secret key does not match the test vector")
	}

	piTest, ok := VrfProve(&sk, alpha)
	if !ok {
		t.Errorf("Failed to produce a proof")
	}
	if piTest != pi {
		t.Errorf("Proof produced by Prove() does not match the test vector")
	}

	ok, betaTest := VrfVerify(&pk, &pi, alpha)
	if !ok {
		t.Errorf("Verify() fails on proof from the test vector")
	}
	if betaTest != beta {
		t.Errorf("VRF output does not match test vector")
	}

	betaTest2, ok := VrfProofToHash(&pi)
	if !ok {
		t.Errorf("VrfProofToHash() fails on proof from the test vector")
	}
	if betaTest2 != beta {
		t.Errorf("VrfProofToHash() does not match test vector")
	}
}

// ECVRF-ED25519-SHA512-Elligator2 test vectors from: https://www.ietf.org/id/draft-irtf-cfrg-vrf-03.txt appendix A.4
func TestVRFTestVectors(t *testing.T) {
	testVector(t,
		"9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60", //sk
		"d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a", //pk
		"", // alpha
		"b6b4699f87d56126c9117a7da55bd0085246f4c56dbc95d20172612e9d38e8d7ca65e573a126ed88d4e30a46f80a666854d675cf3ba81de0de043c3774f061560f55edc256a787afe701677c0f602900", // pi
		"5b49b554d05c0cd5a5325376b3387de59d924fd1e13ded44648ab33c21349a603f25b84ec5ed887995b33da5e3bfcb87cd2f64521c4c62cf825cffabbe5d31cc",                                 // beta
	)

	testVector(t,
		"4ccd089b28ff96da9db6c346ec114e0f5b8a319f35aba624da8cf6ed4fb8a6fb", //sk
		"3d4017c3e843895a92b70aa74d1b7ebc9c982ccf2ec4968cc0cd55f12af4660c", //pk
		"72", // alpha
		"ae5b66bdf04b4c010bfe32b2fc126ead2107b697634f6f7337b9bff8785ee111200095ece87dde4dbe87343f6df3b107d91798c8a7eb1245d3bb9c5aafb093358c13e6ae1111a55717e895fd15f99f07", // pi
		"94f4487e1b2fec954309ef1289ecb2e15043a2461ecc7b2ae7d4470607ef82eb1cfa97d84991fe4a7bfdfd715606bc27e2967a6c557cfb5875879b671740b7d8",                                 // beta
	)

	testVector(t,
		"c5aa8df43f9f837bedb7442f31dcb7b166d38535076f094b85ce3a2e0b4458f7", //sk
		"fc51cd8e6218a1a38da47ed00230f0580816ed13ba3303ac5deb911548908025", //pk
		"af82", // alpha
		"dfa2cba34b611cc8c833a6ea83b8eb1bb5e2ef2dd1b0c481bc42ff36ae7847f6ab52b976cfd5def172fa412defde270c8b8bdfbaae1c7ece17d9833b1bcf31064fff78ef493f820055b561ece45e1009", // pi
		"2031837f582cd17a9af9e0c7ef5a6540e3453ed894b62c293686ca3c1e319dde9d0aa489a4b59a9594fc2328bc3deff3c8a0929a369a72b1180a596e016b5ded",                                 // beta
	)
}

func BenchmarkVrfVerify(b *testing.B) {
	pks := make([]VrfPublicKey, b.N)
	ss := make([][]byte, b.N)
	proofs := make([]VrfProof, b.N)

	for i := 0; i < b.N; i++ {
		var sk VrfPrivateKey
		pks[i], sk = VrfGenerateKey()
		ss[i] = randBytes()
		var ok bool
		proofs[i], ok = VrfProve(&sk, ss[i])
		if !ok {
			panic("Failed to construct VRF proof")
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = VrfVerify(&pks[i], &proofs[i], ss[i])
	}
}
