// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

var KaleidoMainnetBootnodes = []string{
	"enode://8dff9fb2a061c753884c8a85ae6f88bd047b7ff13a8b3093eda1122624a3a2846a546d5d6cf4f2cb54f23307edcb6ab18fc8b58aabb7348376ffe3e8d0c9b4dc@13.52.47.128:38880",
	"enode://3df593f5b46e8ee4ef8d0cb7b6bb6ea8d9a1fc342a959952da76ec30bcd9285bdb93016d6f98869ccb82f3d27de9d8fbfd844a185960cf3be911617ef6f8ca8a@35.156.122.24:38880",
	"enode://85a4c0a235b1cca64fe3fc1c61e435140a90a49b08fb79482459004bc918fcc3922cf5b55499e46d2a6d7c5d9fef2c51f6fec34d9abdd34f456e503184fd1c93@3.9.54.38:38880",
	"enode://00801c4a847db0fe09d58b526a2c21bb7d3ae0c2172ff9b7f4028a18ca86f2d3b98dd685f70994f3e543fee8682c60da8dc689f5e65a79075bb0391e639c7450@207.148.69.198:38880",
	"enode://187f4f2de29823833948c2cf97b7f96da919b1d50d4f7ebb7b327d68d930600a50545e096c3bd874f8e785d38aad6c57114e0cc10f31cf795cb29fea938594c1@149.28.182.120:38880",
}
var KaleidoTestnetBootnodes = []string{
	"enode://ab0fb69e483605352f82a5c344685040e339701c3bfe9262692e81d432fb483deb6235ab82e01049af03e610434aef66f69b84cea2f37e79d4a68f5e16877af0@13.52.47.128:38882",
	"enode://d186d5bf4f663f1eb10606b2be3604eec7aea4a9d09992659074dd86ef6aa403321f7dabe50e4e2745c714b62530b0ea9f62cc98af43e4b1a4d2360302f7ad87@35.156.122.24:38882",
	"enode://5dd5d06832704c753d0c7f52cd831eb121c4c89e44742a97f3c353c8442402a0f7e8c2fb5d2607ef913476e8fc0e805650a50aae0883a6f70167fb1b5d79eb47@3.9.54.38:38882",
	"enode://a1794c8fbad5dba7b9c03e0d4d441a6c159c371a837cf5f1e4ecfccb4e5bc21552eaaf97176e21d9d2a52621290e14d826d1147bba3e117015f21f2683127bc7@207.148.69.198:38882",
	"enode://2524a02b4c309d7d7f465efef81f21e852e712b576185022bf42233833797602c33d88003fc2b249348c164c2b68ebecbb1d5950841be41deec7259329861946@149.28.182.120:38882",
}

var KaleidoMainnetBootnodesV5 = []string{
	"enode://1165e00c626eea130f042e19bc8818d7663d1603e2cc39734f53145304fc3d4e1ac5cbc81b94af8d74db1e31923783ac63d2a8542355d3ffb90e9fca95f5977d@13.52.47.128:38881",
	"enode://90085a14d0f9550ff0fb4d4facc68a9d58e1ad28259607ef36f1c04ca8f929e722d2c032a30406113bec2c327ec6fea39e4adc090f60a716d6a6b5c478fa3557@35.156.122.24:38881",
	"enode://5be35331ab98c3068a7ab319b835a278b814251e70c53e0b786b1b5ced1287afc2ffa4f27cbe5ed7844fd244d04a93f927778a58a88b7eb4231863505c772ad2@3.9.54.38:38881",
	"enode://4ff50c87fbbc917c72cb0275eb30aed3a2cf2ad0c3f78c5f2289ab48e3e974f33c99c355fd6616445ac039982fcf017f8c4c6632ba3f400358b55aa746a731be@207.148.69.198:38881",
	"enode://d59f875219003fd0dce09f0222b75888ca5810fab62f550eeb15429755e1d3823ad6f59d65a4fcbe4b6a9bb951ad2e39d744d3999dd667a312a85d2d66abcbdd@149.28.182.120:38881",
}

var KaleidoTestnetBootnodesV5 = []string{
	"enode://3c1c983d64e351668489dc55cc9fa5124c7f724394af23c297b7987223c3e830dab7a420c97c4a7086eb7e63290c8e2457a0e5c508f01ba536b7534ede862805@13.52.47.128:38883",
	"enode://0cb5e181214c50cc36d860aefd888b4aed764a32d6f86995aec33dee7ddb9a9566d8db7dee083f017ba73d68f8a8c6cd97b176dc6ba5d109c34941bdaabb146e@35.156.122.24:38883",
	"enode://2cb626d7a4e884f3c9721b2e5a8566781a53a95246b63098e28d1d7c23a5dbee51bed593a8cc847f19a2c8d5bcc59090e02e91eea2d02569d73da33453476221@3.9.54.38:38883",
	"enode://f2dc43e384a1496dc67521258291acc8262f7e422e584930ff0913cf1e955a0a5f20ad3c47bf4db99a2e761a5725f12466179a08f1d92459d3f165109730e1e5@207.148.69.198:38883",
	"enode://a1ee495fe65db45ae308dc4401a9ca3ec8b94e5358475c7d9215f4721484f8a24d1e20389f0d8951ceaedad4e34c0a1c5de1cfd08701906cce61efccf1bfb6c6@149.28.182.120:38883",
}

// Remove follows for ignoring connect to ethereum p2p network

// MainnetBootnodes are the enode URLs of the P2P bootstrap nodes running on
// the main Ethereum network.
var MainnetBootnodes = []string{}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Ropsten test network.
var TestnetBootnodes = []string{}

// RinkebyBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Rinkeby test network.
var RinkebyBootnodes = []string{}

// GoerliBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Görli test network.
var GoerliBootnodes = []string{
	// Upstrem bootnodes
	"enode://011f758e6552d105183b1761c5e2dea0111bc20fd5f6422bc7f91e0fabbec9a6595caf6239b37feb773dddd3f87240d99d859431891e4a642cf2a0a9e6cbb98a@51.141.78.53:30303",
	"enode://176b9417f511d05b6b2cf3e34b756cf0a7096b3094572a8f6ef4cdcb9d1f9d00683bf0f83347eebdf3b81c3521c2332086d9592802230bf528eaf606a1d9677b@13.93.54.137:30303",
	"enode://46add44b9f13965f7b9875ac6b85f016f341012d84f975377573800a863526f4da19ae2c620ec73d11591fa9510e992ecc03ad0751f53cc02f7c7ed6d55c7291@94.237.54.114:30313",
	"enode://c1f8b7c2ac4453271fa07d8e9ecf9a2e8285aa0bd0c07df0131f47153306b0736fd3db8924e7a9bf0bed6b1d8d4f87362a71b033dc7c64547728d953e43e59b2@52.64.155.147:30303",
	"enode://f4a9c6ee28586009fb5a96c8af13a58ed6d8315a9eee4772212c1d4d9cebe5a8b8a78ea4434f318726317d04a3f531a1ef0420cf9752605a562cfe858c46e263@213.186.16.82:30303",

	// Ethereum Foundation bootnode
	"enode://573b6607cd59f241e30e4c4943fd50e99e2b6f42f9bd5ca111659d309c06741247f4f1e93843ad3e8c8c18b6e2d94c161b7ef67479b3938780a97134b618b5ce@52.56.136.200:30303",
}

// DiscoveryV5Bootnodes are the enode URLs of the P2P bootstrap nodes for the
// experimental RLPx v5 topic-discovery network.
var DiscoveryV5Bootnodes = []string{}
