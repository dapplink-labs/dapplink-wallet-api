package tron

import (
	"context"
	"encoding/hex"
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
)

type ChainAdaptor struct {
	tronClient     *TronClient
	tronDataClient *TronData
}

func NewChainAdaptor(conf *config.Config) (chain.IChainAdaptor, error) {
	rpc := conf.WalletNode.Tron
	tronClient := DialTronClient(rpc.RpcUrl, rpc.RpcUser, rpc.RpcPass)
	tronDataClient, err := NewTronDataClient(conf.WalletNode.Tron.DataApiUrl, conf.WalletNode.Tron.DataApiKey, time.Second*15)
	if err != nil {
		log.Error("new tron data client fail", "err", err)
		return nil, err
	}
	return &ChainAdaptor{
		tronClient:     tronClient,
		tronDataClient: tronDataClient,
	}, nil
}

func (c *ChainAdaptor) ConvertAddresses(ctx context.Context, req *walletapi.ConvertAddressesRequest) (*walletapi.ConvertAddressesResponse, error) {
	var retAddressList []*walletapi.Addresses
	for _, publicKeyItem := range req.PublicKey {
		var addressItem *walletapi.Addresses
		publicKeyBytes, err := hex.DecodeString(strings.TrimPrefix(publicKeyItem.PublicKey, "0x"))
		if err != nil {
			addressItem = &walletapi.Addresses{
				Address: "",
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
					Address: addr.String(),
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
		Timestamp:  uint64(blockResp.BlockHeader.RawData.Timestamp),
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
		for _, tx := range blockResp.Transactions {
			var fromAddrs []*walletapi.FromAddress
			var toAddrs []*walletapi.ToAddress
			var contractAddress string
			var txType uint32
			if len(tx.RawData.Contract) > 0 {
				contract := tx.RawData.Contract[0]
				switch contract.Type {
				case "TransferContract": // Native TRX transfer
					txType = 1
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
				case "TriggerSmartContract": // TRC20 Token transfer
					txType = 2
					if contract.Parameter.Value.ContractAddress != "" {
						contractAddress = HexToTronAddress(contract.Parameter.Value.ContractAddress)
					}
					if contract.Parameter.Value.OwnerAddress != "" {
						fromAddr := HexToTronAddress(contract.Parameter.Value.OwnerAddress)
						fromAddrs = append(fromAddrs, &walletapi.FromAddress{
							Address: fromAddr,
						})
					}
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
			transaction := &walletapi.TransactionList{
				TxHash:          tx.TxID,
				From:            fromAddrs,
				To:              toAddrs,
				ContractAddress: contractAddress,
				TxType:          txType,
			}
			txList = append(txList, transaction)
		}
	}
	return &walletapi.BlockResponse{
		Code:         common.ReturnCode_SUCCESS,
		Msg:          "success",
		Height:       strconv.FormatInt(blockResp.BlockHeader.RawData.Number, 10),
		Hash:         blockResp.BlockID,
		Transactions: txList,
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
	if req.ContractAddress == "" {
		account, err := c.tronClient.GetBalance(req.Address)
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
	} else {
		return &walletapi.AccountBalanceResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  "TRC20 balance query not implemented yet",
		}, nil
	}
}

func (c *ChainAdaptor) SendTransaction(ctx context.Context, req *walletapi.SendTransactionsRequest) (*walletapi.SendTransactionResponse, error) {
	var txnRetList []*walletapi.RawTransactionReturn
	for _, rawTx := range req.RawTx {
		txnRetList = append(txnRetList, &walletapi.RawTransactionReturn{
			TxHash: rawTx.RawTx,
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
		unsignedTxList = append(unsignedTxList, &walletapi.UnsignedTransactionMessageHash{
			UnsignedTx: base64Tx.Base64Tx,
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
		signedTxList = append(signedTxList, &walletapi.SignedTxWithHash{
			SignedTx: txWithSig.Base64Tx,
			TxHash:   "",
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
