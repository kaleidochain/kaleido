package ed25519

import "encoding/binary"

const (
	ForwardSecureSignatureSize = SignatureSize + PublicKeySize + SignatureSize
)

type ForwardSecurePublicKey PublicKey

type signedSubKey struct {
	Sk    PrivateKey
	PkSig Signature
}

type ForwardSecurePrivateKey struct {
	PublicKey ForwardSecurePublicKey
	Start     uint64
	SubKeys   []signedSubKey
}

func (key ForwardSecurePrivateKey) ValidOffset(offset uint64) bool {
	limit := key.Start + uint64(len(key.SubKeys))
	return offset >= key.Start && offset < limit
}

type ForwardSecureSignature struct {
	Sig   Signature
	Pk    PublicKey
	PkSig Signature
}

func GenerateForwardSecureKey(offset, count uint64) (pk ForwardSecurePublicKey, sk ForwardSecurePrivateKey) {
	rawPk, skToBeDestroyed := GenerateKey()

	subs := make([]signedSubKey, count)
	for i := uint64(0); i < count; i++ {
		subPk, subSk := GenerateKey()

		data := toBytes(offset+i, subPk)
		subPkSig := Sign(&skToBeDestroyed, data)

		subs[i] = signedSubKey{
			Sk:    subSk,
			PkSig: subPkSig,
		}
	}

	pk = ForwardSecurePublicKey(rawPk)
	sk = ForwardSecurePrivateKey{
		PublicKey: pk,
		Start:     offset,
		SubKeys:   subs,
	}
	return
}

func toBytes(offset uint64, subPk PublicKey) []byte {
	var buffer [8 + PublicKeySize]byte
	binary.BigEndian.PutUint64(buffer[0:8], offset)
	copy(buffer[8:PublicKeySize], subPk[:])
	return buffer[:]
}

func ForwardSecurePrivateKeyToPublicKey(sk *ForwardSecurePrivateKey) ForwardSecurePublicKey {
	return sk.PublicKey
}

func ForwardSecureSign(sk *ForwardSecurePrivateKey, offset uint64, data []byte) ForwardSecureSignature {
	if !sk.ValidOffset(offset) {
		return ForwardSecureSignature{}
	}

	idx := offset - sk.Start
	subSk := &sk.SubKeys[idx].Sk
	subPk, _ := PrivateKeyToPublicKey(subSk)

	sig := Sign(subSk, data)

	return ForwardSecureSignature{
		Sig:   sig,
		Pk:    subPk,
		PkSig: sk.SubKeys[idx].PkSig,
	}
}

func ForwardSecureVerify(pk *ForwardSecurePublicKey, offset uint64, data []byte, sig ForwardSecureSignature) bool {
	verifyData := toBytes(offset, sig.Pk)
	masterPk := (*PublicKey)(pk)

	return Verify(masterPk, verifyData, &sig.PkSig) && Verify(&sig.Pk, data, &sig.Sig)
}

func ForwardSecureUpdate(sk *ForwardSecurePrivateKey, offset uint64) (deleted uint64) {
	if offset > sk.Start {
		deleted = offset - sk.Start
		if max := uint64(len(sk.SubKeys)); deleted > max {
			deleted = max
		}

		sk.Start += deleted
		sk.SubKeys = sk.SubKeys[deleted:]
	}

	return
}
