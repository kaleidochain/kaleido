package gameutils

import (
	"errors"
	"fmt"
	"math/big"

	"golang.org/x/crypto/sha3"

	"github.com/kaleidochain/kaleido/accounts"
	"github.com/kaleidochain/kaleido/accounts/keystore"
	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/crypto"
	"github.com/kaleidochain/kaleido/crypto/secp256k1"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	ErrInvalidSig = errors.New("invalid transaction v, r, s values")
	errNoSigner   = errors.New("missing signing methods")
)

type MsgWrap struct {
	GameAddress common.Address
	Deskid      DeskID //桌子ID
	Hand        uint64 //局数

	DestSeats []SeatID //期望接收消息的位置
	SrcSeat   SeatID   //发送消息的位置
	Handler   SeatID   //需要处理的位置

	MsgCode MsgCodeType //消息类型  send code = 自定义的消息类型  ack code =2
	SeqNum  uint32      //消息序号

	//Data interface{}
	Data []byte //消息内容

	Sign      SignData //签名
	InterSign SignData //inter签名
}

type MsgAck struct {
	Deskid   DeskID
	DestSeat SeatID
	SrcSeat  SeatID

	MsgCode MsgCodeType //消息类型  code = 自定义的消息类型
	SeqNum  uint32
}

type SignData struct {

	// Signature values
	V *big.Int `json:"v" gencodec:"required"`
	R *big.Int `json:"r" gencodec:"required"`
	S *big.Int `json:"s" gencodec:"required"`

	// This is only used when marshaling to JSON.
	//Hash *common.Hash `json:"hash" rlp:"-"`
}

// Hash returns the hash to be sned by the sender.
// It does not uniquely identify the transaction.
func (msg *MsgWrap) Hash() common.Hash {
	return rlpHash([]interface{}{
		msg.GameAddress,
		msg.Deskid,
		msg.Hand,

		msg.DestSeats,
		msg.SrcSeat,
		msg.Handler,

		msg.MsgCode,
		msg.SeqNum,

		msg.Data,
	})
}

func (msg *MsgWrap) PublicKey() ([]byte, error) {
	return PublicKeyImpl(msg.Sign, msg.Hash())
}

func PublicKeyImpl(Sign SignData, hash common.Hash) ([]byte, error) {
	if Sign.V.BitLen() > 8 {
		return nil, ErrInvalidSig
	}
	V := byte(Sign.V.Uint64() - 27)
	if !crypto.ValidateSignatureValues(V, Sign.R, Sign.S, true) {
		return nil, ErrInvalidSig
	}
	// encode the snature in uncompressed format
	r, s := Sign.R.Bytes(), Sign.S.Bytes()
	sig := make([]byte, 65)
	copy(sig[32-len(r):32], r)
	copy(sig[64-len(s):64], s)
	sig[64] = V

	// recover the public key from the snature
	//hash := msg.Hash()
	pub, err := crypto.Ecrecover(hash[:], sig)
	if err != nil {
		return nil, err
	}
	if len(pub) == 0 || pub[0] != 4 {
		return nil, errors.New("invalid public key")
	}
	return pub, nil
}

func (msg *MsgWrap) Signature(ks *keystore.KeyStore, address common.Address) error {
	msgHash := msg.Hash()

	msgSign, err := signatureImpl(ks, address, msgHash)
	if err != nil {
		fmt.Println("SignHash fail ", err)
		return err
	}

	msg.Sign.R = new(big.Int).SetBytes(msgSign[:32])
	msg.Sign.S = new(big.Int).SetBytes(msgSign[32:64])
	msg.Sign.V = new(big.Int).SetBytes([]byte{msgSign[64] + 27})
	return nil
}

func (msg *MsgWrap) VerifySignature() bool {
	msgHash := msg.Hash()

	pubkey, err := PublicKeyImpl(msg.Sign, msg.Hash())
	if err != nil {
		fmt.Println(err)
		return false
	}

	cpy := cpySign(msg.Sign)

	return secp256k1.VerifySignature(pubkey, msgHash[:], cpy)
}

func (msg *MsgWrap) SignatureInter(ks *keystore.KeyStore, address common.Address) error {
	msgHash := msg.Hash()

	msgSign, err := signatureImpl(ks, address, msgHash)
	if err != nil {
		fmt.Println("SignHash fail ", err)
		return err
	}

	msg.InterSign.R = new(big.Int).SetBytes(msgSign[:32])
	msg.InterSign.S = new(big.Int).SetBytes(msgSign[32:64])
	msg.InterSign.V = new(big.Int).SetBytes([]byte{msgSign[64] + 27})
	return nil
}

func (msg *MsgWrap) VerifySignatureInter() bool {
	msgHash := msg.Hash()

	pubkey, err := PublicKeyImpl(msg.InterSign, msg.Hash())
	if err != nil {
		fmt.Println(err)
		return false
	}

	cpy := cpySign(msg.InterSign)

	return secp256k1.VerifySignature(pubkey, msgHash[:], cpy)
}

func signatureImpl(ks *keystore.KeyStore, address common.Address, msgHash common.Hash) ([]byte, error) {
	//t2 := time.Now()
	msgSign, err := ks.SignHash(accounts.Account{Address: address}, msgHash[:])
	//elapsed2 := time.Since(t2)
	//fmt.Println("SignHash elapsed: ", elapsed2)

	return msgSign, err
}

func rlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewLegacyKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}

func cpySign(sign SignData) []byte {
	cpy := make([]byte, 64)
	R, S := sign.R.Bytes(), sign.S.Bytes()
	copy(cpy[32-len(R):32], R)
	copy(cpy[64-len(S):64], S)
	return cpy
}
