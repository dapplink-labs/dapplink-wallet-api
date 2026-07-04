package bnbchain

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"

	"github.com/dapplink-labs/dapplink-wallet-api/chain/evmbase"
)

var (
	transferEventTopic = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	handleOpsSelector  = crypto.Keccak256([]byte("handleOps((address,uint256,bytes,bytes,bytes32,uint256,bytes32,bytes,bytes)[],address)"))[:4]
)

type userOpERC20Transfer struct {
	Contract string
	From     string
	To       string
	Amount   string
	Index    uint
}

func (c *ChainAdaptor) tryParseUserOpERC20Transfer(blockItem evmbase.TransactionList) (userOpERC20Transfer, bool) {
	transfers := c.tryParseUserOpBEP20Transfers(blockItem)
	if len(transfers) == 0 {
		return userOpERC20Transfer{}, false
	}
	return transfers[0], true
}

func (c *ChainAdaptor) tryParseUserOpBEP20Transfers(blockItem evmbase.TransactionList) []userOpERC20Transfer {
	if c.entryPointAddress == (common.Address{}) {
		return nil
	}
	if normalizeAddress(blockItem.To) != normalizeAddress(c.entryPointAddress.Hex()) {
		return nil
	}
	input := strings.TrimPrefix(blockItem.Input, "0x")
	if len(input) < 8 || !strings.EqualFold(input[:8], common.Bytes2Hex(handleOpsSelector)) {
		return nil
	}

	receipt, err := c.ethClient.TxReceiptByHash(common.HexToHash(blockItem.Hash))
	if err != nil || receipt == nil {
		log.Warn("fetch receipt for UserOp tx failed", "hash", blockItem.Hash, "err", err)
		return nil
	}

	return c.parseUserOpBEP20TransfersFromReceipt(receipt.Logs)
}

func (c *ChainAdaptor) parseUserOpERC20TransferFromReceipt(logs []*types.Log) (userOpERC20Transfer, bool) {
	transfers := c.parseUserOpBEP20TransfersFromReceipt(logs)
	if len(transfers) == 0 {
		return userOpERC20Transfer{}, false
	}
	return transfers[0], true
}

func (c *ChainAdaptor) parseUserOpBEP20TransfersFromReceipt(logs []*types.Log) []userOpERC20Transfer {
	if len(c.contractAddrIndex) == 0 {
		return nil
	}

	transfers := make([]userOpERC20Transfer, 0)
	for _, lg := range logs {
		if lg == nil || len(lg.Topics) != 3 || lg.Topics[0] != transferEventTopic {
			continue
		}
		token := normalizeAddress(lg.Address.Hex())
		if _, tracked := c.contractAddrIndex[token]; !tracked {
			continue
		}
		from := common.HexToAddress(lg.Topics[1].Hex()).Hex()
		to := common.HexToAddress(lg.Topics[2].Hex()).Hex()
		value := new(big.Int).SetBytes(lg.Data)
		if value.Sign() <= 0 {
			continue
		}
		transfers = append(transfers, userOpERC20Transfer{
			Contract: lg.Address.Hex(),
			From:     from,
			To:       to,
			Amount:   value.String(),
			Index:    lg.Index,
		})
	}
	return transfers
}
