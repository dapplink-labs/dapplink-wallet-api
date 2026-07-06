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

const (
	transferKindOuter        = "outer"
	transferKindReceiptERC20 = "receipt_erc20"
)

var (
	transferEventTopic = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	handleOpsSelector  = crypto.Keccak256([]byte("handleOps((address,uint256,bytes,bytes,bytes32,uint256,bytes32,bytes,bytes)[],address)"))[:4]
)

type erc20TransferLog struct {
	Contract string
	From     string
	To       string
	Amount   string
	LogIndex uint32
}

type userOpERC20Transfer struct {
	Contract string
	From     string
	To       string
	Amount   string
}

func (c *ChainAdaptor) tryParseUserOpERC20Transfer(blockItem evmbase.TransactionList) (userOpERC20Transfer, bool) {
	if c.entryPointAddress == (common.Address{}) {
		return userOpERC20Transfer{}, false
	}
	if normalizeAddress(blockItem.To) != normalizeAddress(c.entryPointAddress.Hex()) {
		return userOpERC20Transfer{}, false
	}
	input := strings.TrimPrefix(blockItem.Input, "0x")
	if len(input) < 8 || !strings.EqualFold(input[:8], common.Bytes2Hex(handleOpsSelector)) {
		return userOpERC20Transfer{}, false
	}

	receipt, err := c.ethClient.TxReceiptByHash(common.HexToHash(blockItem.Hash))
	if err != nil || receipt == nil {
		log.Warn("fetch receipt for UserOp tx failed", "hash", blockItem.Hash, "err", err)
		return userOpERC20Transfer{}, false
	}

	transfer, ok := c.firstERC20TransferFromReceipt(receipt.Logs)
	if !ok {
		return userOpERC20Transfer{}, false
	}
	return userOpERC20Transfer{
		Contract: transfer.Contract,
		From:     transfer.From,
		To:       transfer.To,
		Amount:   transfer.Amount,
	}, true
}

func (c *ChainAdaptor) parseERC20TransfersFromReceipt(logs []*types.Log) []erc20TransferLog {
	out := make([]erc20TransferLog, 0, len(logs))
	for _, lg := range logs {
		if lg == nil || len(lg.Topics) != 3 || lg.Topics[0] != transferEventTopic {
			continue
		}
		token := normalizeAddress(lg.Address.Hex())
		if len(c.contractAddrIndex) > 0 {
			if _, tracked := c.contractAddrIndex[token]; !tracked {
				continue
			}
		}
		from := common.HexToAddress(lg.Topics[1].Hex()).Hex()
		to := common.HexToAddress(lg.Topics[2].Hex()).Hex()
		value := new(big.Int).SetBytes(lg.Data)
		if value.Sign() <= 0 {
			continue
		}
		out = append(out, erc20TransferLog{
			Contract: lg.Address.Hex(),
			From:     from,
			To:       to,
			Amount:   value.String(),
			LogIndex: uint32(lg.Index),
		})
	}
	return out
}

func (c *ChainAdaptor) firstERC20TransferFromReceipt(logs []*types.Log) (erc20TransferLog, bool) {
	transfers := c.parseERC20TransfersFromReceipt(logs)
	if len(transfers) == 0 {
		return erc20TransferLog{}, false
	}
	return transfers[0], true
}

func transferDedupKey(txHash, contractAddress, fromAddress, toAddress, amount string) string {
	return strings.ToLower(strings.TrimSpace(txHash)) + "|" +
		normalizeAddress(contractAddress) + "|" +
		normalizeAddress(fromAddress) + "|" +
		normalizeAddress(toAddress) + "|" +
		strings.TrimSpace(amount)
}

func receiptFeeString(receipt *types.Receipt) string {
	if receipt == nil {
		return "0"
	}
	fee := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), receipt.EffectiveGasPrice)
	return fee.String()
}

func receiptStatusValue(receipt *types.Receipt) uint32 {
	if receipt == nil || receipt.Status != types.ReceiptStatusSuccessful {
		return 0
	}
	return 1
}
