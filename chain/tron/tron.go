package tron

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/dapplink-labs/dapplink-wallet-api/protobuf/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/fbsobreira/gotron-sdk/pkg/address"

	"github.com/dapplink-labs/dapplink-wallet-api/chain"
	"github.com/dapplink-labs/dapplink-wallet-api/config"
	"github.com/dapplink-labs/dapplink-wallet-api/protobuf/walletapi"
)

const (
	ChainID   string = "DappLinkTron"
	ChainName string = "Tron"

	justLendRentalQueryPrefix = "__justlend_rental__"
)

func parseJustLendRentalQuery(contractAddress string) (renter string, isRentalQuery bool) {
	trimmed := strings.TrimSpace(contractAddress)
	if !strings.HasPrefix(strings.ToLower(trimmed), justLendRentalQueryPrefix) {
		return "", false
	}
	if len(trimmed) > len(justLendRentalQueryPrefix)+1 && trimmed[len(justLendRentalQueryPrefix)] == '|' {
		return strings.TrimSpace(trimmed[len(justLendRentalQueryPrefix)+1:]), true
	}
	return "", true
}

type ChainAdaptor struct {
	tronClient     *TronClient
	tronDataClient *TronData
	sponsor        config.TronSponsor
}

func NewChainAdaptor(conf *config.Config) (chain.IChainAdaptor, error) {
	rpc := conf.WalletNode.Tron
	tronClient := DialTronClient(rpc.RpcUrl, rpc.RpcUser, rpc.RpcPass)
	tronDataClient, err := NewTronDataClient(conf.WalletNode.Tron.DataApiUrl, conf.WalletNode.Tron.DataApiKey, time.Second*15)
	if err != nil {
		log.Error("new tron data client fail", "err", err)
		return nil, err
	}
	sponsor := rpc.Sponsor
	if sponsor.ActivationTransferSun <= 0 {
		sponsor.ActivationTransferSun = defaultActivationTransferSun
	}
	if sponsor.DelegateBalanceSun <= 0 {
		sponsor.DelegateBalanceSun = defaultDelegateBalanceSun
	}
	if sponsor.EstimatedEnergy <= 0 {
		sponsor.EstimatedEnergy = defaultEstimatedEnergy
	}
	return &ChainAdaptor{
		tronClient:     tronClient,
		tronDataClient: tronDataClient,
		sponsor:        sponsor,
	}, nil
}

func (c *ChainAdaptor) ConvertAddresses(ctx context.Context, req *walletapi.ConvertAddressesRequest) (*walletapi.ConvertAddressesResponse, error) {
	var retAddressList []*walletapi.Addresses
	for _, publicKeyItem := range req.PublicKey {
		var addressItem *walletapi.Addresses
		publicKeyBytes, err := hex.DecodeString(strings.TrimPrefix(publicKeyItem.PublicKey, "0x"))
		if err != nil {
			addressItem = &walletapi.Addresses{
				Address:   "",
				PublicKey: publicKeyItem.PublicKey,
				Type:      publicKeyItem.Type,
			}
			log.Error("decode public key fail", "err", err)
		} else {
			pubKey, err := btcec.ParsePubKey(publicKeyBytes)
			if err != nil {
				addressItem = &walletapi.Addresses{
					Address: "",
				}
				log.Error("parse public key fail", "err", err)
			} else {
				addr := address.PubkeyToAddress(*pubKey.ToECDSA())
				log.Info("convert addresses", "address", addr.String())
				addressItem = &walletapi.Addresses{
					Address:   addr.String(),
					PublicKey: publicKeyItem.PublicKey,
					Type:      publicKeyItem.Type,
				}
			}
		}
		retAddressList = append(retAddressList, addressItem)
	}
	return &walletapi.ConvertAddressesResponse{
		Code:    common.ReturnCode_SUCCESS,
		Msg:     "success",
		Address: retAddressList,
	}, nil
}

func (c *ChainAdaptor) ValidAddresses(ctx context.Context, req *walletapi.ValidAddressesRequest) (*walletapi.ValidAddressesResponse, error) {
	var retAddressList []*walletapi.AddressesValid
	for _, addr := range req.Addresses {
		tronAddr, err := address.Base58ToAddress(addr.Address)
		valid := err == nil && tronAddr.IsValid()
		retAddressList = append(retAddressList, &walletapi.AddressesValid{
			Address: addr.Address,
			Valid:   valid,
		})
	}
	return &walletapi.ValidAddressesResponse{
		Code:         common.ReturnCode_SUCCESS,
		Msg:          "success",
		AddressValid: retAddressList,
	}, nil
}

func (c *ChainAdaptor) GetLastestBlock(ctx context.Context, req *walletapi.LastestBlockRequest) (*walletapi.LastestBlockResponse, error) {
	blockResp, err := c.tronClient.GetLatestBlock()
	if err != nil {
		log.Error("get latest block fail", "err", err)
		return &walletapi.LastestBlockResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  err.Error(),
		}, err
	}
	return &walletapi.LastestBlockResponse{
		Code:       common.ReturnCode_SUCCESS,
		Msg:        "success",
		Height:     uint64(blockResp.BlockHeader.RawData.Number),
		Hash:       blockResp.BlockID,
		ParentHash: blockResp.BlockHeader.RawData.ParentHash,
		Timestamp:  blockTimestampSeconds(blockResp.BlockHeader.RawData.Timestamp),
	}, nil
}

func (c *ChainAdaptor) GetBlock(ctx context.Context, req *walletapi.BlockRequest) (*walletapi.BlockResponse, error) {
	blockResp, err := c.tronClient.GetBlockByNumber(req.HashHeight)
	if err != nil {
		log.Error("get block fail", "err", err)
		return &walletapi.BlockResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  err.Error(),
		}, err
	}
	var txList []*walletapi.TransactionList
	if blockResp.Transactions != nil {
		txList = parseBlockTransactions(blockResp)
	}
	return &walletapi.BlockResponse{
		Code:         common.ReturnCode_SUCCESS,
		Msg:          "success",
		Height:       strconv.FormatInt(blockResp.BlockHeader.RawData.Number, 10),
		Hash:         blockResp.BlockID,
		ParentHash:   blockResp.BlockHeader.RawData.ParentHash,
		Timestamp:    blockTimestampSeconds(blockResp.BlockHeader.RawData.Timestamp),
		Transactions: txList,
	}, nil
}

func (c *ChainAdaptor) GetBatchBlock(ctx context.Context, req *walletapi.BatchBlockRequest) (*walletapi.BatchBlockResponse, error) {
	startHeight, err := strconv.ParseInt(req.NextHeight, 10, 64)
	if err != nil {
		return &walletapi.BatchBlockResponse{Code: common.ReturnCode_ERROR, Msg: "invalid next height"}, nil
	}
	endHeight, err := strconv.ParseInt(req.EndHeight, 10, 64)
	if err != nil {
		return &walletapi.BatchBlockResponse{Code: common.ReturnCode_ERROR, Msg: "invalid end height"}, nil
	}
	if startHeight > endHeight {
		return &walletapi.BatchBlockResponse{Code: common.ReturnCode_ERROR, Msg: "next height is greater than end height"}, nil
	}

	if !req.WithTransactions {
		headers := make([]*walletapi.BlockHeaderInfo, 0, endHeight-startHeight+1)
		for height := startHeight; height <= endHeight; height++ {
			blockResp, blockErr := c.tronClient.GetBlockByNumber(height)
			if blockErr != nil {
				log.Error("get block header fail", "height", height, "err", blockErr)
				return &walletapi.BatchBlockResponse{Code: common.ReturnCode_ERROR, Msg: blockErr.Error()}, nil
			}
			headers = append(headers, &walletapi.BlockHeaderInfo{
				Height:     strconv.FormatInt(blockResp.BlockHeader.RawData.Number, 10),
				Hash:       blockResp.BlockID,
				ParentHash: blockResp.BlockHeader.RawData.ParentHash,
				Timestamp:  blockTimestampSeconds(blockResp.BlockHeader.RawData.Timestamp),
			})
		}
		return &walletapi.BatchBlockResponse{
			Code:    common.ReturnCode_SUCCESS,
			Msg:     "get batch block success",
			Headers: headers,
		}, nil
	}

	blocks := make([]*walletapi.BlockWithTransactions, 0, endHeight-startHeight+1)
	for height := startHeight; height <= endHeight; height++ {
		blockResp, blockErr := c.tronClient.GetBlockByNumber(height)
		if blockErr != nil {
			log.Error("get batch block fail", "height", height, "err", blockErr)
			return &walletapi.BatchBlockResponse{Code: common.ReturnCode_ERROR, Msg: blockErr.Error()}, nil
		}
		blocks = append(blocks, &walletapi.BlockWithTransactions{
			Height:       strconv.FormatInt(blockResp.BlockHeader.RawData.Number, 10),
			Hash:         blockResp.BlockID,
			ParentHash:   blockResp.BlockHeader.RawData.ParentHash,
			Timestamp:    blockTimestampSeconds(blockResp.BlockHeader.RawData.Timestamp),
			Transactions: parseBlockTransactions(blockResp),
		})
	}
	return &walletapi.BatchBlockResponse{
		Code:   common.ReturnCode_SUCCESS,
		Msg:    "get batch block success",
		Blocks: blocks,
	}, nil
}

func (c *ChainAdaptor) GetTransactionByHash(ctx context.Context, req *walletapi.TransactionByHashRequest) (*walletapi.TransactionByHashResponse, error) {
	tx, err := c.tronClient.GetTransactionByHash(req.Hash)
	if err != nil {
		log.Error("get transaction fail", "err", err)
		return &walletapi.TransactionByHashResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  err.Error(),
		}, err
	}
	var fromAddrs []*walletapi.FromAddress
	var toAddrs []*walletapi.ToAddress
	var contractAddress string
	var txType uint32
	if len(tx.RawData.Contract) > 0 {
		contract := tx.RawData.Contract[0]
		switch contract.Type {
		case "TransferContract":
			txType = 1 // Native TRX transfer
			if contract.Parameter.Value.OwnerAddress != "" {
				fromAddr := HexToTronAddress(contract.Parameter.Value.OwnerAddress)
				fromAddrs = append(fromAddrs, &walletapi.FromAddress{
					Address: fromAddr,
					Amount:  strconv.FormatInt(contract.Parameter.Value.Amount, 10),
				})
			}
			if contract.Parameter.Value.ToAddress != "" {
				toAddr := HexToTronAddress(contract.Parameter.Value.ToAddress)
				toAddrs = append(toAddrs, &walletapi.ToAddress{
					Address: toAddr,
					Amount:  strconv.FormatInt(contract.Parameter.Value.Amount, 10),
				})
			}
		case "TriggerSmartContract":
			txType = 2 // TRC20 Token transfer
			if contract.Parameter.Value.ContractAddress != "" {
				contractAddress = HexToTronAddress(contract.Parameter.Value.ContractAddress)
			}
			if contract.Parameter.Value.OwnerAddress != "" {
				fromAddr := HexToTronAddress(contract.Parameter.Value.OwnerAddress)
				fromAddrs = append(fromAddrs, &walletapi.FromAddress{
					Address: fromAddr,
				})
			}
			// Parse data to get to address and amount
			if contract.Parameter.Value.Data != "" {
				data := contract.Parameter.Value.Data
				if len(data) >= 136 && strings.HasPrefix(data, "a9059cbb") {
					toAddrHex := "41" + data[32:72]
					toAddr := HexToTronAddress(toAddrHex)
					amountHex := data[72:136]
					amount := "0"
					if amountBig, ok := new(big.Int).SetString(amountHex, 16); ok {
						amount = amountBig.String()
					}
					fromAddrs[0].Amount = amount
					toAddrs = append(toAddrs, &walletapi.ToAddress{
						Address: toAddr,
						Amount:  amount,
					})
				}
			}
		}
	}
	return &walletapi.TransactionByHashResponse{
		Code: common.ReturnCode_SUCCESS,
		Msg:  "success",
		Transaction: &walletapi.TransactionList{
			TxHash:          req.Hash,
			From:            fromAddrs,
			To:              toAddrs,
			ContractAddress: contractAddress,
			TxType:          txType,
			Status:          contractRetStatus(tx),
		},
	}, nil
}

func (c *ChainAdaptor) GetTransactionByAddress(ctx context.Context, req *walletapi.TransactionByAddressRequest) (*walletapi.TransactionByAddressResponse, error) {
	page := int(req.Page)
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 10
	}
	txs, err := c.tronDataClient.GetTransactionsByAddress(req.Address, page, pageSize)
	if err != nil {
		log.Error("get transactions for address fail", "err", err)
		return &walletapi.TransactionByAddressResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  err.Error(),
		}, err
	}
	var txList []*walletapi.TransactionList
	for _, tx := range txs {
		txList = append(txList, &walletapi.TransactionList{
			TxHash: tx.TxID,
		})
	}
	return &walletapi.TransactionByAddressResponse{
		Code:        common.ReturnCode_SUCCESS,
		Msg:         "success",
		Transaction: txList,
	}, nil
}

func (c *ChainAdaptor) GetAccountBalance(ctx context.Context, req *walletapi.AccountBalanceRequest) (*walletapi.AccountBalanceResponse, error) {
	if strings.EqualFold(strings.TrimSpace(req.ContractAddress), "__activated__") {
		activated, err := c.tronClient.IsAccountActivated(req.Address)
		if err != nil {
			return &walletapi.AccountBalanceResponse{Code: common.ReturnCode_ERROR, Msg: err.Error()}, err
		}
		balance := "0"
		if activated {
			balance = "1"
		}
		return &walletapi.AccountBalanceResponse{
			Code:    common.ReturnCode_SUCCESS,
			Msg:     "success",
			Balance: balance,
		}, nil
	}
	if strings.EqualFold(strings.TrimSpace(req.ContractAddress), "__resource__") {
		resource, err := c.tronClient.GetAccountResource(req.Address)
		if err != nil {
			return &walletapi.AccountBalanceResponse{Code: common.ReturnCode_ERROR, Msg: err.Error()}, err
		}
		available := resource.EnergyLimit - resource.EnergyUsed
		if available < 0 {
			available = 0
		}
		return &walletapi.AccountBalanceResponse{
			Code:    common.ReturnCode_SUCCESS,
			Msg:     "success",
			Balance: strconv.FormatInt(available, 10),
		}, nil
	}
	if strings.EqualFold(strings.TrimSpace(req.ContractAddress), "__bandwidth__") {
		resource, err := c.tronClient.GetAccountResource(req.Address)
		if err != nil {
			return &walletapi.AccountBalanceResponse{Code: common.ReturnCode_ERROR, Msg: err.Error()}, err
		}
		freeAvailable := resource.FreeNetLimit - resource.FreeNetUsed
		if freeAvailable < 0 {
			freeAvailable = 0
		}
		netAvailable := resource.NetLimit - resource.NetUsed
		if netAvailable < 0 {
			netAvailable = 0
		}
		return &walletapi.AccountBalanceResponse{
			Code:    common.ReturnCode_SUCCESS,
			Msg:     "success",
			Balance: strconv.FormatInt(freeAvailable+netAvailable, 10),
		}, nil
	}
	if renterOverride, isRentalQuery := parseJustLendRentalQuery(req.ContractAddress); isRentalQuery {
		renter := renterOverride
		if renter == "" {
			renter = strings.TrimSpace(c.sponsor.Address)
		}
		if renter == "" {
			return &walletapi.AccountBalanceResponse{
				Code: common.ReturnCode_ERROR,
				Msg:  "tron sponsor address is not configured",
			}, nil
		}
		contract := strings.TrimSpace(c.sponsor.JustLendContract)
		if contract == "" {
			contract = DefaultJustLendContractMainnet
		}
		delegatedSun, err := c.tronClient.QueryJustLendRentalDelegatedSun(renter, req.Address, contract)
		if err != nil {
			return &walletapi.AccountBalanceResponse{Code: common.ReturnCode_ERROR, Msg: err.Error()}, err
		}
		return &walletapi.AccountBalanceResponse{
			Code:    common.ReturnCode_SUCCESS,
			Msg:     "success",
			Balance: strconv.FormatInt(delegatedSun, 10),
		}, nil
	}
	if strings.EqualFold(strings.TrimSpace(req.ContractAddress), "__delegatable__") {
		maxSize, err := c.tronClient.GetCanDelegatedMaxSize(req.Address, 1)
		if err != nil {
			return &walletapi.AccountBalanceResponse{Code: common.ReturnCode_ERROR, Msg: err.Error()}, err
		}
		return &walletapi.AccountBalanceResponse{
			Code:    common.ReturnCode_SUCCESS,
			Msg:     "success",
			Balance: strconv.FormatInt(maxSize, 10),
		}, nil
	}
	if req.ContractAddress == "" {
		account, err := c.tronClient.GetAccount(req.Address)
		if err != nil {
			log.Error("get account fail", "err", err)
			return &walletapi.AccountBalanceResponse{
				Code: common.ReturnCode_ERROR,
				Msg:  err.Error(),
			}, err
		}
		return &walletapi.AccountBalanceResponse{
			Code:    common.ReturnCode_SUCCESS,
			Msg:     "success",
			Balance: strconv.FormatInt(account.Balance, 10),
		}, nil
	}
	balance, err := c.tronClient.GetTRC20Balance(req.Address, req.ContractAddress)
	if err != nil {
		log.Error("get trc20 balance fail", "err", err)
		return &walletapi.AccountBalanceResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  err.Error(),
		}, err
	}
	return &walletapi.AccountBalanceResponse{
		Code:    common.ReturnCode_SUCCESS,
		Msg:     "success",
		Balance: balance,
	}, nil
}

func (c *ChainAdaptor) SendTransaction(ctx context.Context, req *walletapi.SendTransactionsRequest) (*walletapi.SendTransactionResponse, error) {
	var txnRetList []*walletapi.RawTransactionReturn
	for _, rawTx := range req.RawTx {
		txBytes, err := base64.StdEncoding.DecodeString(rawTx.RawTx)
		if err != nil {
			log.Error("decode signed tx fail", "err", err)
			txnRetList = append(txnRetList, &walletapi.RawTransactionReturn{TxHash: "", IsSuccess: false})
			continue
		}
		var transaction Transaction
		if err := json.Unmarshal(txBytes, &transaction); err != nil {
			log.Error("unmarshal signed tx fail", "err", err)
			txnRetList = append(txnRetList, &walletapi.RawTransactionReturn{TxHash: "", IsSuccess: false})
			continue
		}
		txHash, err := c.tronClient.BroadcastTransaction(&transaction)
		if err != nil {
			log.Error("broadcast transaction fail", "err", err)
			txnRetList = append(txnRetList, &walletapi.RawTransactionReturn{TxHash: "", IsSuccess: false})
			continue
		}
		txnRetList = append(txnRetList, &walletapi.RawTransactionReturn{
			TxHash:    txHash,
			IsSuccess: true,
		})
	}
	return &walletapi.SendTransactionResponse{
		Code:   common.ReturnCode_SUCCESS,
		Msg:    "success",
		TxnRet: txnRetList,
	}, nil
}

func (c *ChainAdaptor) BuildTransactionSchema(ctx context.Context, request *walletapi.TransactionSchemaRequest) (*walletapi.TransactionSchemaResponse, error) {
	return &walletapi.TransactionSchemaResponse{
		Code: common.ReturnCode_SUCCESS,
		Msg:  "success",
	}, nil
}

func (c *ChainAdaptor) BuildUnSignTransaction(ctx context.Context, request *walletapi.UnSignTransactionRequest) (*walletapi.UnSignTransactionResponse, error) {
	var unsignedTxList []*walletapi.UnsignedTransactionMessageHash
	for _, base64Tx := range request.Base64Txn {
		jsonBytes, err := base64.StdEncoding.DecodeString(base64Tx.Base64Tx)
		if err != nil {
			log.Error("decode string fail", "err", err)
			return nil, err
		}
		var data TxStructure
		if err := json.Unmarshal(jsonBytes, &data); err != nil {
			log.Error("parse json fail", "err", err)
			return nil, err
		}
		var transaction *Transaction
		transaction, err = buildUnsignedTransaction(c.tronClient, data)
		if err != nil {
			log.Error("create transaction fail", "err", err)
			return nil, err
		}
		txBytes, err := json.Marshal(transaction)
		if err != nil {
			log.Error("marshal transaction fail", "err", err)
			return nil, err
		}
		unsignedTxBase64 := base64.StdEncoding.EncodeToString(txBytes)
		unsignedTxList = append(unsignedTxList, &walletapi.UnsignedTransactionMessageHash{
			UnsignedTx: unsignedTxBase64,
		})
	}
	return &walletapi.UnSignTransactionResponse{
		Code:        common.ReturnCode_SUCCESS,
		Msg:         "success",
		UnsignedTxn: unsignedTxList,
	}, nil
}

func (c *ChainAdaptor) BuildSignedTransaction(ctx context.Context, request *walletapi.SignedTransactionRequest) (*walletapi.SignedTransactionResponse, error) {
	var signedTxList []*walletapi.SignedTxWithHash
	for _, txWithSig := range request.TxnWithSignature {
		txBytes, err := base64.StdEncoding.DecodeString(txWithSig.Base64Tx)
		if err != nil {
			log.Error("decode base64 tx fail", "err", err)
			signedTxList = append(signedTxList, &walletapi.SignedTxWithHash{
				SignedTx:  "",
				TxHash:    "",
				IsSuccess: false,
			})
			continue
		}
		var transaction Transaction
		if err := json.Unmarshal(txBytes, &transaction); err != nil {
			log.Error("unmarshal transaction fail", "err", err)
			signedTxList = append(signedTxList, &walletapi.SignedTxWithHash{
				SignedTx:  "",
				TxHash:    "",
				IsSuccess: false,
			})
			continue
		}
		signatureBytes, err := hex.DecodeString(strings.TrimPrefix(txWithSig.Signature, "0x"))
		if err != nil {
			log.Error("decode signature fail", "err", err)
			signedTxList = append(signedTxList, &walletapi.SignedTxWithHash{
				SignedTx:  "",
				TxHash:    "",
				IsSuccess: false,
			})
			continue
		}
		signatureHex := formatTronSignatureHex(hex.EncodeToString(signatureBytes))
		transaction.Signature = []string{signatureHex}
		if transaction.TxID == "" {
			if transaction.RawDataHex != "" {
				rawDataBytes, err := hex.DecodeString(transaction.RawDataHex)
				if err != nil {
					log.Error("decode raw data hex fail", "err", err)
					signedTxList = append(signedTxList, &walletapi.SignedTxWithHash{
						SignedTx:  "",
						TxHash:    "",
						IsSuccess: false,
					})
					continue
				}
				hash := sha256.Sum256(rawDataBytes)
				transaction.TxID = hex.EncodeToString(hash[:])
			}
		}
		signedTxBytes, err := json.Marshal(transaction)
		if err != nil {
			log.Error("marshal signed tx fail", "err", err)
			signedTxList = append(signedTxList, &walletapi.SignedTxWithHash{
				SignedTx:  "",
				TxHash:    "",
				IsSuccess: false,
			})
			continue
		}
		signedTxList = append(signedTxList, &walletapi.SignedTxWithHash{
			SignedTx:  base64.StdEncoding.EncodeToString(signedTxBytes),
			TxHash:    transaction.TxID,
			IsSuccess: true,
		})
	}
	return &walletapi.SignedTransactionResponse{
		Code:      common.ReturnCode_SUCCESS,
		Msg:       "success",
		SignedTxn: signedTxList,
	}, nil
}

func (c *ChainAdaptor) GetAddressApproveList(ctx context.Context, request *walletapi.AddressApproveListRequest) (*walletapi.AddressApproveListResponse, error) {
	return &walletapi.AddressApproveListResponse{
		Code: common.ReturnCode_SUCCESS,
		Msg:  "don't support in this stage, support in the future",
	}, nil
}

func (c *ChainAdaptor) BuildSponsoredTransfer(ctx context.Context, request *walletapi.SponsoredTransferRequest) (*walletapi.SponsoredTransferBuildResponse, error) {
	return &walletapi.SponsoredTransferBuildResponse{Code: common.ReturnCode_ERROR, Msg: "unsupported chain"}, nil
}

func (c *ChainAdaptor) SendSponsoredTransfer(ctx context.Context, request *walletapi.SponsoredTransferSendRequest) (*walletapi.SendTransactionResponse, error) {
	return &walletapi.SendTransactionResponse{Code: common.ReturnCode_ERROR, Msg: "unsupported chain"}, nil
}

func (c *ChainAdaptor) CallContract(ctx context.Context, request *walletapi.CallContractRequest) (*walletapi.CallContractResponse, error) {
	return &walletapi.CallContractResponse{Code: common.ReturnCode_ERROR, Msg: "callContract not supported on Tron"}, nil
}
