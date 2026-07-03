package bnbchain

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"

	"github.com/dapplink-labs/dapplink-wallet-api/chain/evmbase"
)

type TraceCallFrame = evmbase.TraceCallFrame

type nativeTraceTransfer struct {
	From   string
	To     string
	Amount string
	Index  uint
}

const (
	transferEntryExternal    = "external"
	transferEntryTokenLog    = "token_log"
	transferEntryNativeTrace = "native_trace"
)

// transferUniqueHash keeps normal chain hashes unchanged, while giving each
// AA-derived internal transfer a stable idempotency key under the same outer tx.
func transferUniqueHash(txHash, entryType string, index uint) string {
	if entryType == transferEntryExternal && index == 0 {
		return txHash
	}
	return fmt.Sprintf("%s:%s:%d", txHash, entryType, index)
}

// canonicalTxHash strips the internal-transfer suffix before querying the chain.
func canonicalTxHash(hash string) string {
	parts := strings.Split(hash, ":")
	if len(parts) >= 3 {
		return parts[0]
	}
	return hash
}

func collectNativeTraceTransfers(frame TraceCallFrame) []nativeTraceTransfer {
	transfers := make([]nativeTraceTransfer, 0)
	var walk func(TraceCallFrame)
	walk = func(current TraceCallFrame) {
		// Failed internal calls must not create deposit records.
		if strings.TrimSpace(current.Error) != "" {
			return
		}
		if amount, ok := parseTraceValue(current.Value); ok &&
			amount.Sign() > 0 &&
			common.IsHexAddress(current.From) &&
			common.IsHexAddress(current.To) {
			transfers = append(transfers, nativeTraceTransfer{
				From:   common.HexToAddress(current.From).Hex(),
				To:     common.HexToAddress(current.To).Hex(),
				Amount: amount.String(),
				Index:  uint(len(transfers)),
			})
		}
		for _, child := range current.Calls {
			walk(child)
		}
	}
	// Start from child calls so the root EntryPoint call is not treated as a deposit.
	for _, child := range frame.Calls {
		walk(child)
	}
	return transfers
}

func parseTraceValue(raw string) (*big.Int, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, false
	}
	base := 10
	if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X") {
		value = value[2:]
		base = 16
	}
	amount, ok := new(big.Int).SetString(value, base)
	return amount, ok
}

func (c *ChainAdaptor) tryParseUserOpNativeTransfers(blockItem evmbase.TransactionList) []nativeTraceTransfer {
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

	// Native BNB movement inside AA handleOps is only visible through call traces.
	trace, err := c.ethClient.TraceTransaction(common.HexToHash(blockItem.Hash))
	if err != nil || trace == nil {
		log.Warn("fetch trace for UserOp native transfers failed", "hash", blockItem.Hash, "err", err)
		return nil
	}
	return collectNativeTraceTransfers(*trace)
}
