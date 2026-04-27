package substratebase

import (
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

type SubstrateBlock struct {
	BlockHash types.Hash
	Header    types.Header
	Block     types.SignedBlock
}

type SubstrateAccountInfo struct {
	Nonce       types.U32
	Consumers   types.U32
	Providers   types.U32
	Sufficients types.U32
	Data        types.AccountInfo
}

type SubstrateTransaction struct {
	TxHash      string
	BlockHash   string
	BlockNumber uint64
	From        string
	To          string
	Amount      string
	Fee         string
	Status      uint32
	TxType      uint32
}

type ExtrinsicPayload struct {
	Era                types.ExtrinsicEra
	Nonce              uint64
	Tip                uint64
	SpecVersion        uint32
	GenesisHash        types.Hash
	BlockHash          types.Hash
	TransactionVersion uint32
}

const (
	TxTypeTransfer   uint32 = 0
	TxTypeStaking    uint32 = 1
	TxTypeGovernance uint32 = 2
	TxTypeOther      uint32 = 3

	PolkadotSS58Prefix uint16 = 0
	KusamaSS58Prefix   uint16 = 2
)
