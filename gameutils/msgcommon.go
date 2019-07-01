package gameutils

import (
	"fmt"
	"math/big"

	"github.com/kaleidochain/kaleido/accounts/keystore"
	"github.com/kaleidochain/kaleido/common"
	"github.com/kaleidochain/kaleido/crypto/secp256k1"
	"github.com/ethereum/go-ethereum/rlp"
)

// for message wrap
const (
	// Protocol messages belonging to poker1
	StatusMsgCode MsgCodeType = 0x00

	TransferMsgCode MsgCodeType = 0x01

	AckMsgCode MsgCodeType = 0x02

	DiffMsgCode MsgCodeType = 0x03 //提交分歧给公证者

	NotifySubmitMsgCode MsgCodeType = 0x04 //通知另一方提交公证

	//游戏开始前座位与状态消息
	SitDownMsgCode          MsgCodeType = 0x10
	SitDownResultMsgCode    MsgCodeType = 0x11
	OnSitDownMsgCode        MsgCodeType = 0x12
	LeaveMsgCode            MsgCodeType = 0x13
	ChangeSeatMsgCode       MsgCodeType = 0x14
	ChangeSeatResultMsgCode MsgCodeType = 0x15

	//现长度最大为 var ProtocolLengths = []uint64{0x20}，超过后相应修改ProtocolLengths里的值
)

//for gamengine
const (
	GameMsgWapDataCode MsgCodeType = 0x20 //游戏层消息包装

	EcPointDataCode MsgCodeType = 0x21 //牌点生成消息

	SumPointSignDataCode MsgCodeType = 0x22 //牌点和签名消息

	PublicKeyDataCode MsgCodeType = 0x23 //生成公钥消息

	SumpublicKeySignDataCode MsgCodeType = 0x24 //公钥和签名消息

	ShuffleDataCode MsgCodeType = 0x25 //洗牌消息

	FinalshuffleSignDataCode MsgCodeType = 0x26 //最终洗牌结果签名消息

	KeyCardDataCode MsgCodeType = 0x27 // 密钥消息

)

// statusData is the network packet for the status message.
type StatusData struct {
	ProtocolVersion uint32
	NetworkId       uint64
}

//SitData 成员是要公开的，即首字母大写，否则会失败
type SitData struct {
	GameAddress common.Address
	Desk        DeskID
	Seat        SeatID
	ID          UserID
	Address     common.Address
}

type SitReusltData struct {
	SeatList []*SitData
}

type LeaveData struct {
	Seat SeatID
}

type ChangeSeatData struct {
	GameAddress common.Address
	Desk        DeskID

	ID      UserID
	Address common.Address

	OldSeat SeatID
	NewSeat SeatID
}

//ecPointData 牌点消息
type EcPointData struct {
	//Points []*EcPoint
	PHash [][]byte
	PSign [][]byte
	ID    SeatID
}

type SumpointSignData struct {
	PointHash []byte
	Sign      []byte
	DeskID    []byte
	ID        SeatID
}

//sumpublicKeyDataRsp 公钥求和生成共识
type SumpublicKeySignData struct {
	SkeyHash []byte
	Sign     []byte
	DeskID   []byte
	ID       SeatID
}

type FinalShuffSignData struct {
	SfPointHash []byte
	Sign        []byte
	DeskID      []byte
	ID          SeatID
}

//shuffleDataRsp 回复消息
type ShuffleDataRsp struct {
	ID SeatID
}

//dealCardData 给自己发明牌或者发公共明牌时才发消息
type DealCardData struct {
	Cursor []uint
	SeatTo SeatID
	Open   bool
	DeskID string
	Signs  string
	ID     SeatID
}

//callCardData 收到callCardData更新牌点游标
type CallCardData struct {
	Num      uint
	CallSeat SeatID
	DeskID   string
	Signs    string
	ID       SeatID
}

type NotifyNotaryData struct {
	DeskID []byte
	ID     SeatID
}

//publicKeyData 公钥消息
type PublicKeyData struct {
	Key    *PublicKey
	E      *big.Int
	Z      *big.Int
	DeskID []byte
	ID     SeatID
}

//shuffleData 洗牌消息
type ShuffleData struct {
	Points []*PointPair
	E      *big.Int
	Z      *big.Int
	DeskID []byte
	ID     SeatID
}

//keyCardData 收到dealCardData 找出对应牌点的密钥，发回去。
type KeyCardData struct {
	Cursor []uint
	K      []*PublicKey
	E      []*big.Int
	Z      []*big.Int
	OP     uint
	SeatTo SeatID
	DeskID []byte
	ID     SeatID
}

//betDat 下注
type BetData struct {
	Bet     uint64
	BetSeat SeatID
	DeskID  []byte
	ID      SeatID
}

//CheckOutKeyData 弃牌玩家发出的密钥
type CheckOutKeyData struct {
	Cursor []uint
	K      []*PublicKey
	E      []*big.Int
	Z      []*big.Int
}

type CheckOutOriginalData struct {
	DeskID []byte
	ID     SeatID
	CsData CheckOutSignData
}

type CheckOutSignData struct {
	ContractAddr common.Address
	TableID      uint64
	Hand         uint32
}

type CheckOutData struct {
	CokData CheckOutKeyData
	CooData CheckOutOriginalData
	Sign    []byte //对CooData 的签名
}

type MsgData struct {
	Data []byte
}

func (s *MsgData) Decode(data interface{}) error {
	return rlp.DecodeBytes(s.Data, data)
}

type BetSignData struct {
	BetStateHash string
	Sign         string
	DeskID       string
	ID           SeatID
}

type SettleSignData struct {
	SeHash string
	Sign   string
	DeskID string
	ID     SeatID
}

type GameBetData struct {
	ID       SeatID
	Balance  uint64
	Turnbet  uint64
	Totalbet uint64
	Fold     bool
	AllIn    bool
}

//用于下注共识签名和公证,与其它阶段的区分开,下注前的阶段的只需要一个数据，后面都需要多个
type DeskCommonData struct { //桌上的公共数据，用于公证下注和结算中的发牌
	Points []*PointPair //洗完后的牌点点对
	//不能用map，map无法用rlp编码，
	PointSumMapCard []EcPoint //同OriginalPoint，只是没转换成点对，不需要转换成点对
	PublicCards     []Card    //桌子上公共的牌，从消息列表中无法获取的
	BetData         []GameBetData
	BetData2        []byte
	CurTurn         SeatID

	FoldSeats []SeatID //FoldSeats跟FoldMsg相当于map
	FoldMsg   []MsgWrap
}

type NotaireSign struct {
	ID   SeatID
	Sign []byte
}

type NotairePK struct {
	ID SeatID
	Pk *PublicKey
}

type Witness struct {
	CheckState uint //上一次达成共识的状态 这里是Po_check
	Data       []byte
	SignData   []byte         //签名的数据hash
	Sign       []*NotaireSign //各个玩家的签名

	PublicKey []*NotairePK //各个玩家的公钥，没有达成公钥共识，为空。
	SumPkey   *PublicKey   //公钥和,没有达成公钥共识时，为空

	OriginalPoint []*PointPair //初始牌点,没有洗过的牌点对
	ShufflePoint  []*PointPair //洗牌后的牌点
}

type NotaireContext struct {
	GameAddress common.Address //标识游戏，公证者可根据此项下载lua代码，还有房间abi等
	Version     uint8
	Wt          *Witness //
	From        SeatID   //谁提交上来的
	Hand        uint32
	//MyData []byte
	//OpData []byte

	MsgList  []MsgWrap //上一次共识到现在出现异常时收集的消息列表
	DeskID   []byte    //牌点求和哈希值
	TableCtr uint64    //合约中table id

	Sign SignData //签名
}

type Judgment struct { //返回给合约执行的
	//From     common.Address //公证者地址,合约中保存了
	GameOver bool           //是否结束游戏
	DeskID   []byte         //牌点求和哈希值
	TableCtr uint64         //合约中table id
	Result   map[SeatID]int //相关人的奖惩，正数为奖，负数为罚，执行结果由合约通知相关人

	MakeUp map[int]*MsgWrap //补齐的消息，包括消息应该被放的位置

	//证据也要传，合约上执行可以不用到，但可以保存数据
	//NContext *NotaireContext
}

// Hash returns the hash to be sned by the sender.
// It does not uniquely identify the transaction.
func (nc *NotaireContext) Hash() common.Hash {
	return rlpHash([]interface{}{
		nc.GameAddress,
		nc.Version,
		nc.Wt,
		nc.From,
		nc.Hand,

		nc.MsgList,
		nc.DeskID,
		nc.TableCtr,
	})
}

func (nc *NotaireContext) PublicKey() ([]byte, error) {
	return PublicKeyImpl(nc.Sign, nc.Hash())
}

func (nc *NotaireContext) Signature(ks *keystore.KeyStore, address common.Address) error {
	msgHash := nc.Hash()

	msgSign, err := signatureImpl(ks, address, msgHash)
	if err != nil {
		fmt.Println("SignHash fail ", err)
		return err
	}

	nc.Sign.R = new(big.Int).SetBytes(msgSign[:32])
	nc.Sign.S = new(big.Int).SetBytes(msgSign[32:64])
	nc.Sign.V = new(big.Int).SetBytes([]byte{msgSign[64] + 27})
	return nil
}

func (nc *NotaireContext) VerifySignature() bool {
	msgHash := nc.Hash()
	//fmt.Println("msgHash = ", msgHash)

	pubkey, err := PublicKeyImpl(nc.Sign, msgHash)
	if err != nil {
		fmt.Println(err)
		return false
	}
	//fmt.Println("pubkey = ", pubkey)

	cpy := cpySign(nc.Sign)

	return secp256k1.VerifySignature(pubkey, msgHash[:], cpy)
}
