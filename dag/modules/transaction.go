/*
   This file is part of go-palletone.
   go-palletone is free software: you can redistribute it and/or modify
   it under the terms of the GNU General Public License as published by
   the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.
   go-palletone is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU General Public License for more details.
   You should have received a copy of the GNU General Public License
   along with go-palletone.  If not, see <http://www.gnu.org/licenses/>.
*/
/*
 * @author PalletOne core developers <dev@pallet.one>
 * @date 2018
 */

package modules

import (
	"fmt"
	"math/big"
	"strconv"
	"time"

	//	"github.com/palletone/go-palletone/common/crypto"
	//	"github.com/palletone/go-palletone/common/crypto/sha3"
	//	"github.com/palletone/go-palletone/common/hexutil"
	//  "github.com/Re-volution/sizestruct"
	"github.com/palletone/go-palletone/common"
	"github.com/palletone/go-palletone/common/rlp"
)

var (
	TXFEE      = big.NewInt(5000000) // transaction fee =5ptn
	TX_MAXSIZE = uint32(256 * 1024)
)

// TxOut defines a bitcoin transaction output.
type TxOut struct {
	Value    int64
	PkScript []byte
    Asset    Asset
}
// TxIn defines a bitcoin transaction input.
type TxIn struct {
	PreviousOutPoint OutPoint
	SignatureScript  []byte
	Sequence         uint32
}

func NewTransaction(msg []Message, lock uint32) *Transaction {
	return newTransaction(msg, lock)
}

func NewContractCreation(msg []Message, lock uint32) *Transaction {
	return newTransaction(msg, lock)
}

func newTransaction(msg []Message, lock uint32) *Transaction {
	tx := new(Transaction)
	tx.TxMessages = msg[:]
	tx.Locktime = lock

	return tx
}
// AddTxIn adds a transaction input to the message.
func (pld *PaymentPayload) AddTxIn(ti Input) {
	pld.Inputs = append(pld.Inputs, ti)
}
// AddTxOut adds a transaction output to the message.
func (pld *PaymentPayload) AddTxOut(to Output) {
	pld.Outputs = append(pld.Outputs, to)
}

func (t *Transaction) SetHash(hash common.Hash) {
	if t.TxHash == (common.Hash{}) {
		t.TxHash = hash
	} else {
		t.TxHash.Set(hash)
	}
}

type TxPoolTransaction struct {
	Transaction

	CreationDate string  `json:"creation_date" rlp:"-"`
	Priority_lvl float64 `json:"priority_lvl" rlp:"-"` // 打包的优先级
	Nonce        uint64  // transaction'hash maybe repeat.
	Pending      bool
	Confirmed    bool
	Extra        []byte
}

//// ChainId returns which chain id this transaction was signed for (if at all)
//func (tx Transaction) ChainId() *big.Int {
//	return deriveChainId(tx.data.V)
//}
//
//// Protected returns whether the transaction is protected from replay protection.
//func (tx Transaction) Protected() bool {
//	return isProtectedV(tx.data.V)
//}
//
//func isProtectedV(V *big.Int) bool {
//	if V.BitLen() <= 8 {
//		v := V.Uint64()
//		return v != 27 && v != 28
//	}
//	// anything not 27 or 28 are considered unprotected
//	return true
//}
//
//// EncodeRLP implements rlp.Encoder
//func (tx *Transaction) EncodeRLP(w io.Writer) error {
//	return rlp.Encode(w, &tx.data)
//}
//
//// DecodeRLP implements rlp.Decoder
//func (tx *Transaction) DecodeRLP(s *rlp.Stream) error {
//	_, UnitSize, _ := s.Kind()
//	err := s.Decode(&tx.data)
//	if err == nil {
//		tx.UnitSize.Store(common.StorageSize(rlp.ListSize(UnitSize)))
//	}
//
//	return err
//}
//
//// MarshalJSON encodes the web3 RPC transaction format.
//func (tx *Transaction) MarshalJSON() ([]byte, error) {
//	UnitHash := tx.Hash()
//	data := tx.data
//	data.Hash = &UnitHash
//	return data.MarshalJSON()
//}
//
//// UnmarshalJSON decodes the web3 RPC transaction format.
//func (tx *Transaction) UnmarshalJSON(input []byte) error {
//	var dec txdata
//	if err := dec.UnmarshalJSON(input); err != nil {
//		return err
//	}
//	var V byte
//	if isProtectedV(dec.V) {
//		chainID := deriveChainId(dec.V).Uint64()
//		V = byte(dec.V.Uint64() - 35 - 2*chainID)
//	} else {
//		V = byte(dec.V.Uint64() - 27)
//	}
//	if !crypto.ValidateSignatureValues(V, dec.R, dec.S, false) {
//		return errors.New("invalid transaction v, r, s values")
//	}
//	*tx = Transaction{data: dec}
//	return nil
//}
//
//func (tx Transaction) Data() []byte { return common.CopyBytes(tx.data.Payload) }
//

func (tx *TxPoolTransaction) GetPriorityLvl() float64 {
	// priority_lvl=  fee/size*(1+(time.Now-CreationDate)/24)
	var priority_lvl float64
	if txfee := tx.Fee(); txfee.Int64() > 0 {
		t0, _ := time.Parse(TimeFormatString, tx.CreationDate)
		priority_lvl, _ = strconv.ParseFloat(fmt.Sprintf("%f", float64(txfee.Int64())/tx.Size().Float64()*(1+float64(time.Now().Hour()-t0.Hour())/24)), 64)
	}
	return priority_lvl
}
func (tx *TxPoolTransaction) SetPriorityLvl(priority float64) {
	tx.Priority_lvl = priority
}

// Hash hashes the RLP encoding of tx.
// It uniquely identifies the transaction.
func (tx Transaction) Hash() common.Hash {
	withoutSigTx := Transaction{}
	withoutSigTx.CopyFrTransaction(&tx)
	withoutSigTx.TxHash = common.Hash{}
	v := rlp.RlpHash(withoutSigTx)
	return v
}

// Size returns the true RLP encoded storage UnitSize of the transaction, either by
// encoding and returning it, or returning a previsouly cached value.
func (tx *Transaction) Size() common.StorageSize {
	c := writeCounter(0)
	rlp.Encode(&c, &tx)
	return common.StorageSize(c)
}

func (tx *Transaction) CreateDate() string {
	n := time.Now()
	return n.Format(TimeFormatString)
}

func (tx *Transaction) Fee() *big.Int {
	return TXFEE
}

func (tx *Transaction) Address() common.Address {
	return common.Address{}
}

// Cost returns amount + price
func (tx *Transaction) Cost() *big.Int {
	//if tx.TxFee.Cmp(TXFEE) < 0 {
	//	tx.TxFee = TXFEE
	//}
	//return tx.TxFee
	return TXFEE
}

func (tx *Transaction) CopyFrTransaction(cpy *Transaction) {
	tx.TxHash.Set(cpy.TxHash)
	tx.Locktime = cpy.Locktime
	tx.TxMessages = make([]Message, len(cpy.TxMessages))
	for _, msg := range cpy.TxMessages {
		newMsg := Message{}
		newMsg = *newMsg.CopyMessages(&msg)
		tx.TxMessages = append(tx.TxMessages, newMsg)
	}
}

//// AsMessage returns the transaction as a core.Message.
////
//// AsMessage requires a signer to derive the sender.
////
//// XXX Rename message to something less arbitrary?
//func (tx *Transaction) AsMessage(s Signer) (Message, error) {
//	msg := Message{
//		from:       *tx.data.From,
//		gasPrice:   new(big.Int).Set(tx.data.Price),
//		to:         tx.data.Recipient,
//		amount:     tx.data.Amount,
//		data:       tx.data.Payload,
//		checkNonce: true,
//	}
//
//	var err error
//	msg.from, err = Sender(s, tx)
//	return msg, err
//}
//

// Len returns the length of s.
func (s Transactions) Len() int { return len(s) }

// Swap swaps the i'th and the j'th element in s.
func (s Transactions) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// GetRlp implements Rlpable and returns the i'th element of s in rlp.
func (s Transactions) GetRlp(i int) []byte {
	enc, _ := rlp.EncodeToBytes(s[i])
	return enc
}
func (s Transactions) Hash() common.Hash {
	v := rlp.RlpHash(s)
	return v
}

// TxDifference returns a new set t which is the difference between a to b.
func TxDifference(a, b Transactions) (keep Transactions) {
	keep = make(Transactions, 0, len(a))

	remove := make(map[common.Hash]struct{})
	for _, tx := range b {
		remove[tx.Hash()] = struct{}{}
	}

	for _, tx := range a {
		if _, ok := remove[tx.Hash()]; !ok {
			keep = append(keep, tx)
		}
	}

	return keep
}

// single account, otherwise a nonce comparison doesn't make much sense.
type TxByNonce TxPoolTxs

func (s TxByNonce) Len() int           { return len(s) }
func (s TxByNonce) Less(i, j int) bool { return s[i].Nonce < s[j].Nonce }
func (s TxByNonce) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// TxByPrice implements both the sort and the heap interface, making it useful
// for all at once sorting as well as individually adding and removing elements.
type TxByPrice TxPoolTxs

func (s TxByPrice) Len() int      { return len(s) }
func (s TxByPrice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s *TxByPrice) Push(x interface{}) {
	*s = append(*s, x.(*TxPoolTransaction))
}

func (s *TxByPrice) Pop() interface{} {
	old := *s
	n := len(old)
	x := old[n-1]
	*s = old[0 : n-1]
	return x
}

type TxByPriority []*TxPoolTransaction

func (s TxByPriority) Len() int           { return len(s) }
func (s TxByPriority) Less(i, j int) bool { return s[i].Priority_lvl > s[j].Priority_lvl }
func (s TxByPriority) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func (s *TxByPriority) Push(x interface{}) {
	*s = append(*s, x.(*TxPoolTransaction))
}

func (s *TxByPriority) Pop() interface{} {
	old := *s
	n := len(old)
	x := old[n-1]
	*s = old[0 : n-1]
	return x
}

//// Message is a fully derived transaction and implements core.Message
////
//// NOTE: In a future PR this will be removed.
//
//func NewMessage(from, to *common.Address, nonce uint64, amount *big.Int, gasLimit uint64, gasPrice *big.Int, data []byte, checkNonce bool) Message {
//	return Message{
//		from:       *from,
//		to:         to,
//		nonce:      nonce,
//		amount:     amount,
//		gasLimit:   gasLimit,
//		gasPrice:   gasPrice,
//		data:       data,
//		checkNonce: checkNonce,
//	}
//}
//
//func (m Message) From() *common.Address { return &m.from }
//func (m Message) To() *common.Address   { return m.to }
//func (m Message) GasPrice() *big.Int    { return m.gasPrice }
//func (m Message) Value() *big.Int       { return m.amount }
//func (m Message) Gas() uint64           { return m.gasLimit }
//func (m Message) Nonce() uint64         { return m.nonce }
//func (m Message) Data() []byte          { return m.data }
//func (m Message) CheckNonce() bool      { return m.checkNonce }
//
//// deriveChainId derives the chain id from the given v parameter
//func deriveChainId(v *big.Int) *big.Int {
//	if v.BitLen() <= 64 {
//		v := v.Uint64()
//		if v == 27 || v == 28 {
//			return new(big.Int)
//		}
//		return new(big.Int).SetUint64((v - 35) / 2)
//	}
//	v = new(big.Int).Sub(v, big.NewInt(35))
//	return v.Div(v, big.NewInt(2))
//}
//func rlpHash(x interface{}) (h common.Hash) {
//	hw := sha3.NewKeccak256()
//	rlp.Encode(hw, x)
//	hw.Sum(h[:0])
//	return h
//}
//
//// deriveSigner makes a *best* guess about which signer to use.
//func deriveSigner(V *big.Int) Signer {
//	if V.Sign() != 0 && isProtectedV(V) {
//		return NewEIP155Signer(deriveChainId(V))
//	} else {
//		return HomesteadSigner{}
//	}
//}
//
type writeCounter common.StorageSize

func (c *writeCounter) Write(b []byte) (int, error) {
	*c += writeCounter(len(b))
	return len(b), nil
}

var (
	EmptyRootHash = DeriveSha(Transactions{})
)

type TxLookupEntry struct {
	UnitHash  common.Hash
	UnitIndex uint64
	Index     uint64
}
