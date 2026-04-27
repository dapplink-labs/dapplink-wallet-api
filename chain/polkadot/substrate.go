package polkadot

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/centrifuge/go-substrate-rpc-client/v4/registry"
	registryparser "github.com/centrifuge/go-substrate-rpc-client/v4/registry/parser"
	"github.com/centrifuge/go-substrate-rpc-client/v4/scale"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/centrifuge/go-substrate-rpc-client/v4/xxhash"
	"github.com/dapplink-labs/dapplink-wallet-api/config"
	"github.com/ethereum/go-ethereum/log"

	"github.com/dapplink-labs/dapplink-wallet-api/chain"
	"github.com/dapplink-labs/dapplink-wallet-api/chain/substratebase"
	wallet_api "github.com/dapplink-labs/dapplink-wallet-api/protobuf/wallet-api"
)

type SubstrateChainAdaptor struct {
	substrateClient substratebase.SubstrateClient
	ss58Prefix      uint8
	metadata        *types.Metadata
	runtimeVersion  types.RuntimeVersion
	callRegistry    registry.CallRegistry
	callRegistryRV  types.RuntimeVersion
	eventRegistry   registry.EventRegistry
	eventRegistryRV types.RuntimeVersion
}

const substrateTxHashScanWorkers = 50
const substrateTxHashScanLogInterval = 200
const substrateTxAddressScanWorkers = 50
const substrateTxAddressScanBatchSize = 50
const substrateTxAddressDefaultPageSize = 20
const substrateTxAddressMaxPageSize = 100
const substrateAssetBalanceLookupWorkers = 32

type extrinsicTransferEvent struct {
	from   string
	to     string
	amount string
}

// extrinsicEventSummary 聚合某一笔 extrinsic 对应的链上事件。
type extrinsicEventSummary struct {
	status    *wallet_api.TxStatus
	fee       string
	transfers []extrinsicTransferEvent
	actions   []string
}

type parsedBlockEvents struct {
	summaries         map[uint32]*extrinsicEventSummary
	blockTransactions []*wallet_api.TransactionList
}

type locatedExtrinsic struct {
	blockHash   types.Hash
	signedBlock *types.SignedBlock
	extIndex    int
}

type addressBlockTransactions struct {
	height       uint64
	transactions []*wallet_api.TransactionList
}

type addressMatchedTransaction struct {
	extIndex    uint32
	transaction *wallet_api.TransactionList
}

type substrateAssetBalance struct {
	AssetID  uint32
	Symbol   string
	Balance  string
	Decimals uint32
}

type substrateAssetMetadata struct {
	Deposit  types.U128
	Name     []byte
	Symbol   []byte
	Decimals types.U8
	IsFrozen bool
}

type substrateAssetBalanceLookupResult struct {
	balance substrateAssetBalance
	err     error
}

type callTransactionInfo struct {
	txType     uint32
	fromAmount string
	to         []*wallet_api.ToAddress
	actions    []string
}

type transactionCallSummaryMeta struct {
	CallSummary    []string `json:"call_summary,omitempty"`
	SummarySources []string `json:"summary_sources,omitempty"`
}

func NewSubstrateChainAdaptor(chainId string) func(conf *config.Config) (chain.IChainAdaptor, error) {
	return func(conf *config.Config) (chain.IChainAdaptor, error) {
		node := getNodeConfig(chainId, conf)
		if node.SubstrateRpcUrl == "" {
			return nil, fmt.Errorf("substrate rpc url not configured")
		}
		substrateCli, err := substratebase.NewSubstrateClient(node.SubstrateRpcUrl)
		if err != nil {
			log.Error("Dial substrate client fail", "err", err)
			return nil, err
		}
		return &SubstrateChainAdaptor{
			substrateClient: substrateCli,
			ss58Prefix:      getSS58Prefix(chainId),
		}, nil
	}
}

// ConvertAddresses 转换公钥为地址
func (c SubstrateChainAdaptor) ConvertAddresses(ctx context.Context, req *wallet_api.ConvertAddressesRequest) (*wallet_api.ConvertAddressesResponse, error) {
	var retAddressList []*wallet_api.Addresses
	for _, publicKeyItem := range req.PublicKey {
		var addressItem *wallet_api.Addresses
		publicKeyBytes, err := hex.DecodeString(strings.TrimPrefix(publicKeyItem.PublicKey, "0x"))
		if err != nil {
			addressItem = &wallet_api.Addresses{Address: ""}
			log.Error("decode public key fail", "err", err)
		} else {
			ss58Address := substratebase.SS58Encode(publicKeyBytes, c.ss58Prefix)
			log.Info("convert substrate addresses", "address", ss58Address)
			addressItem = &wallet_api.Addresses{Address: ss58Address}
		}
		retAddressList = append(retAddressList, addressItem)
	}
	return &wallet_api.ConvertAddressesResponse{
		Code:    wallet_api.ApiReturnCode_APISUCCESS,
		Msg:     "success",
		Address: retAddressList,
	}, nil
}

// ValidAddresses 验证地址是否有效
func (c SubstrateChainAdaptor) ValidAddresses(ctx context.Context, req *wallet_api.ValidAddressesRequest) (*wallet_api.ValidAddressesResponse, error) {
	var retAddressesValid []*wallet_api.AddressesValid
	for _, addressItem := range req.Addresses {
		var addressesValidItem wallet_api.AddressesValid
		addressesValidItem.Address = addressItem.GetAddress()
		_, _, err := substratebase.SS58Decode(addressItem.GetAddress())
		if err != nil {
			addressesValidItem.Valid = false
		} else {
			addressesValidItem.Valid = true
		}
		retAddressesValid = append(retAddressesValid, &addressesValidItem)
	}
	return &wallet_api.ValidAddressesResponse{
		Code:         wallet_api.ApiReturnCode_APISUCCESS,
		Msg:          "success",
		AddressValid: retAddressesValid,
	}, nil
}

// GetLastestBlock 获取最新区块高度和哈希
func (c SubstrateChainAdaptor) GetLastestBlock(ctx context.Context, req *wallet_api.LastestBlockRequest) (*wallet_api.LastestBlockResponse, error) {
	block, err := c.substrateClient.GetBlockLatest()
	if err != nil {
		log.Error("Get latest substrate block fail", "err", err)
		return nil, err
	}
	blockHash, err := c.substrateClient.GetBlockHash(uint64(block.Block.Header.Number))
	if err != nil {
		log.Error("Get latest substrate block hash fail", "err", err)
		return nil, err
	}
	return &wallet_api.LastestBlockResponse{
		Code:   wallet_api.ApiReturnCode_APISUCCESS,
		Msg:    "get lastest block success",
		Hash:   blockHash.Hex(),
		Height: uint64(block.Block.Header.Number),
	}, nil
}

// GetBlock 根据区块哈希或区块高度获取 Substrate 区块信息
// 当 req.IsBlockHash 为 true 时，通过区块哈希查询；否则通过区块高度查询
// 返回区块的高度、哈希及其中包含的所有 Extrinsic 交易列表
func (c SubstrateChainAdaptor) GetBlock(ctx context.Context, req *wallet_api.BlockRequest) (*wallet_api.BlockResponse, error) {
	var signedBlock *types.SignedBlock
	var blockHash types.Hash
	var err error
	// 调用方既可能传区块高度，也可能直接传区块哈希。
	// isBlockHash 为 true 时，通过区块哈希查询；否则通过区块高度查询
	if req.IsBlockHash {
		blockHashBytes, e := hex.DecodeString(strings.TrimPrefix(req.HashHeight, "0x"))
		if e != nil {
			return &wallet_api.BlockResponse{Code: wallet_api.ApiReturnCode_APIERROR, Msg: "invalid block hash"}, nil
		}
		copy(blockHash[:], blockHashBytes)
		signedBlock, err = c.substrateClient.GetBlock(blockHash)
	} else {
		blockNumber, e := strconv.ParseUint(req.HashHeight, 10, 64)
		if e != nil {
			return &wallet_api.BlockResponse{Code: wallet_api.ApiReturnCode_APIERROR, Msg: "invalid block number"}, nil
		}
		blockHash, e = c.substrateClient.GetBlockHash(blockNumber)
		if e != nil {
			return &wallet_api.BlockResponse{Code: wallet_api.ApiReturnCode_APIERROR, Msg: "get block hash fail"}, nil
		}
		signedBlock, err = c.substrateClient.GetBlock(blockHash)
	}

	if err != nil {
		log.Error("Get substrate block fail", "err", err)
		return &wallet_api.BlockResponse{Code: wallet_api.ApiReturnCode_APIERROR, Msg: "get block failed"}, nil
	}

	var transactionList []*wallet_api.TransactionList

	meta, metaErr := c.getMetadata()
	if metaErr != nil {
		log.Warn("Get substrate metadata fail, will parse extrinsics without call details", "err", metaErr)
	}

	callRegistry, crErr := c.getCallRegistry()
	if crErr != nil {
		log.Warn("Get substrate call registry fail, falling back to basic extrinsic parsing", "err", crErr)
	}

	eventSummaries, eventErr := c.getBlockExtrinsicEventSummaries(blockHash)
	if eventErr != nil {
		log.Warn("Get substrate block events fail, transaction fields will be partially filled", "err", eventErr)
	}

	for idx, ext := range signedBlock.Block.Extrinsics {
		// 主路径优先走结构化 call 解码，再用事件结果回填 fee/status/to/amount。
		txItem := c.parseBlockExtrinsic(ext, meta, callRegistry)
		if eventSummaries != nil {
			c.applyExtrinsicEventSummary(txItem, eventSummaries[uint32(idx)])
		}
		transactionList = append(transactionList, txItem)
	}

	return &wallet_api.BlockResponse{
		Code:         wallet_api.ApiReturnCode_APISUCCESS,
		Msg:          "get block success",
		Height:       strconv.FormatUint(uint64(signedBlock.Block.Header.Number), 10),
		Hash:         blockHash.Hex(),
		Transactions: transactionList,
	}, nil
}

// parseBlockExtrinsic 负责单笔 extrinsic 的基础解析。
// 这里先走结构化 call 解码；如果遇到不支持的调用或解码失败，再回退到旧的基础解析逻辑。
func (c SubstrateChainAdaptor) parseBlockExtrinsic(ext types.Extrinsic, meta *types.Metadata, callRegistry registry.CallRegistry) *wallet_api.TransactionList {
	if callRegistry != nil {
		txItem, err := c.parseExtrinsicToTransaction(ext, callRegistry)
		if err == nil {
			return txItem
		}
		log.Debug("Parse substrate extrinsic via call registry failed, falling back to basic parser", "err", err)
	}

	return c.parseExtrinsicBasic(ext, meta)
}

// GetTransactionByHash 获取指定交易哈希的交易详情
// Substrate 链不支持按 txHash 直接查询，需要通过 blockHash:extrinsicIndex 格式定位交易
func (c SubstrateChainAdaptor) GetTransactionByHash(ctx context.Context, req *wallet_api.TransactionByHashRequest) (*wallet_api.TransactionByHashResponse, error) {
	var signedBlock *types.SignedBlock
	var matchedBlockHash types.Hash
	var extIndex int
	var found bool

	if parts := strings.SplitN(req.Hash, ":", 2); len(parts) == 2 {
		blockHashBytes, e := hex.DecodeString(strings.TrimPrefix(parts[0], "0x"))
		if e != nil {
			return &wallet_api.TransactionByHashResponse{Code: wallet_api.ApiReturnCode_APIERROR, Msg: "invalid block hash in composite key"}, nil
		}
		var blockHash types.Hash
		copy(blockHash[:], blockHashBytes)
		signedBlock, e = c.substrateClient.GetBlock(blockHash)
		if e != nil {
			return &wallet_api.TransactionByHashResponse{Code: wallet_api.ApiReturnCode_APIERROR, Msg: "get block failed"}, nil
		}
		matchedBlockHash = blockHash
		extIndex, e = strconv.Atoi(parts[1])
		if e != nil || extIndex < 0 || extIndex >= len(signedBlock.Block.Extrinsics) {
			return &wallet_api.TransactionByHashResponse{Code: wallet_api.ApiReturnCode_APIERROR, Msg: "invalid extrinsic index"}, nil
		}
		found = true
	} else {
		// 原生 Substrate 节点没有像 EVM 一样稳定的 txHash 索引。
		// 本地测试场景下，这里退化为并发扫块：多个 worker 各自按步长回扫区块，
		// 遍历 block.extrinsics 后重新计算哈希并匹配目标 txHash。
		located, locateErr := c.locateExtrinsicByHash(ctx, req.Hash, substrateTxHashScanWorkers)
		if locateErr != nil {
			return nil, locateErr
		}
		if located != nil {
			signedBlock = located.signedBlock
			matchedBlockHash = located.blockHash
			extIndex = located.extIndex
			found = true
		}
	}

	if !found || signedBlock == nil {
		return &wallet_api.TransactionByHashResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  "transaction not found",
		}, nil
	}

	ext := signedBlock.Block.Extrinsics[extIndex]

	meta, _ := c.getMetadata()

	callRegistry, crErr := c.getCallRegistry()
	if crErr != nil {
		log.Warn("Get substrate call registry fail, returning basic tx info", "err", crErr)
		txItem := c.parseExtrinsicBasic(ext, meta)
		return &wallet_api.TransactionByHashResponse{
			Code:        wallet_api.ApiReturnCode_APISUCCESS,
			Msg:         "get transaction success (basic, call registry unavailable)",
			Transaction: txItem,
		}, nil
	}

	txItem, parseErr := c.parseExtrinsicToTransaction(ext, callRegistry)
	if parseErr != nil {
		log.Debug("Parse substrate transaction via call registry failed, falling back to basic parser", "err", parseErr)
		txItem = c.parseExtrinsicBasic(ext, meta)
	}

	if eventSummaries, eventErr := c.getBlockExtrinsicEventSummaries(matchedBlockHash); eventErr == nil {
		c.applyExtrinsicEventSummary(txItem, eventSummaries[uint32(extIndex)])
	}
	return &wallet_api.TransactionByHashResponse{
		Code:        wallet_api.ApiReturnCode_APISUCCESS,
		Msg:         "get transaction success",
		Transaction: txItem,
	}, nil
}

// locateExtrinsicByHash txHash 查询场景。
// 它会启动多个 worker，从最新区块开始按固定步长并发回扫，
// 每个 worker 负责一组不重复的高度，直到找到匹配的 extrinsic。
func (c SubstrateChainAdaptor) locateExtrinsicByHash(
	ctx context.Context,
	txHash string,
	workerCount int,
) (*locatedExtrinsic, error) {
	if workerCount <= 0 {
		workerCount = 1
	}

	latestBlock, err := c.substrateClient.GetBlockLatest()
	if err != nil {
		return nil, fmt.Errorf("get latest block failed: %w", err)
	}

	targetHash := strings.TrimPrefix(strings.ToLower(txHash), "0x")
	startHeight := uint64(latestBlock.Block.Header.Number)
	log.Info("开始并发扫描交易哈希", "txHash", txHash, "最新区块高度", startHeight, "worker数量", workerCount)

	scanCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	resultCh := make(chan *locatedExtrinsic, 1)
	doneCh := make(chan struct{})

	var wg sync.WaitGroup
	for workerID := 0; workerID < workerCount; workerID++ {
		wg.Add(1)
		go func(workerOffset uint64) {
			defer wg.Done()

			if startHeight < workerOffset {
				return
			}

			step := uint64(workerCount)
			scannedBlocks := uint64(0)
			for height := startHeight - workerOffset; ; {
				select {
				case <-scanCtx.Done():
					return
				default:
				}

				scannedBlocks++
				if scannedBlocks == 1 || scannedBlocks%substrateTxHashScanLogInterval == 0 {
					rangeHigh := height
					rangeLow := uint64(0)
					rangeSpan := step * (substrateTxHashScanLogInterval - 1)
					if rangeHigh > rangeSpan {
						rangeLow = rangeHigh - rangeSpan
					}
					log.Info(
						"并发扫描交易进度",
						"worker", workerOffset,
						"当前扫描高度区间", fmt.Sprintf("%d~%d", rangeHigh, rangeLow),
						"步长", step,
						"已扫描区块数", scannedBlocks,
					)
				}

				blockHash, hashErr := c.substrateClient.GetBlockHash(height)
				if hashErr == nil {
					signedBlock, blockErr := c.substrateClient.GetBlock(blockHash)
					if blockErr == nil {
						for extIndex, ext := range signedBlock.Block.Extrinsics {
							hash, extHashErr := blake2bHashExtrinsic(ext)
							if extHashErr != nil {
								continue
							}
							if strings.TrimPrefix(strings.ToLower(hash), "0x") != targetHash {
								continue
							}

							select {
							case resultCh <- &locatedExtrinsic{
								blockHash:   blockHash,
								signedBlock: signedBlock,
								extIndex:    extIndex,
							}:
								log.Info(
									"并发扫描命中目标交易",
									"txHash", txHash,
									"worker", workerOffset,
									"区块高度", uint64(signedBlock.Block.Header.Number),
									"区块哈希", blockHash.Hex(),
									"extrinsicIndex", extIndex,
								)
								cancel()
							default:
							}
							return
						}
					}
				}

				if height < step {
					return
				}
				height -= step
			}
		}(uint64(workerID))
	}

	go func() {
		wg.Wait()
		close(doneCh)
	}()

	select {
	case result := <-resultCh:
		return result, nil
	case <-doneCh:
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GetTransactionByAddress 获取指定地址的交易列表
func (c SubstrateChainAdaptor) GetTransactionByAddress(ctx context.Context, req *wallet_api.TransactionByAddressRequest) (*wallet_api.TransactionByAddressResponse, error) {
	targetAddress := normalizeComparableAddress(req.Address)
	if targetAddress == "" {
		return &wallet_api.TransactionByAddressResponse{
			Code:        wallet_api.ApiReturnCode_APIERROR,
			Msg:         "address is empty",
			Transaction: nil,
		}, nil
	}

	page := req.Page
	if page == 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize == 0 {
		pageSize = substrateTxAddressDefaultPageSize
	}
	if pageSize > substrateTxAddressMaxPageSize {
		pageSize = substrateTxAddressMaxPageSize
	}

	latestBlock, err := c.substrateClient.GetBlockLatest()
	if err != nil {
		log.Error("Get latest substrate block fail", "err", err)
		return nil, err
	}

	meta, metaErr := c.getMetadata()
	if metaErr != nil {
		log.Warn("Get substrate metadata fail, will parse extrinsics without call details", "err", metaErr)
	}

	callRegistry, crErr := c.getCallRegistry()
	if crErr != nil {
		log.Warn("Get substrate call registry fail, falling back to basic extrinsic parsing", "err", crErr)
	}

	eventRegistry, erErr := c.getEventRegistry()
	if erErr != nil {
		log.Warn("Get substrate event registry fail, will parse extrinsics without event details", "err", erErr)
	}

	offset := (page - 1) * pageSize
	needed := offset + pageSize
	matched := make([]*wallet_api.TransactionList, 0, pageSize)
	seen := uint64(0)

	for currentHeight := uint64(latestBlock.Block.Header.Number); ; {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		batchEnd := uint64(0)
		if currentHeight >= substrateTxAddressScanBatchSize {
			batchEnd = currentHeight - substrateTxAddressScanBatchSize + 1
		}

		log.Info(
			"开始并发扫描地址交易批次",
			"address", req.Address,
			"heightRange", fmt.Sprintf("%d~%d", currentHeight, batchEnd),
			"workerCount", substrateTxAddressScanWorkers,
		)

		blockTxs, scanErr := c.scanAddressTransactionsInHeightRange(
			ctx,
			currentHeight,
			batchEnd,
			req.Address,
			targetAddress,
			meta,
			callRegistry,
			eventRegistry,
			substrateTxAddressScanWorkers,
		)
		if scanErr != nil {
			return nil, scanErr
		}

		for height := currentHeight; ; height-- {
			for _, txItem := range blockTxs[height] {
				seen++
				if seen <= offset {
					continue
				}

				matched = append(matched, txItem)
				if uint64(len(matched)) >= pageSize || seen >= needed {
					return &wallet_api.TransactionByAddressResponse{
						Code:        wallet_api.ApiReturnCode_APISUCCESS,
						Msg:         "get tx list by address success",
						Transaction: matched,
					}, nil
				}
			}

			if height == batchEnd {
				break
			}
		}

		if batchEnd == 0 {
			break
		}
		currentHeight = batchEnd - 1
	}

	return &wallet_api.TransactionByAddressResponse{
		Code:        wallet_api.ApiReturnCode_APISUCCESS,
		Msg:         "get tx list by address success",
		Transaction: matched,
	}, nil
}

// scanAddressTransactionsInHeightRange 并发余额查询结果
// 它会并发地从每个区块中提取交易信息，根据 callRegistry 和 eventRegistry 解析交易调用和事件。
// 返回一个映射，键为区块高度，值为该区块中包含目标地址的交易列表。
func (c SubstrateChainAdaptor) scanAddressTransactionsInHeightRange(
	ctx context.Context,
	startHeight uint64,
	endHeight uint64,
	requestAddress string,
	targetAddress string,
	meta *types.Metadata,
	callRegistry registry.CallRegistry,
	eventRegistry registry.EventRegistry,
	workerCount int,
) (map[uint64][]*wallet_api.TransactionList, error) {
	if workerCount <= 0 {
		workerCount = 1
	}

	totalBlocks := startHeight - endHeight + 1
	if uint64(workerCount) > totalBlocks {
		workerCount = int(totalBlocks)
	}

	jobs := make(chan uint64, workerCount)
	results := make(chan addressBlockTransactions, workerCount)
	scanCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	for workerID := 0; workerID < workerCount; workerID++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for height := range jobs {
				select {
				case <-scanCtx.Done():
					return
				default:
				}

				blockHash, hashErr := c.substrateClient.GetBlockHash(height)
				if hashErr != nil {
					log.Warn("Get substrate block hash fail while scanning address txs", "worker", workerID, "height", height, "err", hashErr)
					continue
				}

				signedBlock, blockErr := c.substrateClient.GetBlock(blockHash)
				if blockErr != nil {
					log.Warn("Get substrate block fail while scanning address txs", "worker", workerID, "height", height, "blockHash", blockHash.Hex(), "err", blockErr)
					continue
				}

				parsed := make([]addressMatchedTransaction, 0, len(signedBlock.Block.Extrinsics))
				matched := make([]addressMatchedTransaction, 0)
				for idx, ext := range signedBlock.Block.Extrinsics {
					txItem := c.parseBlockExtrinsic(ext, meta, callRegistry)
					parsedTx := addressMatchedTransaction{
						extIndex:    uint32(idx),
						transaction: txItem,
					}
					parsed = append(parsed, parsedTx)
					if transactionInvolvesAddress(txItem, targetAddress) {
						matched = append(matched, parsedTx)
					}
				}

				blockEventTransactions := make([]*wallet_api.TransactionList, 0)
				if meta != nil && eventRegistry != nil {
					parsedEvents, eventErr := c.parseBlockEvents(blockHash, meta, eventRegistry)
					if eventErr != nil {
						log.Warn("Get substrate block events fail while scanning address txs", "worker", workerID, "height", height, "blockHash", blockHash.Hex(), "err", eventErr)
					} else {
						if len(matched) > 0 {
							for _, match := range matched {
								c.applyExtrinsicEventSummary(match.transaction, parsedEvents.summaries[match.extIndex])
							}
						} else {
							for _, parsedTx := range parsed {
								c.applyExtrinsicEventSummary(parsedTx.transaction, parsedEvents.summaries[parsedTx.extIndex])
								if transactionInvolvesAddress(parsedTx.transaction, targetAddress) {
									matched = append(matched, parsedTx)
								}
							}
						}

						for _, txItem := range parsedEvents.blockTransactions {
							if transactionInvolvesAddress(txItem, targetAddress) {
								blockEventTransactions = append(blockEventTransactions, txItem)
							}
						}
					}
				}

				if len(matched) == 0 && len(blockEventTransactions) == 0 {
					continue
				}

				transactions := make([]*wallet_api.TransactionList, 0, len(matched)+len(blockEventTransactions))
				for _, match := range matched {
					transactions = append(transactions, match.transaction)
				}
				transactions = append(transactions, blockEventTransactions...)

				log.Info(
					"命中地址交易",
					"requestAddress", requestAddress,
					"height", height,
					"blockHash", blockHash.Hex(),
					"matchedCount", len(transactions),
					"extrinsicMatchedCount", len(matched),
					"blockEventMatchedCount", len(blockEventTransactions),
				)

				select {
				case results <- addressBlockTransactions{height: height, transactions: transactions}:
				case <-scanCtx.Done():
					return
				}
			}
		}(workerID)
	}

	go func() {
		for height := startHeight; ; height-- {
			select {
			case jobs <- height:
			case <-scanCtx.Done():
				close(jobs)
				return
			}

			if height == endHeight {
				break
			}
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	blockTxs := make(map[uint64][]*wallet_api.TransactionList)
	for result := range results {
		blockTxs[result.height] = result.transactions
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return blockTxs, nil
}

// transactionInvolvesAddress 检查交易是否包含指定地址
// 它会检查交易的 From 和 To 字段是否包含目标地址。
// 如果交易包含目标地址，返回 true；否则返回 false。
func transactionInvolvesAddress(txItem *wallet_api.TransactionList, targetAddress string) bool {
	if txItem == nil || targetAddress == "" {
		return false
	}

	for _, from := range txItem.From {
		if from != nil && normalizeComparableAddress(from.Address) == targetAddress {
			return true
		}
	}

	for _, to := range txItem.To {
		if to != nil && normalizeComparableAddress(to.Address) == targetAddress {
			return true
		}
	}

	return false
}

// normalizeComparableAddress
// 它会将地址转换为小写，并移除首尾空格。
// 如果地址以 "0x" 开头，返回小写后的地址；
// 否则，尝试将 SS58 编码的地址转换为十六进制格式，返回小写后的十六进制地址。
// 如果转换失败，返回原始地址的小写形式。
func normalizeComparableAddress(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}

	if strings.HasPrefix(strings.ToLower(address), "0x") {
		return strings.ToLower(address)
	}

	if pubKeyHex, err := substratebase.SS58DecodeToHex(address); err == nil {
		return strings.ToLower(pubKeyHex)
	}

	return strings.ToLower(address)
}

// GetAccountBalance 获取指定地址的账户余额
func (c SubstrateChainAdaptor) GetAccountBalance(ctx context.Context, req *wallet_api.AccountBalanceRequest) (*wallet_api.AccountBalanceResponse, error) {
	pubKeyHex, err := substratebase.SS58DecodeToHex(req.Address)
	if err != nil {
		return &wallet_api.AccountBalanceResponse{
			Code:    wallet_api.ApiReturnCode_APIERROR,
			Msg:     "invalid substrate address",
			Balance: "0",
		}, nil
	}

	if strings.TrimSpace(req.ContractAddress) != "" {
		balance, balanceErr := c.getAssetAccountBalance(pubKeyHex, req.ContractAddress)
		if balanceErr != nil {
			return &wallet_api.AccountBalanceResponse{
				Code:    wallet_api.ApiReturnCode_APIERROR,
				Msg:     "get substrate asset balance fail",
				Balance: "0",
			}, nil
		}
		symbol, decimals := c.getSubstrateAssetMetadataOrDefault(req.ContractAddress)

		return &wallet_api.AccountBalanceResponse{
			Code:    wallet_api.ApiReturnCode_APISUCCESS,
			Msg:     "get substrate asset balance success",
			Balance: balance,
			TokenBalances: []*wallet_api.TokenBalance{{
				Symbol:          symbol,
				Balance:         balance,
				ContractAddress: strings.TrimSpace(req.ContractAddress),
				Decimals:        decimals,
			}},
		}, nil
	}

	accountInfo, err := c.substrateClient.GetAccountInfoLatest(pubKeyHex)
	if err != nil {
		return &wallet_api.AccountBalanceResponse{
			Code:    wallet_api.ApiReturnCode_APIERROR,
			Msg:     "get substrate account balance fail",
			Balance: "0",
		}, nil
	}

	tokenBalances := []*wallet_api.TokenBalance{{
		Symbol:   c.substrateNativeSymbol(),
		Balance:  accountInfo.Data.Free.String(),
		Decimals: c.substrateNativeDecimals(),
	}}

	assetBalances, err := c.getAllAssetAccountBalances(pubKeyHex)
	if err != nil {
		return &wallet_api.AccountBalanceResponse{
			Code:    wallet_api.ApiReturnCode_APIERROR,
			Msg:     "get substrate token balances fail",
			Balance: accountInfo.Data.Free.String(),
		}, nil
	}
	for _, assetBalance := range assetBalances {
		tokenBalances = append(tokenBalances, &wallet_api.TokenBalance{
			Symbol:          assetBalance.Symbol,
			Balance:         assetBalance.Balance,
			ContractAddress: strconv.FormatUint(uint64(assetBalance.AssetID), 10),
			Decimals:        assetBalance.Decimals,
		})
	}

	return &wallet_api.AccountBalanceResponse{
		Code:          wallet_api.ApiReturnCode_APISUCCESS,
		Msg:           "get substrate account token balances success",
		Balance:       accountInfo.Data.Free.String(),
		TokenBalances: tokenBalances,
	}, nil
}

// SendTransaction 发送交易
func (c SubstrateChainAdaptor) SendTransaction(ctx context.Context, req *wallet_api.SendTransactionsRequest) (*wallet_api.SendTransactionResponse, error) {
	var txListRet []*wallet_api.RawTransactionReturn
	for _, txItem := range req.RawTx {
		extBytes, err := hex.DecodeString(strings.TrimPrefix(txItem.RawTx, "0x"))
		if err != nil {
			txListRet = append(txListRet, &wallet_api.RawTransactionReturn{
				TxHash:    "",
				IsSuccess: false,
				Message:   "decode raw tx failed",
			})
			continue
		}

		var ext types.Extrinsic
		decoder := scale.NewDecoder(bytes.NewReader(extBytes))
		if err := decoder.Decode(&ext); err != nil {
			txListRet = append(txListRet, &wallet_api.RawTransactionReturn{
				TxHash:    "",
				IsSuccess: false,
				Message:   "decode extrinsic failed",
			})
			continue
		}

		extHash, err := c.substrateClient.SubmitExtrinsic(ext)
		if err != nil {
			log.Error("SubmitExtrinsic failed", "err", err)
			txListRet = append(txListRet, &wallet_api.RawTransactionReturn{
				TxHash:    "",
				IsSuccess: false,
				Message:   "submit extrinsic failed",
			})
		} else {
			txListRet = append(txListRet, &wallet_api.RawTransactionReturn{
				TxHash:    extHash.Hex(),
				IsSuccess: true,
				Message:   "submit extrinsic success",
			})
		}
	}
	return &wallet_api.SendTransactionResponse{
		Code:   wallet_api.ApiReturnCode_APISUCCESS,
		Msg:    "send tx success",
		TxnRet: txListRet,
	}, nil
}

// getAssetAccountBalance 查询资产账户余额
// 根据资产标识符和账户地址查询资产账户的余额。
// 如果资产标识符是数字，会将其转换为 uint32 类型；否则，会尝试将其解析为 uint32 类型。
// 如果查询成功，返回资产账户的余额字符串；否则返回错误。
func (c SubstrateChainAdaptor) getAssetAccountBalance(pubKeyHex string, assetIdentifier string) (string, error) {
	assetID, err := parseSubstrateAssetID(assetIdentifier)
	if err != nil {
		return "", err
	}

	meta, err := c.getMetadata()
	if err != nil {
		return "", err
	}

	accountID, err := types.NewAccountIDFromHexString(pubKeyHex)
	if err != nil {
		return "", fmt.Errorf("invalid account address: %w", err)
	}

	assetIDKey := []byte{byte(assetID), byte(assetID >> 8), byte(assetID >> 16), byte(assetID >> 24)}
	key, err := types.CreateStorageKey(meta, "Assets", "Account", assetIDKey, accountID.ToBytes())
	if err != nil {
		return "", fmt.Errorf("create assets account storage key failed: %w", err)
	}

	latestBlock, err := c.substrateClient.GetBlockLatest()
	if err != nil {
		return "", fmt.Errorf("get latest block failed: %w", err)
	}
	blockHash, err := c.substrateClient.GetBlockHash(uint64(latestBlock.Block.Header.Number))
	if err != nil {
		return "", fmt.Errorf("get latest block hash failed: %w", err)
	}

	raw, err := c.substrateClient.GetStorageRaw(key, blockHash)
	if err != nil {
		return "", fmt.Errorf("get assets account storage failed: %w", err)
	}
	if raw == nil || len(*raw) < 16 {
		return "0", nil
	}

	// pallet_assets::Account stores balance as the first field, encoded as little-endian u128.
	leBalance := []byte(*raw)[:16]
	beBalance := make([]byte, len(leBalance))
	for i := range leBalance {
		beBalance[len(leBalance)-1-i] = leBalance[i]
	}
	return new(big.Int).SetBytes(beBalance).String(), nil
}

// getAllAssetAccountBalances 查询所有资产账户余额
// 根据账户地址查询所有资产账户的余额。
// 它会获取最新的元数据和资产标识符列表，并发地查询每个资产账户的余额。
// 返回一个包含资产账户余额的切片。
func (c SubstrateChainAdaptor) getAllAssetAccountBalances(pubKeyHex string) ([]substrateAssetBalance, error) {
	meta, err := c.getMetadata()
	if err != nil {
		return nil, err
	}

	accountID, err := types.NewAccountIDFromHexString(pubKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid account address: %w", err)
	}

	blockHash, err := c.getLatestBlockHash()
	if err != nil {
		return nil, err
	}

	assetIDs, err := c.getSubstrateAssetIDs(blockHash)
	if err != nil {
		return nil, err
	}

	return c.getAssetAccountBalancesConcurrently(assetIDs, accountID.ToBytes(), meta, blockHash)
}

// getAssetAccountBalancesConcurrently 并发查询资产账户余额
// 根据资产标识符列表和账户地址并发查询资产账户的余额。
// 它会使用多个 goroutine 并发地查询每个资产账户的余额，返回一个包含资产账户余额的切片。
func (c SubstrateChainAdaptor) getAssetAccountBalancesConcurrently(assetIDs []uint32, accountBytes []byte, meta *types.Metadata, blockHash types.Hash) ([]substrateAssetBalance, error) {
	if len(assetIDs) == 0 {
		return nil, nil
	}

	workerCount := substrateAssetBalanceLookupWorkers
	if workerCount > len(assetIDs) {
		workerCount = len(assetIDs)
	}

	jobs := make(chan uint32, workerCount)
	results := make(chan substrateAssetBalanceLookupResult, len(assetIDs))

	var wg sync.WaitGroup
	for workerID := 0; workerID < workerCount; workerID++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for assetID := range jobs {
				balance, ok, err := c.getAssetAccountBalanceAt(assetID, accountBytes, meta, blockHash)
				if err != nil {
					results <- substrateAssetBalanceLookupResult{err: err}
					continue
				}
				if !ok || balance.Sign() == 0 {
					continue
				}

				metadata, metadataErr := c.getSubstrateAssetMetadata(assetID, meta, blockHash)
				symbol := strconv.FormatUint(uint64(assetID), 10)
				decimals := uint32(0)
				if metadataErr == nil {
					if metadataSymbol := strings.TrimSpace(string(metadata.Symbol)); metadataSymbol != "" {
						symbol = metadataSymbol
					}
					decimals = uint32(metadata.Decimals)
				} else {
					log.Warn("Get substrate asset metadata fail, using asset id as symbol", "assetID", assetID, "err", metadataErr)
				}

				results <- substrateAssetBalanceLookupResult{
					balance: substrateAssetBalance{
						AssetID:  assetID,
						Symbol:   symbol,
						Balance:  balance.String(),
						Decimals: decimals,
					},
				}
			}
		}()
	}

	go func() {
		for _, assetID := range assetIDs {
			jobs <- assetID
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	assetBalances := make([]substrateAssetBalance, 0)
	for result := range results {
		if result.err != nil {
			return nil, result.err
		}
		assetBalances = append(assetBalances, result.balance)
	}

	sort.Slice(assetBalances, func(i, j int) bool {
		return assetBalances[i].AssetID < assetBalances[j].AssetID
	})
	return assetBalances, nil
}

// getSubstrateAssetIDs 查询所有资产标识符
// 根据块哈希查询所有资产标识符。
// 它会获取资产存储前缀下的所有存储键，解析出资产标识符，并返回一个包含所有资产标识符的切片。
func (c SubstrateChainAdaptor) getSubstrateAssetIDs(blockHash types.Hash) ([]uint32, error) {
	prefix := substrateStoragePrefix("Assets", "Asset")
	keys, err := c.substrateClient.GetStorageKeys(prefix, blockHash)
	if err != nil {
		return nil, fmt.Errorf("get assets ids failed: %w", err)
	}

	assetIDs := make([]uint32, 0, len(keys))
	for _, key := range keys {
		assetID, ok := parseSubstrateAssetStorageKey(key)
		if !ok {
			continue
		}
		assetIDs = append(assetIDs, assetID)
	}

	sort.Slice(assetIDs, func(i, j int) bool {
		return assetIDs[i] < assetIDs[j]
	})
	return assetIDs, nil
}

func (c SubstrateChainAdaptor) getAssetAccountBalanceAt(assetID uint32, accountBytes []byte, meta *types.Metadata, blockHash types.Hash) (*big.Int, bool, error) {
	assetIDKey := []byte{byte(assetID), byte(assetID >> 8), byte(assetID >> 16), byte(assetID >> 24)}
	key, err := types.CreateStorageKey(meta, "Assets", "Account", assetIDKey, accountBytes)
	if err != nil {
		return nil, false, fmt.Errorf("create assets account storage key failed: %w", err)
	}

	raw, err := c.substrateClient.GetStorageRaw(key, blockHash)
	if err != nil {
		return nil, false, fmt.Errorf("get assets account storage failed: %w", err)
	}
	balance, ok := decodeSubstrateAssetAccountBalance(raw)
	return balance, ok, nil
}

func parseSubstrateAssetStorageKey(key types.StorageKey) (uint32, bool) {
	const (
		storagePrefixLen       = 32
		blake2ConcatHashLen    = 16
		substrateAssetIDKeyLen = 4
	)

	expectedLen := storagePrefixLen + blake2ConcatHashLen + substrateAssetIDKeyLen
	if len(key) < expectedLen {
		return 0, false
	}

	assetIDOffset := storagePrefixLen + blake2ConcatHashLen
	return binary.LittleEndian.Uint32(key[assetIDOffset : assetIDOffset+substrateAssetIDKeyLen]), true
}

// getSubstrateAssetMetadataOrDefault 查询资产元数据或默认值
// 根据资产标识符查询资产元数据。
// 如果查询成功，返回资产的符号和小数位数；否则返回默认值。
func (c SubstrateChainAdaptor) getSubstrateAssetMetadataOrDefault(assetIdentifier string) (string, uint32) {
	assetID, err := parseSubstrateAssetID(assetIdentifier)
	if err != nil {
		return substrateAssetSymbol(assetIdentifier), substrateAssetDecimals(assetIdentifier)
	}

	meta, err := c.getMetadata()
	if err != nil {
		return substrateAssetSymbol(assetIdentifier), substrateAssetDecimals(assetIdentifier)
	}

	blockHash, err := c.getLatestBlockHash()
	if err != nil {
		return substrateAssetSymbol(assetIdentifier), substrateAssetDecimals(assetIdentifier)
	}

	metadata, err := c.getSubstrateAssetMetadata(assetID, meta, blockHash)
	if err != nil {
		return substrateAssetSymbol(assetIdentifier), substrateAssetDecimals(assetIdentifier)
	}

	symbol := strings.TrimSpace(string(metadata.Symbol))
	if symbol == "" {
		symbol = substrateAssetSymbol(assetIdentifier)
	}
	return symbol, uint32(metadata.Decimals)
}

// getSubstrateAssetMetadata 查询资产元数据
// 根据资产标识符查询资产元数据。
// 获取资产存储前缀下的资产元数据存储键，查询资产元数据，并返回资产的符号和小数位数。
func (c SubstrateChainAdaptor) getSubstrateAssetMetadata(assetID uint32, meta *types.Metadata, blockHash types.Hash) (*substrateAssetMetadata, error) {
	assetIDKey := []byte{byte(assetID), byte(assetID >> 8), byte(assetID >> 16), byte(assetID >> 24)}
	key, err := types.CreateStorageKey(meta, "Assets", "Metadata", assetIDKey)
	if err != nil {
		return nil, fmt.Errorf("create assets metadata storage key failed: %w", err)
	}

	raw, err := c.substrateClient.GetStorageRaw(key, blockHash)
	if err != nil {
		return nil, fmt.Errorf("get assets metadata storage failed: %w", err)
	}
	if raw == nil {
		return nil, fmt.Errorf("asset metadata not found")
	}

	var metadata substrateAssetMetadata
	decoder := scale.NewDecoder(bytes.NewReader([]byte(*raw)))
	if err := decoder.Decode(&metadata); err != nil {
		return nil, fmt.Errorf("decode assets metadata failed: %w", err)
	}
	return &metadata, nil
}

func (c SubstrateChainAdaptor) getLatestBlockHash() (types.Hash, error) {
	latestBlock, err := c.substrateClient.GetBlockLatest()
	if err != nil {
		return types.Hash{}, fmt.Errorf("get latest block failed: %w", err)
	}
	blockHash, err := c.substrateClient.GetBlockHash(uint64(latestBlock.Block.Header.Number))
	if err != nil {
		return types.Hash{}, fmt.Errorf("get latest block hash failed: %w", err)
	}
	return blockHash, nil
}

// substrateNativeSymbol 查询子 Hughes 链的本地资产符号
// 获取子 Hughes 链的资产符号属性。
// 如果属性存在且符号不为空，则返回该符号；否则返回默认值。
func (c SubstrateChainAdaptor) substrateNativeSymbol() string {
	props, err := c.substrateClient.GetChainProperties()
	if err == nil && props != nil && props.IsTokenSymbol {
		if symbol := strings.TrimSpace(string(props.AsTokenSymbol)); symbol != "" {
			return symbol
		}
	}

	if c.ss58Prefix == uint8(substratebase.KusamaSS58Prefix) {
		return "KSM"
	}
	return "DOT"
}

func (c SubstrateChainAdaptor) substrateNativeDecimals() uint32 {
	props, err := c.substrateClient.GetChainProperties()
	if err == nil && props != nil && props.IsTokenDecimals {
		return uint32(props.AsTokenDecimals)
	}

	if c.ss58Prefix == uint8(substratebase.KusamaSS58Prefix) {
		return 12
	}
	return 10
}

func substrateStoragePrefix(prefix string, method string) types.StorageKey {
	return types.StorageKey(append(xxhash.New128([]byte(prefix)).Sum(nil), xxhash.New128([]byte(method)).Sum(nil)...))
}

func parseSubstrateAssetAccountStorageKey(key types.StorageKey, accountBytes []byte) (uint32, bool) {
	const (
		storagePrefixLen       = 32
		blake2ConcatHashLen    = 16
		substrateAssetIDKeyLen = 4
		substrateAccountIDLen  = 32
	)

	expectedLen := storagePrefixLen + blake2ConcatHashLen + substrateAssetIDKeyLen + blake2ConcatHashLen + substrateAccountIDLen
	if len(key) < expectedLen || len(accountBytes) != substrateAccountIDLen {
		return 0, false
	}

	assetIDOffset := storagePrefixLen + blake2ConcatHashLen
	accountOffset := assetIDOffset + substrateAssetIDKeyLen + blake2ConcatHashLen
	if !bytes.Equal(key[accountOffset:accountOffset+substrateAccountIDLen], accountBytes) {
		return 0, false
	}

	return binary.LittleEndian.Uint32(key[assetIDOffset : assetIDOffset+substrateAssetIDKeyLen]), true
}

func decodeSubstrateAssetAccountBalance(raw *types.StorageDataRaw) (*big.Int, bool) {
	if raw == nil || len(*raw) < 16 {
		return nil, false
	}

	leBalance := []byte(*raw)[:16]
	beBalance := make([]byte, len(leBalance))
	for i := range leBalance {
		beBalance[len(leBalance)-1-i] = leBalance[i]
	}
	return new(big.Int).SetBytes(beBalance), true
}

func parseSubstrateAssetID(assetIdentifier string) (uint32, error) {
	normalized := strings.TrimSpace(assetIdentifier)
	normalized = strings.TrimPrefix(normalized, "#")
	if strings.EqualFold(normalized, "DED") {
		return 30, nil
	}

	assetID, err := strconv.ParseUint(normalized, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid substrate asset id %q: %w", assetIdentifier, err)
	}
	return uint32(assetID), nil
}

// BuildTransactionSchema 构建交易 schema
func substrateAssetSymbol(assetIdentifier string) string {
	normalized := strings.TrimSpace(assetIdentifier)
	if strings.EqualFold(strings.TrimPrefix(normalized, "#"), "30") || strings.EqualFold(normalized, "DED") {
		return "DED"
	}
	return normalized
}

func substrateAssetDecimals(assetIdentifier string) uint32 {
	normalized := strings.TrimSpace(assetIdentifier)
	if strings.EqualFold(strings.TrimPrefix(normalized, "#"), "30") || strings.EqualFold(normalized, "DED") {
		return 10
	}
	return 0
}

func (c SubstrateChainAdaptor) BuildTransactionSchema(ctx context.Context, request *wallet_api.TransactionSchemaRequest) (*wallet_api.TransactionSchemaResponse, error) {
	schema := `{
  "type": "object",
  "properties": {
    "mode": {
      "type": "string",
      "description": "交易模式，substrate",
      "enum": ["substrate"]
    },
    "from_address": {
      "type": "string",
      "description": "发送方地址，SS58格式"
    },
    "to_address": {
      "type": "string",
      "description": "接收方地址，SS58格式"
    },
    "amount": {
      "type": "string",
      "description": "转移金额，单位为最小精度"
    },
    "tip": {
      "type": "integer",
      "description": "给区块作者的小费，单位为最小精度"
    },
    "nonce": {
      "type": "integer",
      "description": "发送方账户的nonce"
    },
    "era": {
      "type": "string",
      "description": "交易有效期，immortal表示永不过期，或指定 mortal 格式"
    },
    "block_hash": {
      "type": "string",
      "description": "用于era计算的区块哈希，hex格式"
    },
    "block_number": {
      "type": "integer",
      "description": "用于era计算的区块高度"
    }
  },
  "required": ["mode", "from_address", "to_address", "amount", "nonce"]
}`
	return &wallet_api.TransactionSchemaResponse{
		Code:   wallet_api.ApiReturnCode_APISUCCESS,
		Msg:    "success",
		Schema: schema,
	}, nil
}

// BuildUnSignTransaction 构建未签名交易
func (c SubstrateChainAdaptor) BuildUnSignTransaction(ctx context.Context, request *wallet_api.UnSignTransactionRequest) (*wallet_api.UnSignTransactionResponse, error) {
	var unsignTxnRet []*wallet_api.UnsignedTransactionMessageHash
	for _, unsignedTxItem := range request.Base64Txn {
		result, err := c.buildSubstrateUnSignTx(unsignedTxItem.Base64Tx)
		if err != nil {
			log.Error("buildSubstrateUnSignTx failed", "err", err)
			unsignTxnRet = append(unsignTxnRet, &wallet_api.UnsignedTransactionMessageHash{UnsignedTx: ""})
		} else {
			unsignTxnRet = append(unsignTxnRet, &wallet_api.UnsignedTransactionMessageHash{UnsignedTx: result})
		}
	}
	return &wallet_api.UnSignTransactionResponse{
		Code:        wallet_api.ApiReturnCode_APISUCCESS,
		Msg:         "build unsign transaction success",
		UnsignedTxn: unsignTxnRet,
	}, nil
}

// BuildSignedTransaction 构建已签名交易
func (c SubstrateChainAdaptor) BuildSignedTransaction(ctx context.Context, request *wallet_api.SignedTransactionRequest) (*wallet_api.SignedTransactionResponse, error) {
	var signedTransactionList []*wallet_api.SignedTxWithHash
	for _, txWithSignature := range request.TxnWithSignature {
		signedTx, err := c.buildSubstrateSignedTx(txWithSignature.Base64Tx, txWithSignature.Signature, txWithSignature.PublicKey)
		if err != nil {
			log.Error("buildSubstrateSignedTx failed", "err", err)
			signedTransactionList = append(signedTransactionList, &wallet_api.SignedTxWithHash{
				IsSuccess: false, SignedTx: "", TxHash: "",
			})
		} else {
			signedTransactionList = append(signedTransactionList, signedTx)
		}
	}
	return &wallet_api.SignedTransactionResponse{
		Code:      wallet_api.ApiReturnCode_APISUCCESS,
		Msg:       "build signed transaction success",
		SignedTxn: signedTransactionList,
	}, nil
}

func (c SubstrateChainAdaptor) GetAddressApproveList(ctx context.Context, request *wallet_api.AddressApproveListRequest) (*wallet_api.AddressApproveListResponse, error) {
	return &wallet_api.AddressApproveListResponse{
		Code: wallet_api.ApiReturnCode_APIERROR,
		Msg:  "substrate mode does not support approve list, use proxy query instead",
	}, nil
}

// buildSubstrateUnSignTx 构建未签名交易
func (c SubstrateChainAdaptor) buildSubstrateUnSignTx(base64Tx string) (string, error) {
	txJsonBytes, err := parseBase64Tx(base64Tx)
	if err != nil {
		return "", err
	}

	var subTx substratebase.SubstrateUnSignTx
	if err := json.Unmarshal(txJsonBytes, &subTx); err != nil {
		return "", fmt.Errorf("parse substrate tx json fail: %w", err)
	}

	meta, err := c.substrateClient.GetMetadataLatest()
	if err != nil {
		return "", fmt.Errorf("get metadata fail: %w", err)
	}

	genesisHash, err := c.substrateClient.GetGenesisHash()
	if err != nil {
		return "", fmt.Errorf("get genesis hash fail: %w", err)
	}

	runtime, err := c.substrateClient.GetRuntimeVersionLatest()
	if err != nil {
		return "", fmt.Errorf("get runtime version fail: %w", err)
	}

	toAddress, err := parseSubstrateToAddress(subTx.ToAddress)
	if err != nil {
		return "", err
	}

	amountInt, _ := strconv.ParseUint(subTx.Amount, 10, 64)
	builder := substratebase.NewExtrinsicBuilder(meta, genesisHash, runtime)
	call, err := builder.BuildTransferAllowDeathCall(meta, toAddress, types.NewUCompactFromUInt(amountInt))
	if err != nil {
		return "", fmt.Errorf("build transfer call fail: %w", err)
	}

	era := substratebase.ParseEra(subTx.Era)
	blockHash := parseBlockHash(subTx.BlockHash, genesisHash)

	signingPayload, err := builder.BuildSigningPayload(call, era, subTx.Nonce, subTx.Tip, blockHash)
	if err != nil {
		return "", fmt.Errorf("build signing payload fail: %w", err)
	}

	payloadHash := substratebase.HashSigningPayload(signingPayload)
	return "0x" + hex.EncodeToString(payloadHash[:]), nil
}

// buildSubstrateSignedTx 构建已签名的 Substrate 交易。
// 将 Base64 编码的交易 JSON、签名和公钥组装成完整的 ExtrinsicSignature，
// 编码为十六进制字符串并计算交易哈希，返回签名交易及其哈希。
func (c SubstrateChainAdaptor) buildSubstrateSignedTx(base64Tx string, signatureHex string, publicKeyHex string) (*wallet_api.SignedTxWithHash, error) {
	txJsonBytes, err := parseBase64Tx(base64Tx)
	if err != nil {
		return nil, err
	}

	var subTx substratebase.SubstrateSignedTx
	if err := json.Unmarshal(txJsonBytes, &subTx); err != nil {
		return nil, fmt.Errorf("parse substrate signed tx json fail: %w", err)
	}

	meta, err := c.substrateClient.GetMetadataLatest()
	if err != nil {
		return nil, fmt.Errorf("get metadata fail: %w", err)
	}

	genesisHash, err := c.substrateClient.GetGenesisHash()
	if err != nil {
		return nil, fmt.Errorf("get genesis hash fail: %w", err)
	}

	runtime, err := c.substrateClient.GetRuntimeVersionLatest()
	if err != nil {
		return nil, fmt.Errorf("get runtime version fail: %w", err)
	}

	toAddress, err := parseSubstrateToAddress(subTx.ToAddress)
	if err != nil {
		return nil, err
	}

	amountInt, _ := strconv.ParseUint(subTx.Amount, 10, 64)
	builder := substratebase.NewExtrinsicBuilder(meta, genesisHash, runtime)
	call, err := builder.BuildTransferAllowDeathCall(meta, toAddress, types.NewUCompactFromUInt(amountInt))
	if err != nil {
		return nil, fmt.Errorf("build transfer call fail: %w", err)
	}

	ext := builder.BuildExtrinsic(call)

	era := substratebase.ParseEra(subTx.Era)
	_ = parseBlockHash(subTx.BlockHash, genesisHash)

	pubKeyBytes, err := hex.DecodeString(strings.TrimPrefix(publicKeyHex, "0x"))
	if err != nil {
		return nil, fmt.Errorf("decode public key fail: %w", err)
	}

	accountID, err := types.NewAccountID(pubKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("create account id fail: %w", err)
	}

	signer, err := types.NewMultiAddressFromAccountID(accountID[:])
	if err != nil {
		return nil, fmt.Errorf("create multi address fail: %w", err)
	}

	sigBytes, err := hex.DecodeString(strings.TrimPrefix(signatureHex, "0x"))
	if err != nil {
		return nil, fmt.Errorf("decode signature fail: %w", err)
	}
	if len(sigBytes) != 64 {
		return nil, fmt.Errorf("invalid sr25519 signature length: %d", len(sigBytes))
	}

	ext.Signature = types.ExtrinsicSignatureV4{
		Signer:    signer,
		Signature: types.MultiSignature{IsSr25519: true, AsSr25519: types.NewSignature(sigBytes)},
		Era:       era,
		Nonce:     types.NewUCompactFromUInt(subTx.Nonce),
		Tip:       types.NewUCompactFromUInt(subTx.Tip),
	}
	ext.Version |= types.ExtrinsicBitSigned

	extHex, err := encodeExtrinsicToHex(ext)
	if err != nil {
		return nil, fmt.Errorf("encode extrinsic fail: %w", err)
	}

	extHash, err := blake2bHashExtrinsic(ext)
	if err != nil {
		return nil, fmt.Errorf("hash extrinsic fail: %w", err)
	}

	return &wallet_api.SignedTxWithHash{
		IsSuccess: true,
		SignedTx:  extHex,
		TxHash:    extHash,
	}, nil
}

// parseBase64Tx 将 Base64 编码的交易字符串解码为原始 JSON 字节。
func parseBase64Tx(base64Tx string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(base64Tx)
}

// parseSubstrateToAddress 将 SS58 格式的地址解码并转换为 types.MultiAddress。
// 先 SS58 解码为公钥十六进制，再构造 AccountID 和 MultiAddress。
func parseSubstrateToAddress(ss58Address string) (types.MultiAddress, error) {
	pubKeyHex, err := substratebase.SS58DecodeToHex(ss58Address)
	if err != nil {
		return types.MultiAddress{}, fmt.Errorf("invalid substrate to address: %w", err)
	}

	accountID, err := types.NewAccountIDFromHexString(pubKeyHex)
	if err != nil {
		return types.MultiAddress{}, fmt.Errorf("create account id fail: %w", err)
	}

	addr, err := types.NewMultiAddressFromAccountID(accountID[:])
	if err != nil {
		return types.MultiAddress{}, fmt.Errorf("create multi address fail: %w", err)
	}
	return addr, nil
}

// parseBlockHash 解析区块哈希字符串，如果为空或无效则返回创世区块哈希作为默认值。
func parseBlockHash(blockHashStr string, genesisHash types.Hash) types.Hash {
	if blockHashStr == "" {
		return genesisHash
	}
	blockHashBytes, err := hex.DecodeString(strings.TrimPrefix(blockHashStr, "0x"))
	if err != nil || len(blockHashBytes) != 32 {
		return genesisHash
	}
	var blockHash types.Hash
	copy(blockHash[:], blockHashBytes)
	return blockHash
}

// encodeExtrinsicToHex 将 Extrinsic 进行 SCALE 编码后返回带 "0x" 前缀的十六进制字符串。
func encodeExtrinsicToHex(ext types.Extrinsic) (string, error) {
	var buf bytes.Buffer
	encoder := scale.NewEncoder(&buf)
	if err := encoder.Encode(ext); err != nil {
		return "", fmt.Errorf("encode extrinsic fail: %w", err)
	}
	return "0x" + hex.EncodeToString(buf.Bytes()), nil
}

// blake2bHashExtrinsic 计算 Extrinsic 的 Blake2b-256 哈希值。
// 按照规范编码 version + signature(如有) + method，加上长度前缀后做哈希，
// 返回带 "0x" 前缀的十六进制哈希字符串，用作交易唯一标识。
func blake2bHashExtrinsic(ext types.Extrinsic) (string, error) {
	var body bytes.Buffer
	bodyEnc := scale.NewEncoder(&body)

	err := bodyEnc.Encode(ext.Version)
	if err != nil {
		return "", fmt.Errorf("encode extrinsic version fail: %w", err)
	}

	if ext.IsSigned() {
		err = bodyEnc.Encode(ext.Signature)
		if err != nil {
			return "", fmt.Errorf("encode extrinsic signature fail: %w", err)
		}
	}

	err = bodyEnc.Encode(ext.Method)
	if err != nil {
		return "", fmt.Errorf("encode extrinsic method fail: %w", err)
	}

	bodyBytes := body.Bytes()

	var result bytes.Buffer
	resultEnc := scale.NewEncoder(&result)
	err = resultEnc.EncodeUintCompact(*big.NewInt(0).SetUint64(uint64(len(bodyBytes))))
	if err != nil {
		return "", fmt.Errorf("encode length prefix fail: %w", err)
	}
	result.Write(bodyBytes)

	hash := substratebase.Blake2b256(result.Bytes())
	return "0x" + hex.EncodeToString(hash[:]), nil
}

func (c SubstrateChainAdaptor) parseExtrinsicBasic(ext types.Extrinsic, meta *types.Metadata) *wallet_api.TransactionList {
	txItem := c.newBaseTransaction(ext)

	if meta == nil {
		return txItem
	}

	toAddress, amount, txType := c.decodeCallFromArgs(ext, meta)
	if toAddress != "" {
		txItem.To = append(txItem.To, &wallet_api.ToAddress{
			Address: toAddress,
			Amount:  amount,
		})
		if amount != "" && len(txItem.From) > 0 {
			txItem.From[0].Amount = amount
		}
	}
	if txType > 0 {
		txItem.TxType = txType
	}

	return txItem
}

func (c SubstrateChainAdaptor) decodeCallFromArgs(ext types.Extrinsic, meta *types.Metadata) (string, string, uint32) {
	// 获取正确的 Balances.transfer CallIndex
	balancesTransferIndex, _ := meta.FindCallIndex("Balances.transfer")
	callIndex := ext.Method.CallIndex

	// 修复：如果检测到 System.remark 但可能是 Balances.transfer，直接使用 Balances 的 CallIndex
	if callIndex.SectionIndex == 0 && callIndex.MethodIndex == balancesTransferIndex.MethodIndex {
		callIndex.SectionIndex = 10 // Balances 模块的 index 是 10
	}

	callName := lookupCallName(meta, callIndex)

	if callName == "" {
		callName = lookupCallNameByFindCallIndex(meta, callIndex)
	}

	if callName == "" {
		return "", "", 0
	}

	switch {
	case strings.HasPrefix(callName, "Balances.transfer"),
		strings.HasPrefix(callName, "balances.transfer"):
		// 从原始 extrinsic 字节解析参数
		toAddr, amt, err := c.decodeTransferArgsFromRawExtrinsic(ext)
		if err != nil {
			toAddr, amt = decodeBalanceTransferArgs(ext.Method.Args, c.ss58Prefix)
		}
		return toAddr, amt, 1
	case strings.HasPrefix(callName, "Revive.eth_transact"),
		strings.HasPrefix(callName, "revive.eth_transact"),
		strings.HasPrefix(callName, "Revive.call"),
		strings.HasPrefix(callName, "revive.call"):
		return "", "", 2
	default:
		return "", "", 0
	}
}

// decodeTransferArgsFromRawExtrinsic  extrinsic 字节解析转账参数
func (c SubstrateChainAdaptor) decodeTransferArgsFromRawExtrinsic(ext types.Extrinsic) (string, string, error) {
	// 使用 MarshalJSON 获取原始的 extrinsic 十六进制字符串
	hexBytes, err := ext.MarshalJSON()
	if err != nil {
		return "", "", fmt.Errorf("marshal extrinsic fail: %w", err)
	}

	var hexStr string
	if err := json.Unmarshal(hexBytes, &hexStr); err != nil {
		return "", "", fmt.Errorf("unmarshal hex string fail: %w", err)
	}

	// 移除 0x 前缀并解码为字节
	rawBytes, err := hex.DecodeString(strings.TrimPrefix(hexStr, "0x"))
	if err != nil {
		return "", "", fmt.Errorf("decode hex fail: %w", err)
	}
	if len(rawBytes) == 0 {
		return "", "", fmt.Errorf("raw bytes is empty")
	}

	// 创建解码器
	decoder := scale.NewDecoder(bytes.NewReader(rawBytes))

	// 跳过长度前缀
	_, err = decoder.DecodeUintCompact()
	if err != nil {
		return "", "", fmt.Errorf("decode length prefix fail: %w", err)
	}

	// 读取版本字节
	var version byte
	if err := decoder.Decode(&version); err != nil {
		return "", "", fmt.Errorf("decode version fail: %w", err)
	}

	// 如果是签名交易，跳过签名部分
	if version&0x80 != 0 {
		// 跳过签名者地址
		var signer types.MultiAddress
		if err := decoder.Decode(&signer); err != nil {
			return "", "", fmt.Errorf("decode signer fail: %w", err)
		}

		// 跳过签名
		var sig types.MultiSignature
		if err := decoder.Decode(&sig); err != nil {
			return "", "", fmt.Errorf("decode signature fail: %w", err)
		}

		// 跳过 era
		var era types.ExtrinsicEra
		if err := decoder.Decode(&era); err != nil {
			return "", "", fmt.Errorf("decode era fail: %w", err)
		}

		// 跳过 nonce
		_, err = decoder.DecodeUintCompact()
		if err != nil {
			return "", "", fmt.Errorf("decode nonce fail: %w", err)
		}

		// 跳过 tip
		_, err = decoder.DecodeUintCompact()
		if err != nil {
			return "", "", fmt.Errorf("decode tip fail: %w", err)
		}
	}

	// 读取 CallIndex
	var callIndex types.CallIndex
	if err := decoder.Decode(&callIndex.SectionIndex); err != nil {
		return "", "", fmt.Errorf("decode section index fail: %w", err)
	}
	if err := decoder.Decode(&callIndex.MethodIndex); err != nil {
		return "", "", fmt.Errorf("decode method index fail: %w", err)
	}

	// 现在开始解析转账参数：dest, value
	// 对于 Balances.transfer_allow_death，参数是 (dest: MultiAddress, value: Compact<Balance>)

	// 先尝试解析 MultiAddress
	var dest types.MultiAddress
	if err := decoder.Decode(&dest); err != nil {
		// 如果 MultiAddress 解析失败，尝试直接解析 AccountID
		// 创建新的解码器
		decoder2 := scale.NewDecoder(bytes.NewReader(rawBytes))
		// 重新跳过前面的部分
		decoder2.DecodeUintCompact() // length
		decoder2.Decode(&version)    // version
		if version&0x80 != 0 {
			var signer types.MultiAddress
			decoder2.Decode(&signer)
			var sig types.MultiSignature
			decoder2.Decode(&sig)
			var era types.ExtrinsicEra
			decoder2.Decode(&era)
			decoder2.DecodeUintCompact() // nonce
			decoder2.DecodeUintCompact() // tip
		}
		// 重新读取 CallIndex
		var callIndex2 types.CallIndex
		decoder2.Decode(&callIndex2.SectionIndex)
		decoder2.Decode(&callIndex2.MethodIndex)

		// 尝试解析 AccountID
		var accountId types.AccountID
		if err := decoder2.Decode(&accountId); err != nil {
			return "", "", fmt.Errorf("decode accountId fail: %w", err)
		}
		// 转换为 MultiAddress
		dest = types.MultiAddress{IsID: true, AsID: accountId}
	}

	// 解析金额
	var value types.UCompact
	if err := decoder.Decode(&value); err != nil {
		return "", "", fmt.Errorf("decode value fail: %w", err)
	}

	// 转换地址和金额
	toAddr, err := multiAddressToAddress(dest, c.ss58Prefix)
	if err != nil {
		// 即使地址转换失败，也要返回金额
		amount := (*big.Int)(&value).String()
		return "", amount, nil
	}

	amount := (*big.Int)(&value).String()
	return toAddr, amount, nil
}

func lookupCallNameByFindCallIndex(meta *types.Metadata, callIndex types.CallIndex) string {
	knownCalls := []string{
		"Balances.transfer",
		"Balances.transfer_keep_alive",
		"Balances.transfer_allow_death",
		"Balances.transfer_all",
		"Revive.eth_transact",
		"Revive.call",
	}
	for _, name := range knownCalls {
		idx, err := meta.FindCallIndex(name)
		if err != nil {
			continue
		}
		if idx.SectionIndex == callIndex.SectionIndex && idx.MethodIndex == callIndex.MethodIndex {
			return name
		}
	}
	return ""
}

func lookupCallName(meta *types.Metadata, callIndex types.CallIndex) string {
	if meta.Version == 14 {
		for _, pallet := range meta.AsMetadataV14.Pallets {
			if !pallet.HasCalls {
				continue
			}
			if uint8(pallet.Index) != callIndex.SectionIndex {
				continue
			}
			callType := pallet.Calls.Type.Int64()
			typ, ok := meta.AsMetadataV14.EfficientLookup[callType]
			if !ok || len(typ.Def.Variant.Variants) == 0 {
				continue
			}
			for _, vars := range typ.Def.Variant.Variants {
				if uint8(vars.Index) == callIndex.MethodIndex {
					return string(pallet.Name) + "." + string(vars.Name)
				}
			}
		}
	}
	return ""
}

func decodeBalanceTransferArgs(args types.Args, ss58Prefix uint8) (string, string) {
	if len(args) == 0 {
		return "", ""
	}

	// 方法1: 直接解析 dest 和 value
	decoder := scale.NewDecoder(bytes.NewReader(args))
	var dest types.MultiAddress
	var value types.UCompact

	if err := decoder.Decode(&dest); err == nil {
		if err := decoder.Decode(&value); err == nil {
			amount := (*big.Int)(&value).String()
			toAddr, _ := multiAddressToAddress(dest, ss58Prefix)
			return toAddr, amount
		} else {
		}
	} else {
	}

	// 方法2: 直接解析 AccountID 和 value
	decoder = scale.NewDecoder(bytes.NewReader(args))
	var accountId types.AccountID
	var value2 types.UCompact

	if err := decoder.Decode(&accountId); err == nil {
		if err := decoder.Decode(&value2); err == nil {
			amount := (*big.Int)(&value2).String()
			// 将 AccountID 转换为 MultiAddress
			dest := types.MultiAddress{IsID: true, AsID: accountId}
			toAddr, _ := multiAddressToAddress(dest, ss58Prefix)
			return toAddr, amount
		} else {
		}
	} else {
	}

	// 方法3: 跳过前32字节（AccountID）直接解析金额
	if len(args) > 32 {
		amountBytes := args[32:]
		decoder2 := scale.NewDecoder(bytes.NewReader(amountBytes))
		var value3 types.UCompact
		if err := decoder2.Decode(&value3); err == nil {
			amount := (*big.Int)(&value3).String()
			// 尝试从 args 解析地址
			addrDecoder := scale.NewDecoder(bytes.NewReader(args[:32]))
			var addrAccountId types.AccountID
			if err := addrDecoder.Decode(&addrAccountId); err == nil {
				dest := types.MultiAddress{IsID: true, AsID: addrAccountId}
				toAddr, _ := multiAddressToAddress(dest, ss58Prefix)
				return toAddr, amount
			} else {
			}
			return "", amount
		} else {
		}
	}

	// 方法4: 尝试解析为原始大整数（仅作为最后的尝试）
	if len(args) > 32 {
		amountBytes := args[32:]
		var amtBig big.Int
		amtBig.SetBytes(amountBytes)
		amount := amtBig.String()
		return "", amount
	}

	return "", ""
}

// parseExtrinsicToTransaction 是结构化解析路径：优先按 metadata 里的调用定义读取参数。
func (c SubstrateChainAdaptor) parseExtrinsicToTransaction(ext types.Extrinsic, callRegistry registry.CallRegistry) (*wallet_api.TransactionList, error) {
	txItem := c.newBaseTransaction(ext)

	callName, err := lookupCallNameFromRegistry(ext.Method, callRegistry)
	if err != nil {
		return txItem, err
	}

	if isReviveCall(callName) {
		txItem.TxType = 2
		return txItem, nil
	}

	if !isStructuredTransactionCall(callName) {
		return txItem, nil
	}

	callFields, err := decodeExtrinsicCallFields(ext.Method, callRegistry)
	if err != nil {
		return txItem, err
	}

	callInfo, err := c.extractTransactionInfoFromCall(callName, callFields)
	if err != nil {
		return txItem, err
	}

	applyCallTransactionInfo(txItem, callInfo)

	return txItem, nil
}

// newBaseTransaction 构造交易的公共基础字段，供不同解析路径复用。
func (c SubstrateChainAdaptor) newBaseTransaction(ext types.Extrinsic) *wallet_api.TransactionList {
	var fromList []*wallet_api.FromAddress

	if ext.IsSigned() {
		if fromAddr, err := c.signerToAddress(ext.Signature.Signer); err == nil {
			fromList = append(fromList, &wallet_api.FromAddress{Address: fromAddr})
		}
	}

	extHash := ""
	if h, err := blake2bHashExtrinsic(ext); err == nil {
		extHash = h
	}

	return &wallet_api.TransactionList{
		TxHash: extHash,
		From:   fromList,
		Status: uint32(wallet_api.TxStatus_Success),
	}
}

func applyCallTransactionInfo(txItem *wallet_api.TransactionList, callInfo *callTransactionInfo) {
	if txItem == nil || callInfo == nil {
		return
	}

	if callInfo.txType != 0 {
		txItem.TxType = callInfo.txType
	}

	if callInfo.fromAmount != "" && len(txItem.From) > 0 {
		txItem.From[0].Amount = callInfo.fromAmount
	}

	txItem.To = append(txItem.To, callInfo.to...)

	if len(callInfo.actions) > 0 {
		applyTransactionCallSummaryMeta(txItem, callInfo.actions, "call")
	}
}

func applyTransactionCallSummaryMeta(txItem *wallet_api.TransactionList, actions []string, source string) {
	if txItem == nil || len(actions) == 0 {
		return
	}

	metaTarget := pickTransactionMetaTarget(txItem)
	if metaTarget == nil {
		return
	}

	meta := transactionCallSummaryMeta{}
	if rawMeta := getTransactionMetaData(metaTarget); rawMeta != "" {
		_ = json.Unmarshal([]byte(rawMeta), &meta)
	}

	meta.CallSummary = appendUniqueStrings(meta.CallSummary, actions...)
	if source != "" {
		meta.SummarySources = appendUniqueStrings(meta.SummarySources, source)
	}

	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return
	}

	setTransactionMetaData(metaTarget, string(metaBytes))
}

func pickTransactionMetaTarget(txItem *wallet_api.TransactionList) any {
	if txItem == nil {
		return nil
	}

	if len(txItem.From) > 0 && txItem.From[0] != nil {
		return txItem.From[0]
	}

	if len(txItem.To) > 0 && txItem.To[0] != nil {
		return txItem.To[0]
	}

	return nil
}

func getTransactionMetaData(target any) string {
	switch value := target.(type) {
	case *wallet_api.FromAddress:
		if value == nil {
			return ""
		}
		return value.MetaData
	case *wallet_api.ToAddress:
		if value == nil {
			return ""
		}
		return value.MetaData
	default:
		return ""
	}
}

func setTransactionMetaData(target any, value string) {
	switch target := target.(type) {
	case *wallet_api.FromAddress:
		if target != nil {
			target.MetaData = value
		}
	case *wallet_api.ToAddress:
		if target != nil {
			target.MetaData = value
		}
	}
}

func lookupCallNameFromRegistry(call types.Call, callRegistry registry.CallRegistry) (string, error) {
	callDecoder, ok := callRegistry[call.CallIndex]
	if !ok {
		return "", fmt.Errorf("call decoder not found for call index %d:%d", call.CallIndex.SectionIndex, call.CallIndex.MethodIndex)
	}

	return callDecoder.Name, nil
}

func decodeExtrinsicCallFields(call types.Call, callRegistry registry.CallRegistry) (registry.DecodedFields, error) {
	callDecoder, ok := callRegistry[call.CallIndex]
	if !ok {
		return nil, fmt.Errorf("call decoder not found for call index %d:%d", call.CallIndex.SectionIndex, call.CallIndex.MethodIndex)
	}

	decoder := scale.NewDecoder(bytes.NewReader(call.Args))
	callFields, err := callDecoder.Decode(decoder)
	if err != nil {
		return nil, fmt.Errorf("decode call fields fail: %w", err)
	}

	return callFields, nil
}

func isStructuredTransactionCall(callName string) bool {
	return isBalanceTransferCall(callName) || isUtilityBatchCall(callName)
}

func isBalanceTransferCall(callName string) bool {
	switch callName {
	case "Balances.transfer", "Balances.transfer_keep_alive", "Balances.transfer_allow_death", "Balances.transfer_all":
		return true
	default:
		return false
	}
}

func isUtilityBatchCall(callName string) bool {
	switch callName {
	case "Utility.batch", "Utility.batch_all", "Utility.force_batch":
		return true
	default:
		return false
	}
}

func isReviveCall(callName string) bool {
	switch callName {
	case "Revive.eth_transact", "Revive.call":
		return true
	default:
		return false
	}
}

// extractTransactionInfoFromCall 负责把不同 pallet 的 call 参数，收敛成统一交易信息。
func (c SubstrateChainAdaptor) extractTransactionInfoFromCall(callName string, callFields registry.DecodedFields) (*callTransactionInfo, error) {
	switch callName {
	case "Balances.transfer", "Balances.transfer_keep_alive", "Balances.transfer_allow_death", "Balances.transfer_all":
		return c.extractBalanceTransferInfo(callName, callFields)
	case "Utility.batch", "Utility.batch_all", "Utility.force_batch":
		return c.extractUtilityBatchInfo(callFields), nil
	default:
		return nil, fmt.Errorf("unsupported call: %s", callName)
	}
}

// extractBalanceTransferInfo 处理 Balances pallet 下的直接转账调用。
func (c SubstrateChainAdaptor) extractBalanceTransferInfo(callName string, callFields registry.DecodedFields) (*callTransactionInfo, error) {
	toAddress, err := extractDecodedAddress(callFields, c.ss58Prefix, -1, "dest", "to")
	if err != nil {
		return nil, err
	}

	info := &callTransactionInfo{
		txType: 1,
		to: []*wallet_api.ToAddress{
			{Address: toAddress},
		},
	}

	amount, amountErr := extractDecodedAmount(callFields, -1, "value", "amount")
	if amountErr == nil {
		info.fromAmount = amount
		info.to[0].Amount = amount
	}

	if callName != "Balances.transfer_all" && amountErr != nil {
		return nil, amountErr
	}

	return info, nil
}

// extractUtilityBatchInfo 会继续向 Utility.batch/batch_all 的 calls 里递归展开，
func (c SubstrateChainAdaptor) extractUtilityBatchInfo(callFields registry.DecodedFields) *callTransactionInfo {
	nestedCalls := extractNestedCallFields(callFields)
	if len(nestedCalls) == 0 {
		return nil
	}

	aggregated := &callTransactionInfo{}
	for _, nestedCallFields := range nestedCalls {
		nestedInfo := c.extractTransferInfoFromNestedCall(nestedCallFields)
		mergeCallTransactionInfo(aggregated, nestedInfo)
	}

	if aggregated.txType == 0 && aggregated.fromAmount == "" && len(aggregated.to) == 0 && len(aggregated.actions) == 0 {
		return nil
	}

	return aggregated
}

// extractTransferInfoFromNestedCall 负责处理 Utility.batch/batch_all
func (c SubstrateChainAdaptor) extractTransferInfoFromNestedCall(callFields registry.DecodedFields) *callTransactionInfo {
	toAddress, err := extractDecodedAddress(callFields, c.ss58Prefix, -1, "dest", "to", "recipient")
	if err == nil && (hasDecodedField(callFields, "value", "amount", "balance") || hasDecodedField(callFields, "keep_alive", "keepAlive")) {
		info := &callTransactionInfo{
			txType: 1,
			to: []*wallet_api.ToAddress{
				{Address: toAddress},
			},
		}

		if amount, amountErr := extractDecodedAmount(callFields, -1, "value", "amount"); amountErr == nil {
			info.fromAmount = amount
			info.to[0].Amount = amount
		}

		return info
	}

	aggregated := &callTransactionInfo{}
	for _, nestedCallFields := range extractNestedCallFields(callFields) {
		mergeCallTransactionInfo(aggregated, c.extractTransferInfoFromNestedCall(nestedCallFields))
	}

	if aggregated.txType == 0 && aggregated.fromAmount == "" && len(aggregated.to) == 0 && len(aggregated.actions) == 0 {
		return nil
	}

	return aggregated
}

// extractNestedCallFields 从 call/calls 这类包装字段中继续搜集内部调用。
// gsrpc 对嵌套 Call 的解码结果通常是 DecodedFields/[]any 的组合
func extractNestedCallFields(callFields registry.DecodedFields) []registry.DecodedFields {
	var nested []registry.DecodedFields

	for _, field := range callFields {
		if field == nil {
			continue
		}
		if !matchesDecodedFieldName(field.Name, "call") && !matchesDecodedFieldName(field.Name, "calls") {
			continue
		}
		collectNestedCallFields(field.Value, &nested)
	}

	return nested
}

func collectNestedCallFields(value any, nested *[]registry.DecodedFields) {
	switch v := value.(type) {
	case registry.DecodedFields:
		*nested = append(*nested, v)
		for _, field := range v {
			if field == nil {
				continue
			}
			collectNestedCallFields(field.Value, nested)
		}
		return
	case *registry.DecodedField:
		if v == nil {
			return
		}
		collectNestedCallFields(v.Value, nested)
		return
	}

	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return
	}

	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return
		}
		collectNestedCallFields(rv.Elem().Interface(), nested)
		return
	}

	if rv.Kind() != reflect.Array && rv.Kind() != reflect.Slice {
		return
	}

	for i := 0; i < rv.Len(); i++ {
		collectNestedCallFields(rv.Index(i).Interface(), nested)
	}
}

func hasDecodedField(callFields registry.DecodedFields, names ...string) bool {
	return findDecodedField(callFields, -1, names...) != nil
}

func mergeCallTransactionInfo(dst, src *callTransactionInfo) {
	if dst == nil || src == nil {
		return
	}

	if dst.txType == 0 && src.txType != 0 {
		dst.txType = src.txType
	}

	if src.fromAmount != "" {
		dst.fromAmount = sumAmountStrings(dst.fromAmount, src.fromAmount)
	}

	dst.to = append(dst.to, src.to...)
	dst.actions = appendUniqueStrings(dst.actions, src.actions...)
}

func appendUniqueStrings(dst []string, values ...string) []string {
	seen := make(map[string]struct{}, len(dst))
	for _, item := range dst {
		if item == "" {
			continue
		}
		seen[item] = struct{}{}
	}

	for _, item := range values {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		dst = append(dst, item)
		seen[item] = struct{}{}
	}

	return dst
}

func summarizeNonTransferActionFromEvent(eventName string) (string, bool) {
	switch eventName {
	case "System.ExtrinsicSuccess", "System.ExtrinsicFailed", "System.NewAccount", "System.KilledAccount",
		"TransactionPayment.TransactionFeePaid",
		"Utility.ItemCompleted", "Utility.BatchCompleted", "Utility.BatchCompletedWithErrors", "Utility.BatchInterrupted",
		"Balances.Withdraw", "Balances.Deposit", "Balances.Endowed":
		return "", false
	case "Staking.Chilled":
		return "Staking.chill", true
	case "Staking.Unbonded":
		return "Staking.unbond", true
	case "Staking.Bonded":
		return "Staking.bond", true
	case "Staking.Withdrawn":
		return "Staking.withdraw_unbonded", true
	default:
		moduleName, itemName, ok := splitQualifiedName(eventName)
		if !ok {
			return "", false
		}
		return moduleName + "." + camelToSnake(itemName), true
	}
}

func splitQualifiedName(name string) (string, string, bool) {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func camelToSnake(input string) string {
	if input == "" {
		return ""
	}

	var builder strings.Builder
	for index, r := range input {
		if index > 0 && r >= 'A' && r <= 'Z' {
			builder.WriteByte('_')
		}
		builder.WriteRune(r)
	}

	return strings.ToLower(builder.String())
}

func sumAmountStrings(left, right string) string {
	switch {
	case left == "":
		return right
	case right == "":
		return left
	}

	leftValue, ok := new(big.Int).SetString(left, 10)
	if !ok {
		return right
	}

	rightValue, ok := new(big.Int).SetString(right, 10)
	if !ok {
		return left
	}

	return leftValue.Add(leftValue, rightValue).String()
}

// extractDecodedAddress 兼容 call/event 中常见的地址表示形式：
// MultiAddress、AccountID、十六进制字符串、字节数组，以及递归包装后的复合结构。
func extractDecodedAddress(callFields registry.DecodedFields, ss58Prefix uint8, fallbackIndex int, fieldNames ...string) (string, error) {
	field := findDecodedField(callFields, fallbackIndex, fieldNames...)
	if field == nil {
		return "", fmt.Errorf("address field not found")
	}

	switch value := field.Value.(type) {
	case types.MultiAddress:
		return multiAddressToAddress(value, ss58Prefix)
	case *types.MultiAddress:
		if value == nil {
			return "", fmt.Errorf("address field is nil")
		}
		return multiAddressToAddress(*value, ss58Prefix)
	case types.AccountID:
		return substratebase.SS58Encode(value[:], ss58Prefix), nil
	case *types.AccountID:
		if value == nil {
			return "", fmt.Errorf("address field is nil")
		}
		return substratebase.SS58Encode(value[:], ss58Prefix), nil
	case string:
		return decodeFlexibleAddress(value, ss58Prefix)
	case []byte:
		return encodeFlexibleAddressBytes(value, ss58Prefix)
	case types.Bytes:
		return encodeFlexibleAddressBytes(value, ss58Prefix)
	default:
		if addr, ok := decodeFlexibleAddressValue(field.Value, ss58Prefix); ok {
			return addr, nil
		}
		return "", fmt.Errorf("unsupported address field type %T", field.Value)
	}
}

// decodeFlexibleAddressValue 用于处理 registry 解码后的“套壳”结果，
// 例如 DecodedFields、[]any、[u8;32] 等，最终统一还原成地址字节。
func decodeFlexibleAddressValue(value any, ss58Prefix uint8) (string, bool) {
	switch v := value.(type) {
	case registry.DecodedFields:
		for _, field := range v {
			if field == nil {
				continue
			}
			if addr, ok := decodeFlexibleAddressValue(field.Value, ss58Prefix); ok {
				return addr, true
			}
		}
		return "", false
	case *registry.DecodedField:
		if v == nil {
			return "", false
		}
		return decodeFlexibleAddressValue(v.Value, ss58Prefix)
	}

	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return "", false
	}

	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return "", false
		}
		return decodeFlexibleAddressValue(rv.Elem().Interface(), ss58Prefix)
	}

	switch rv.Kind() {
	case reflect.Array, reflect.Slice:
		if buf, ok := convertByteLikeValueToBytes(rv); ok {
			addr, err := encodeFlexibleAddressBytes(buf, ss58Prefix)
			return addr, err == nil
		}
		return "", false
	default:
		return "", false
	}
}

func convertByteLikeValueToBytes(rv reflect.Value) ([]byte, bool) {
	if !rv.IsValid() {
		return nil, false
	}

	if rv.Kind() != reflect.Array && rv.Kind() != reflect.Slice {
		return nil, false
	}

	buf := make([]byte, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		elem := rv.Index(i)
		if elem.Kind() == reflect.Interface || elem.Kind() == reflect.Pointer {
			if elem.IsNil() {
				return nil, false
			}
			elem = elem.Elem()
		}

		switch elem.Kind() {
		case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
			buf[i] = byte(elem.Uint())
		case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
			buf[i] = byte(elem.Int())
		default:
			return nil, false
		}
	}

	return buf, true
}

func decodeFlexibleAddress(value string, ss58Prefix uint8) (string, error) {
	if value == "" {
		return "", fmt.Errorf("address field is empty")
	}

	if strings.HasPrefix(value, "0x") {
		raw, err := hex.DecodeString(strings.TrimPrefix(value, "0x"))
		if err != nil {
			return "", err
		}
		return encodeFlexibleAddressBytes(raw, ss58Prefix)
	}

	return value, nil
}

func encodeFlexibleAddressBytes(raw []byte, ss58Prefix uint8) (string, error) {
	switch len(raw) {
	case 32:
		return substratebase.SS58Encode(raw, ss58Prefix), nil
	case 20:
		return "0x" + hex.EncodeToString(raw), nil
	default:
		return "", fmt.Errorf("unsupported address length %d", len(raw))
	}
}

func extractDecodedAmount(callFields registry.DecodedFields, fallbackIndex int, fieldNames ...string) (string, error) {
	field := findDecodedField(callFields, fallbackIndex, fieldNames...)
	if field == nil {
		return "", fmt.Errorf("amount field not found")
	}

	switch value := field.Value.(type) {
	case types.UCompact:
		v := big.Int(value)
		return v.String(), nil
	case *types.UCompact:
		if value == nil {
			return "", fmt.Errorf("amount field is nil")
		}
		v := big.Int(*value)
		return v.String(), nil
	case types.U128:
		if value.Int == nil {
			return "0", nil
		}
		return value.Int.String(), nil
	case *types.U128:
		if value == nil || value.Int == nil {
			return "0", nil
		}
		return value.Int.String(), nil
	case uint8:
		return strconv.FormatUint(uint64(value), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(value), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(value), 10), nil
	case uint64:
		return strconv.FormatUint(value, 10), nil
	case int:
		return strconv.FormatInt(int64(value), 10), nil
	case int32:
		return strconv.FormatInt(int64(value), 10), nil
	case int64:
		return strconv.FormatInt(value, 10), nil
	default:
		return "", fmt.Errorf("unsupported amount field type %T", field.Value)
	}
}

func findDecodedField(callFields registry.DecodedFields, fallbackIndex int, names ...string) *registry.DecodedField {
	for _, field := range callFields {
		for _, name := range names {
			if matchesDecodedFieldName(field.Name, name) {
				return field
			}
		}
	}

	if fallbackIndex >= 0 && fallbackIndex < len(callFields) {
		return callFields[fallbackIndex]
	}
	return nil
}

// 某些 runtime 会把字段名展开成带路径的形式，例如 `sp_core.crypto.AccountId32.to`。
// 这里兼容完整字段名和末尾字段名两种匹配方式。
func matchesDecodedFieldName(actual, expected string) bool {
	if strings.EqualFold(actual, expected) {
		return true
	}

	actualLower := strings.ToLower(actual)
	expectedLower := strings.ToLower(expected)

	if strings.HasSuffix(actualLower, "."+expectedLower) {
		return true
	}

	if idx := strings.LastIndex(actualLower, "."); idx >= 0 && idx+1 < len(actualLower) {
		return actualLower[idx+1:] == expectedLower
	}

	return false
}

func (c SubstrateChainAdaptor) signerToAddress(signer types.MultiAddress) (string, error) {
	return multiAddressToAddress(signer, c.ss58Prefix)
}

// getBlockExtrinsicEventSummaries 读取 System.Events，并按 extrinsic index 聚合成摘要。
func (c *SubstrateChainAdaptor) getBlockExtrinsicEventSummaries(blockHash types.Hash) (map[uint32]*extrinsicEventSummary, error) {
	meta, err := c.getMetadata()
	if err != nil {
		return nil, err
	}

	eventRegistry, err := c.getEventRegistry()
	if err != nil {
		return nil, err
	}

	return c.parseBlockExtrinsicEventSummaries(blockHash, meta, eventRegistry)
}

func (c SubstrateChainAdaptor) parseBlockExtrinsicEventSummaries(
	blockHash types.Hash,
	meta *types.Metadata,
	eventRegistry registry.EventRegistry,
) (map[uint32]*extrinsicEventSummary, error) {
	parsed, err := c.parseBlockEvents(blockHash, meta, eventRegistry)
	if err != nil {
		return nil, err
	}
	return parsed.summaries, nil
}

func (c SubstrateChainAdaptor) parseBlockEvents(
	blockHash types.Hash,
	meta *types.Metadata,
	eventRegistry registry.EventRegistry,
) (*parsedBlockEvents, error) {
	key, err := types.CreateStorageKey(meta, "System", "Events", nil)
	if err != nil {
		return nil, fmt.Errorf("create system events storage key fail: %w", err)
	}

	raw, err := c.substrateClient.GetStorageRaw(key, blockHash)
	if err != nil {
		return nil, fmt.Errorf("get system events storage fail: %w", err)
	}
	if raw == nil {
		return &parsedBlockEvents{summaries: make(map[uint32]*extrinsicEventSummary)}, nil
	}

	events, err := registryparser.NewEventParser().ParseEvents(eventRegistry, raw)
	if err != nil {
		return nil, fmt.Errorf("parse system events fail: %w", err)
	}

	summaries := make(map[uint32]*extrinsicEventSummary)
	blockTransactions := make([]*wallet_api.TransactionList, 0)

	for eventIndex, event := range events {
		if event == nil || event.Phase == nil {
			continue
		}

		if !event.Phase.IsApplyExtrinsic {
			if txItem := c.transactionFromBlockEvent(blockHash, eventIndex, event); txItem != nil {
				blockTransactions = append(blockTransactions, txItem)
			}
			continue
		}

		extrinsicIndex := event.Phase.AsApplyExtrinsic
		summary := summaries[extrinsicIndex]
		if summary == nil {
			summary = &extrinsicEventSummary{}
			summaries[extrinsicIndex] = summary
		}

		switch event.Name {
		case "System.ExtrinsicSuccess":
			status := wallet_api.TxStatus_Success
			summary.status = &status
		case "System.ExtrinsicFailed":
			status := wallet_api.TxStatus_Failed
			summary.status = &status
		case "TransactionPayment.TransactionFeePaid":
			if fee, feeErr := extractDecodedAmount(event.Fields, -1, "actual_fee", "actualFee", "fee"); feeErr == nil {
				summary.fee = fee
			}
		default:
			if !isTransferEventName(event.Name) {
				if action, ok := summarizeNonTransferActionFromEvent(event.Name); ok {
					summary.actions = appendUniqueStrings(summary.actions, action)
				}
				continue
			}

			transfer := extrinsicTransferEvent{}
			if from, fromErr := extractDecodedAddress(event.Fields, c.ss58Prefix, -1, "from", "who", "sender"); fromErr == nil {
				transfer.from = from
			}
			if to, toErr := extractDecodedAddress(event.Fields, c.ss58Prefix, -1, "to", "dest", "recipient"); toErr == nil {
				transfer.to = to
			}
			if amount, amountErr := extractDecodedAmount(event.Fields, -1, "value", "amount", "balance"); amountErr == nil {
				transfer.amount = amount
			}

			if transfer.from != "" || transfer.to != "" || transfer.amount != "" {
				summary.transfers = append(summary.transfers, transfer)
			}
		}
	}

	return &parsedBlockEvents{
		summaries:         summaries,
		blockTransactions: blockTransactions,
	}, nil
}

func (c SubstrateChainAdaptor) transactionFromBlockEvent(
	blockHash types.Hash,
	eventIndex int,
	event *registryparser.Event,
) *wallet_api.TransactionList {
	if event == nil || event.Phase == nil || !isTransferEventName(event.Name) {
		return nil
	}

	transfer := extrinsicTransferEvent{}
	if from, fromErr := extractDecodedAddress(event.Fields, c.ss58Prefix, -1, "from", "who", "sender"); fromErr == nil {
		transfer.from = from
	}
	if to, toErr := extractDecodedAddress(event.Fields, c.ss58Prefix, -1, "to", "dest", "recipient"); toErr == nil {
		transfer.to = to
	}
	if amount, amountErr := extractDecodedAmount(event.Fields, -1, "value", "amount", "balance"); amountErr == nil {
		transfer.amount = amount
	}
	if transfer.from == "" && transfer.to == "" && transfer.amount == "" {
		return nil
	}

	phase := eventPhaseName(event.Phase)
	return &wallet_api.TransactionList{
		TxHash: fmt.Sprintf("%s:event:%d", blockHash.Hex(), eventIndex),
		From: []*wallet_api.FromAddress{{
			Address:  transfer.from,
			Amount:   transfer.amount,
			MetaData: fmt.Sprintf("event:%s:%s", phase, event.Name),
		}},
		To: []*wallet_api.ToAddress{{
			Address:  transfer.to,
			Amount:   transfer.amount,
			MetaData: fmt.Sprintf("event:%s:%s", phase, event.Name),
		}},
		Status: uint32(wallet_api.TxStatus_Success),
		TxType: 1,
	}
}

func eventPhaseName(phase *types.Phase) string {
	if phase == nil {
		return "Unknown"
	}
	switch {
	case phase.IsInitialization:
		return "Initialization"
	case phase.IsFinalization:
		return "Finalization"
	case phase.IsApplyExtrinsic:
		return fmt.Sprintf("ApplyExtrinsic(%d)", phase.AsApplyExtrinsic)
	default:
		return "Unknown"
	}
}

// applyExtrinsicEventSummary 用事件结果补强交易数据。
func (c SubstrateChainAdaptor) applyExtrinsicEventSummary(txItem *wallet_api.TransactionList, summary *extrinsicEventSummary) {
	if txItem == nil || summary == nil {
		return
	}

	if summary.status != nil {
		txItem.Status = uint32(*summary.status)
	}

	if txItem.Fee == "" && summary.fee != "" {
		txItem.Fee = summary.fee
	}

	// 对 utility.batch_all 这类非转账调用
	if len(summary.actions) > 0 {
		applyTransactionCallSummaryMeta(txItem, summary.actions, "event")
	}

	if len(summary.transfers) == 0 {
		return
	}

	if txItem.TxType == 0 {
		txItem.TxType = 1
	}

	if len(txItem.From) == 0 && summary.transfers[0].from != "" {
		txItem.From = append(txItem.From, &wallet_api.FromAddress{Address: summary.transfers[0].from})
	}

	if len(txItem.From) > 0 && txItem.From[0].Amount == "" && summary.transfers[0].amount != "" {
		txItem.From[0].Amount = summary.transfers[0].amount
	}

	// 事件层通常拿到的是“最终结果”，因此这里采用合并策略：
	// 已经从 call 里解析出的 To 地址保留，再用事件补齐缺失的地址/金额；
	// 如果 call 层完全没识别出来，则直接按事件顺序追加。
	for i, transfer := range summary.transfers {
		if transfer.to == "" && transfer.amount == "" {
			continue
		}

		if i < len(txItem.To) && txItem.To[i] != nil {
			if txItem.To[i].Address == "" {
				txItem.To[i].Address = transfer.to
			}
			if txItem.To[i].Amount == "" {
				txItem.To[i].Amount = transfer.amount
			}
			continue
		}

		txItem.To = append(txItem.To, &wallet_api.ToAddress{
			Address: transfer.to,
			Amount:  transfer.amount,
		})
	}
}

func isTransferEventName(eventName string) bool {
	return strings.HasSuffix(eventName, ".Transfer") || strings.HasSuffix(eventName, ".Transferred")
}

func multiAddressToAddress(addr types.MultiAddress, ss58Prefix uint8) (string, error) {
	switch {
	case addr.IsID:
		return substratebase.SS58Encode(addr.AsID[:], ss58Prefix), nil
	case addr.IsAddress32:
		return substratebase.SS58Encode(addr.AsAddress32[:], ss58Prefix), nil
	case addr.IsAddress20:
		return "0x" + hex.EncodeToString(addr.AsAddress20[:]), nil
	case addr.IsIndex:
		return strconv.FormatUint(uint64(addr.AsIndex), 10), nil
	case addr.IsRaw:
		if len(addr.AsRaw) == 32 {
			return substratebase.SS58Encode(addr.AsRaw, ss58Prefix), nil
		}
		return "0x" + hex.EncodeToString(addr.AsRaw), nil
	default:
		return "", fmt.Errorf("unsupported multi address type")
	}
}
func (c *SubstrateChainAdaptor) getMetadata() (*types.Metadata, error) {
	rv, err := c.substrateClient.GetRuntimeVersionLatest()
	if err != nil {
		return nil, fmt.Errorf("get runtime version fail: %w", err)
	}

	// Metadata 会随着 runtime 升级变化，这里按 SpecVersion 做缓存。
	if c.metadata != nil && c.runtimeVersion.SpecVersion == rv.SpecVersion {
		return c.metadata, nil
	}

	meta, err := c.substrateClient.GetMetadataLatest()
	if err != nil {
		return nil, fmt.Errorf("get metadata fail: %w", err)
	}

	c.metadata = meta
	c.runtimeVersion = *rv
	return meta, nil
}

func (c *SubstrateChainAdaptor) getCallRegistry() (registry.CallRegistry, error) {
	rv, err := c.substrateClient.GetRuntimeVersionLatest()
	if err != nil {
		return nil, fmt.Errorf("get runtime version fail: %w", err)
	}

	// CallRegistry 依赖当前 runtime 的 metadata，版本不变时可直接复用。
	if c.callRegistry != nil && c.callRegistryRV.SpecVersion == rv.SpecVersion {
		return c.callRegistry, nil
	}

	meta, err := c.substrateClient.GetMetadataLatest()
	if err != nil {
		return nil, fmt.Errorf("get metadata fail: %w", err)
	}

	cr, err := registry.NewFactory().CreateCallRegistry(meta)
	if err != nil {
		return nil, fmt.Errorf("create call registry fail: %w", err)
	}

	c.callRegistry = cr
	c.callRegistryRV = *rv
	return cr, nil
}

func (c *SubstrateChainAdaptor) getEventRegistry() (registry.EventRegistry, error) {
	rv, err := c.substrateClient.GetRuntimeVersionLatest()
	if err != nil {
		return nil, fmt.Errorf("get runtime version fail: %w", err)
	}

	// EventRegistry 与 CallRegistry 一样，需要和当前 runtime 版本保持一致。
	if c.eventRegistry != nil && c.eventRegistryRV.SpecVersion == rv.SpecVersion {
		return c.eventRegistry, nil
	}

	meta, err := c.substrateClient.GetMetadataLatest()
	if err != nil {
		return nil, fmt.Errorf("get metadata fail: %w", err)
	}

	er, err := registry.NewFactory().CreateEventRegistry(meta)
	if err != nil {
		return nil, fmt.Errorf("create event registry fail: %w", err)
	}

	c.eventRegistry = er
	c.eventRegistryRV = *rv
	return er, nil
}
