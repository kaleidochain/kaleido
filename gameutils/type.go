package gameutils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"log"
	"math/big"

	"github.com/kaleidochain/kaleido/crypto"
	"github.com/kaleidochain/kaleido/crypto/secp256k1"
)

type UserID = string

type DeskID = uint32

// SeatID int//http://colobu.com/2017/06/26/learn-go-type-aliases/ 了解 Go 1.9 的类型别名
type SeatID = uint8 //int8无法序列化

type Card = uint32

type MsgCodeType = uint64

type KeyType = ecdsa.PrivateKey

type PublicKey struct {
	X *big.Int
	Y *big.Int
}

type EcPoint struct {
	//X, Y *big.Int
	PointBytes [65]byte //secp256k1.S256().Marshal(point.X, point.Y)
}

type PointPair struct {
	C1 *PublicKey
	C2 *PublicKey
}

type LogicData struct {
	SrcSeat SeatID
	Code    MsgCodeType
	Data    []byte
}

type DealOP struct {
	SeatTo   SeatID
	Currsors []uint
	Open     uint
}

type DealOPBackup struct {
	Currsor uint
	DOps    []DealOP
}

//HashToInt hash转为一个曲线上的整数
func HashToInt(hash []byte, c elliptic.Curve) *big.Int {
	orderBits := c.Params().N.BitLen()
	orderBytes := (orderBits + 7) / 8
	if len(hash) > orderBytes {
		hash = hash[:orderBytes]
	}

	ret := new(big.Int).SetBytes(hash)
	excess := len(hash)*8 - orderBits
	if excess > 0 {
		ret.Rsh(ret, uint(excess))
	}
	return ret
}

func PublicKeyVerify(pd *PublicKeyData) (result int) {

	result = -1

	zgx, zgy := secp256k1.S256().ScalarBaseMult(pd.Z.Bytes())

	ehx, ehy := secp256k1.S256().ScalarMult(pd.Key.X, pd.Key.Y, pd.E.Bytes())

	if zgx == nil || zgy == nil || ehx == nil || ehy == nil {

		return
	}

	rx, ry := secp256k1.S256().Add(zgx, zgy, ehx, ehy)

	rhash := crypto.Keccak256(secp256k1.S256().Gx.Bytes(), secp256k1.S256().Gy.Bytes(), rx.Bytes(), ry.Bytes(), pd.Key.X.Bytes(), pd.Key.Y.Bytes())
	r := HashToInt(rhash, secp256k1.S256())

	result = r.Cmp(pd.E)

	return
}

func PointVerify(pd *EcPointData) (result int, ep []*EcPoint) {
	result = -1

	if len(pd.PHash) != len(pd.PSign) {
		log.Println("len not equal")
		return
	}
	ep = make([]*EcPoint, len(pd.PHash))

	for i, v := range pd.PHash {
		point, err := crypto.Ecrecover(v, pd.PSign[i])
		if err != nil {
			log.Println("Recover publickey: ", err.Error())
			return
		}

		R := new(big.Int).SetBytes(pd.PSign[i][:32])
		S := new(big.Int).SetBytes(pd.PSign[i][32:64])
		//V := new(big.Int).SetBytes([]byte{pd.PSign[i][64] + 27})
		cpy := make([]byte, 64)
		rb, sb := R.Bytes(), S.Bytes()
		copy(cpy[32-len(rb):32], rb)
		copy(cpy[64-len(sb):64], sb)

		if secp256k1.VerifySignature(point, v, cpy) == false {
			log.Println("Verifysignature error")
			return
		}

		ep[i] = new(EcPoint)
		for k := 0; k < 65; k++ {
			ep[i].PointBytes[k] = point[k]
		}

	}

	result = 0

	return
}
