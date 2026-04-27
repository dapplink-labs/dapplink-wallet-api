package substratebase

import "github.com/centrifuge/go-substrate-rpc-client/v4/types"

type SubstrateTransferTx struct {
	FromAddress string `json:"from_address"`
	ToAddress   string `json:"to_address"`
	Amount      string `json:"amount"`
	Tip         uint64 `json:"tip"`
}

type SubstrateUnSignTx struct {
	Mode        string `json:"mode"`
	FromAddress string `json:"from_address"`
	ToAddress   string `json:"to_address"`
	Amount      string `json:"amount"`
	Tip         uint64 `json:"tip"`
	Nonce       uint64 `json:"nonce"`
	Era         string `json:"era"`
	BlockHash   string `json:"block_hash"`
	BlockNumber uint64 `json:"block_number"`
}

type SubstrateSignedTx struct {
	Mode        string `json:"mode"`
	FromAddress string `json:"from_address"`
	ToAddress   string `json:"to_address"`
	Amount      string `json:"amount"`
	Tip         uint64 `json:"tip"`
	Nonce       uint64 `json:"nonce"`
	Signature   string `json:"signature"`
	PublicKey   string `json:"public_key"`
	Era         string `json:"era"`
	BlockHash   string `json:"block_hash"`
	BlockNumber uint64 `json:"block_number"`
}

func IsSubstrateTx(base64Tx string) bool {
	return true
}

func ParseEra(eraStr string) types.ExtrinsicEra {
	if eraStr == "immortal" || eraStr == "" {
		return types.ExtrinsicEra{IsImmortalEra: true}
	}
	return types.ExtrinsicEra{IsImmortalEra: false}
}
