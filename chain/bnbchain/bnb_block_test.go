package bnbchain

import (
	"errors"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/dapplink-labs/dapplink-wallet-api/chain/evmbase"
	"github.com/dapplink-labs/dapplink-wallet-api/config"
)

func TestBuildBlockTransactionsKeepsOriginalHashForDirectBEP20Transfer(t *testing.T) {
	token := common.HexToAddress("0x55d398326f99059fF775485246999027B3197955")
	from := common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	to := common.HexToAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	txHash := "0x1111111111111111111111111111111111111111111111111111111111111111"

	c := ChainAdaptor{}
	txs := c.buildBlockTransactions(evmbase.TransactionList{
		From:     from.Hex(),
		To:       token.Hex(),
		Hash:     txHash,
		Value:    "0",
		Input:    erc20TransferInput(to, big.NewInt(123)),
		GasPrice: "7",
	}, "0xblock", 12)

	if len(txs) != 1 {
		t.Fatalf("len(txs) = %d, want 1", len(txs))
	}
	if txs[0].TxHash != txHash {
		t.Fatalf("TxHash = %q, want original hash %q", txs[0].TxHash, txHash)
	}
	if txs[0].ContractAddress != token.Hex() {
		t.Fatalf("ContractAddress = %q, want %q", txs[0].ContractAddress, token.Hex())
	}
	if len(txs[0].To) != 1 || txs[0].To[0].Address != to.Hex() || txs[0].To[0].Amount != "123" {
		t.Fatalf("to transfer mismatch: %#v", txs[0].To)
	}
}

func TestBuildBlockTransactionsDoesNotFallbackToExternalWhenAAParsingFails(t *testing.T) {
	entryPoint := common.HexToAddress("0x0000000000000000000000000000000000004337")
	c := ChainAdaptor{
		entryPointAddress: entryPoint,
		ethClient: &countingEthClient{
			receiptErr: errors.New("receipt unavailable"),
			traceErr:   errors.New("trace unavailable"),
		},
	}

	txs := c.buildBlockTransactions(evmbase.TransactionList{
		From:     "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		To:       entryPoint.Hex(),
		Hash:     "0x2222222222222222222222222222222222222222222222222222222222222222",
		Value:    "1000000000000000000",
		Input:    handleOpsInput(),
		GasPrice: "7",
	}, "0xblock", 12)

	if len(txs) != 0 {
		t.Fatalf("len(txs) = %d, want 0 when AA receipt and trace parsing fail: %#v", len(txs), txs)
	}
}

func TestBuildBlockTransactionsParsesAATokenLogWithLegacyHashSuffix(t *testing.T) {
	usdt := common.HexToAddress("0x55d398326f99059fF775485246999027B3197955")
	entryPoint := common.HexToAddress("0x0000000000000000000000000000000000004337")
	sender := common.HexToAddress("0x1111111111111111111111111111111111111111")
	user := common.HexToAddress("0x2222222222222222222222222222222222222222")
	txHash := "0x9999999999999999999999999999999999999999999999999999999999999999"

	c := ChainAdaptor{
		entryPointAddress: entryPoint,
		ethClient: &countingEthClient{
			receipt: &types.Receipt{
				Status: types.ReceiptStatusSuccessful,
				Logs:   []*types.Log{transferLog(usdt, sender, user, big.NewInt(1000), 7)},
			},
		},
		contractAddrIndex: newContractAddrIndex([]string{usdt.Hex()}),
	}

	txs := c.buildBlockTransactions(evmbase.TransactionList{
		From:     "0x4444444444444444444444444444444444444444",
		To:       entryPoint.Hex(),
		Hash:     txHash,
		Value:    "0",
		Input:    handleOpsInput(),
		GasPrice: "7",
	}, "0xblock", 100)

	if len(txs) != 1 {
		t.Fatalf("len(txs) = %d, want 1: %#v", len(txs), txs)
	}
	if txs[0].TxHash != txHash+":token_log:7" {
		t.Fatalf("TxHash = %q", txs[0].TxHash)
	}
}

func TestBuildBlockTransactionsParsesAllowedRouterReceiptTransfer(t *testing.T) {
	usdt := common.HexToAddress("0x55d398326f99059fF775485246999027B3197955")
	fakeToken := common.HexToAddress("0x9999999999999999999999999999999999999999")
	router := common.HexToAddress("0x1111111111111111111111111111111111111111")
	user := common.HexToAddress("0x2222222222222222222222222222222222222222")
	pair := common.HexToAddress("0x3333333333333333333333333333333333333333")
	txHash := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	c := ChainAdaptor{
		ethClient: &countingEthClient{
			receipt: &types.Receipt{
				Status: types.ReceiptStatusSuccessful,
				Logs: []*types.Log{
					transferLog(fakeToken, pair, user, big.NewInt(999), 3),
					transferLog(usdt, pair, user, big.NewInt(1000), 7),
				},
			},
		},
		contractAddrIndex: newContractAddrIndex([]string{usdt.Hex()}),
		depositPolicyIndex: newDepositPolicyIndex(config.DepositPolicyConfig{
			Enabled: true,
			RouterReceipt: config.ReceiptSourcePolicy{
				Enabled: true,
				AllowedSources: []config.NamedAddressPolicy{{
					Name:    "PancakeSwap Router",
					Address: router.Hex(),
					Action:  "accept",
				}},
			},
		}),
	}

	txs := c.buildBlockTransactions(evmbase.TransactionList{
		From:     "0x4444444444444444444444444444444444444444",
		To:       router.Hex(),
		Hash:     txHash,
		Value:    "0",
		Input:    "0x12345678",
		GasPrice: "7",
	}, "0xblock", 100)

	if len(txs) != 1 {
		t.Fatalf("len(txs) = %d, want 1: %#v", len(txs), txs)
	}
	if txs[0].TxHash != txHash+":router_token_log:7" {
		t.Fatalf("TxHash = %q", txs[0].TxHash)
	}
	if txs[0].ContractAddress != usdt.Hex() {
		t.Fatalf("ContractAddress = %q", txs[0].ContractAddress)
	}
	if len(txs[0].To) != 1 || txs[0].To[0].Address != user.Hex() || txs[0].To[0].Amount != "1000" {
		t.Fatalf("to transfer mismatch: %#v", txs[0].To)
	}
}

func TestBuildBlockTransactionsSkipsUnknownRouterReceiptTransfer(t *testing.T) {
	usdt := common.HexToAddress("0x55d398326f99059fF775485246999027B3197955")
	router := common.HexToAddress("0x1111111111111111111111111111111111111111")
	user := common.HexToAddress("0x2222222222222222222222222222222222222222")
	pair := common.HexToAddress("0x3333333333333333333333333333333333333333")

	c := ChainAdaptor{
		ethClient: &countingEthClient{
			receipt: &types.Receipt{
				Status: types.ReceiptStatusSuccessful,
				Logs:   []*types.Log{transferLog(usdt, pair, user, big.NewInt(1000), 7)},
			},
		},
		contractAddrIndex: newContractAddrIndex([]string{usdt.Hex()}),
	}

	txs := c.buildBlockTransactions(evmbase.TransactionList{
		From:     "0x4444444444444444444444444444444444444444",
		To:       router.Hex(),
		Hash:     "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Value:    "0",
		Input:    "0x12345678",
		GasPrice: "7",
	}, "0xblock", 100)

	if len(txs) != 1 {
		t.Fatalf("len(txs) = %d, want external fallback transaction", len(txs))
	}
	if txs[0].TxHash != "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("TxHash = %q", txs[0].TxHash)
	}
	if len(txs[0].To) != 1 || txs[0].To[0].Address != router.Hex() {
		t.Fatalf("unknown router should not parse receipt transfer into user deposit: %#v", txs[0].To)
	}
}

func TestBuildBlockTransactionsSkipsFailedRouterReceiptTransfer(t *testing.T) {
	usdt := common.HexToAddress("0x55d398326f99059fF775485246999027B3197955")
	router := common.HexToAddress("0x1111111111111111111111111111111111111111")
	user := common.HexToAddress("0x2222222222222222222222222222222222222222")
	pair := common.HexToAddress("0x3333333333333333333333333333333333333333")

	c := ChainAdaptor{
		ethClient: &countingEthClient{
			receipt: &types.Receipt{
				Status: types.ReceiptStatusFailed,
				Logs:   []*types.Log{transferLog(usdt, pair, user, big.NewInt(1000), 7)},
			},
		},
		contractAddrIndex: newContractAddrIndex([]string{usdt.Hex()}),
		depositPolicyIndex: newDepositPolicyIndex(config.DepositPolicyConfig{
			Enabled: true,
			RouterReceipt: config.ReceiptSourcePolicy{
				Enabled: true,
				AllowedSources: []config.NamedAddressPolicy{{
					Name:    "PancakeSwap Router",
					Address: router.Hex(),
					Action:  "accept",
				}},
			},
		}),
	}

	txs := c.buildBlockTransactions(evmbase.TransactionList{
		From:     "0x4444444444444444444444444444444444444444",
		To:       router.Hex(),
		Hash:     "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		Value:    "0",
		Input:    "0x12345678",
		GasPrice: "7",
	}, "0xblock", 100)

	if len(txs) != 0 {
		t.Fatalf("len(txs) = %d, want 0 for failed whitelisted router receipt: %#v", len(txs), txs)
	}
}

func TestBuildBlockWithTransactionsLimitsRPCFanout(t *testing.T) {
	entryPoint := common.HexToAddress("0x0000000000000000000000000000000000004337")
	client := &countingEthClient{
		receiptDelay: 10 * time.Millisecond,
		traceErr:     errors.New("trace unavailable"),
	}
	c := ChainAdaptor{
		entryPointAddress: entryPoint,
		ethClient:         client,
	}

	const txCount = 40
	rpcBlock := &evmbase.RpcBlock{
		Hash:       common.HexToHash("0x3333333333333333333333333333333333333333333333333333333333333333"),
		ParentHash: common.HexToHash("0x4444444444444444444444444444444444444444444444444444444444444444"),
		Number:     "0x1",
		Timestamp:  "0x1",
	}
	for i := 0; i < txCount; i++ {
		rpcBlock.Transactions = append(rpcBlock.Transactions, evmbase.TransactionList{
			From:     "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			To:       entryPoint.Hex(),
			Hash:     common.BigToHash(big.NewInt(int64(i + 1))).Hex(),
			Value:    "0",
			Input:    handleOpsInput(),
			GasPrice: "7",
		})
	}

	if _, err := c.buildBlockWithTransactions(rpcBlock); err != nil {
		t.Fatalf("buildBlockWithTransactions() error = %v", err)
	}
	const maxExpectedBlockTransactionWorkers = 16
	if client.maxReceiptConcurrency > maxExpectedBlockTransactionWorkers {
		t.Fatalf("max receipt concurrency = %d, want <= %d", client.maxReceiptConcurrency, maxExpectedBlockTransactionWorkers)
	}
}

func erc20TransferInput(to common.Address, amount *big.Int) string {
	return "0x" + erc20TransferMethod +
		common.Bytes2Hex(common.LeftPadBytes(to.Bytes(), 32)) +
		common.Bytes2Hex(common.LeftPadBytes(amount.Bytes(), 32))
}

func handleOpsInput() string {
	return "0x" + common.Bytes2Hex(handleOpsSelector)
}

type countingEthClient struct {
	evmbase.EthClient

	receiptDelay time.Duration
	receiptErr   error
	traceErr     error
	receipt      *types.Receipt

	mu                    sync.Mutex
	receiptConcurrency    int
	maxReceiptConcurrency int
}

func (c *countingEthClient) TxReceiptByHash(common.Hash) (*types.Receipt, error) {
	c.mu.Lock()
	c.receiptConcurrency++
	if c.receiptConcurrency > c.maxReceiptConcurrency {
		c.maxReceiptConcurrency = c.receiptConcurrency
	}
	c.mu.Unlock()

	if c.receiptDelay > 0 {
		time.Sleep(c.receiptDelay)
	}

	c.mu.Lock()
	c.receiptConcurrency--
	c.mu.Unlock()

	if c.receiptErr != nil {
		return nil, c.receiptErr
	}
	if c.receipt != nil {
		return c.receipt, nil
	}
	return &types.Receipt{}, nil
}

func (c *countingEthClient) TraceTransaction(common.Hash) (*evmbase.TraceCallFrame, error) {
	if c.traceErr != nil {
		return nil, c.traceErr
	}
	return &evmbase.TraceCallFrame{}, nil
}
