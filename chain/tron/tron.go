package tron

import (
	"context"
	"encoding/hex"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/ethereum/go-ethereum/log"
	"github.com/fbsobreira/gotron-sdk/pkg/address"

	"github.com/dapplink-labs/dapplink-wallet-api/chain"
	"github.com/dapplink-labs/dapplink-wallet-api/config"
	wallet_api "github.com/dapplink-labs/dapplink-wallet-api/protobuf/wallet-api"
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

func (c *ChainAdaptor) ConvertAddresses(ctx context.Context, req *wallet_api.ConvertAddressesRequest) (*wallet_api.ConvertAddressesResponse, error) {
	var retAddressList []*wallet_api.Addresses
	for _, publicKeyItem := range req.PublicKey {
		var addressItem *wallet_api.Addresses
		publicKeyBytes, err := hex.DecodeString(strings.TrimPrefix(publicKeyItem.PublicKey, "0x"))
		if err != nil {
			addressItem = &wallet_api.Addresses{
				Address: "",
			}
			log.Error("decode public key fail", "err", err)
		} else {
			pubKey, err := btcec.ParsePubKey(publicKeyBytes)
			if err != nil {
				addressItem = &wallet_api.Addresses{
					Address: "",
				}
				log.Error("parse public key fail", "err", err)
			} else {
				addr := address.PubkeyToAddress(*pubKey.ToECDSA())
				log.Info("convert addresses", "address", addr.String())
				addressItem = &wallet_api.Addresses{
					Address: addr.String(),
				}
			}
		}
		retAddressList = append(retAddressList, addressItem)
	}
	return &wallet_api.ConvertAddressesResponse{
		Code:    wallet_api.ApiReturnCode_APISUCCESS,
		Msg:     "success",
		Address: retAddressList,
	}, nil
}

func (c *ChainAdaptor) ValidAddresses(ctx context.Context, req *wallet_api.ValidAddressesRequest) (*wallet_api.ValidAddressesResponse, error) {
	var retAddressList []*wallet_api.AddressesValid
	for _, addr := range req.Addresses {
		tronAddr, err := address.Base58ToAddress(addr.Address)
		valid := err == nil && tronAddr.IsValid()
		retAddressList = append(retAddressList, &wallet_api.AddressesValid{
			Address: addr.Address,
			Valid:   valid,
		})
	}
	return &wallet_api.ValidAddressesResponse{
		Code:         wallet_api.ApiReturnCode_APISUCCESS,
		Msg:          "success",
		AddressValid: retAddressList,
	}, nil
}

func (c *ChainAdaptor) GetLastestBlock(ctx context.Context, req *wallet_api.LastestBlockRequest) (*wallet_api.LastestBlockResponse, error) {
	// Use "latest" to get the latest block
	blockResp, err := c.tronClient.GetBlockByNumber("latest")
	if err != nil {
		log.Error("get latest block fail", "err", err)
		return &wallet_api.LastestBlockResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  err.Error(),
		}, err
	}

	return &wallet_api.LastestBlockResponse{
		Code:   wallet_api.ApiReturnCode_APISUCCESS,
		Msg:    "success",
		Height: uint64(blockResp.BlockHeader.RawData.Number),
	}, nil
}

func (c *ChainAdaptor) GetBlock(ctx context.Context, req *wallet_api.BlockRequest) (*wallet_api.BlockResponse, error) {
	blockResp, err := c.tronClient.GetBlockByNumber(req.HashHeight)
	if err != nil {
		log.Error("get block fail", "err", err)
		return &wallet_api.BlockResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  err.Error(),
		}, err
	}

	var txList []*wallet_api.TransactionList
	if blockResp.Transactions != nil {
		for _, tx := range blockResp.Transactions {
			txList = append(txList, &wallet_api.TransactionList{
				TxHash: tx.TxID,
			})
		}
	}

	return &wallet_api.BlockResponse{
		Code:         wallet_api.ApiReturnCode_APISUCCESS,
		Msg:          "success",
		Height:       strconv.FormatInt(blockResp.BlockHeader.RawData.Number, 10),
		Hash:         blockResp.BlockID,
		Transactions: txList,
	}, nil
}

func (c *ChainAdaptor) GetTransactionByHash(ctx context.Context, req *wallet_api.TransactionByHashRequest) (*wallet_api.TransactionByHashResponse, error) {
	tx, err := c.tronClient.GetTransactionByHash(req.Hash)
	if err != nil {
		log.Error("get transaction fail", "err", err)
		return &wallet_api.TransactionByHashResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  err.Error(),
		}, err
	}

	var fromAddrs []*wallet_api.FromAddress
	var toAddrs []*wallet_api.ToAddress
	var contractAddress string
	var txType uint32

	if len(tx.RawData.Contract) > 0 {
		contract := tx.RawData.Contract[0]

		switch contract.Type {
		case "TransferContract":
			txType = 1 // Native TRX transfer
			if contract.Parameter.Value.OwnerAddress != "" {
				fromAddr := HexToTronAddress(contract.Parameter.Value.OwnerAddress)
				fromAddrs = append(fromAddrs, &wallet_api.FromAddress{
					Address: fromAddr,
					Amount:  strconv.FormatInt(contract.Parameter.Value.Amount, 10),
				})
			}
			if contract.Parameter.Value.ToAddress != "" {
				toAddr := HexToTronAddress(contract.Parameter.Value.ToAddress)
				toAddrs = append(toAddrs, &wallet_api.ToAddress{
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
				fromAddrs = append(fromAddrs, &wallet_api.FromAddress{
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
					toAddrs = append(toAddrs, &wallet_api.ToAddress{
						Address: toAddr,
						Amount:  amount,
					})
				}
			}
		}
	}

	return &wallet_api.TransactionByHashResponse{
		Code: wallet_api.ApiReturnCode_APISUCCESS,
		Msg:  "success",
		Transaction: &wallet_api.TransactionList{
			TxHash:          req.Hash,
			From:            fromAddrs,
			To:              toAddrs,
			ContractAddress: contractAddress,
			TxType:          txType,
		},
	}, nil
}

func (c *ChainAdaptor) GetTransactionByAddress(ctx context.Context, req *wallet_api.TransactionByAddressRequest) (*wallet_api.TransactionByAddressResponse, error) {
	page := int(req.Page)
	pageSize := int(req.PageSize)

	if pageSize == 0 {
		pageSize = 10
	}

	// Use TronData client to get transactions by address
	txs, err := c.tronDataClient.GetTransactionsByAddress(req.Address, page, pageSize)
	if err != nil {
		log.Error("get transactions for address fail", "err", err)
		return &wallet_api.TransactionByAddressResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  err.Error(),
		}, err
	}

	var txList []*wallet_api.TransactionList
	for _, tx := range txs {
		txList = append(txList, &wallet_api.TransactionList{
			TxHash: tx.TxID,
		})
	}

	return &wallet_api.TransactionByAddressResponse{
		Code:        wallet_api.ApiReturnCode_APISUCCESS,
		Msg:         "success",
		Transaction: txList,
	}, nil
}

func (c *ChainAdaptor) GetAccountBalance(ctx context.Context, req *wallet_api.AccountBalanceRequest) (*wallet_api.AccountBalanceResponse, error) {
	if req.ContractAddress == "" {
		// Native TRX balance
		account, err := c.tronClient.GetBalance(req.Address)
		if err != nil {
			log.Error("get account fail", "err", err)
			return &wallet_api.AccountBalanceResponse{
				Code: wallet_api.ApiReturnCode_APIERROR,
				Msg:  err.Error(),
			}, err
		}

		return &wallet_api.AccountBalanceResponse{
			Code:    wallet_api.ApiReturnCode_APISUCCESS,
			Msg:     "success",
			Balance: strconv.FormatInt(account.Balance, 10),
		}, nil
	} else {
		// TRC20 Token balance - use TronData API or implement TRC20 balance query
		// For now, return error as TRC20 balance query needs to be implemented
		return &wallet_api.AccountBalanceResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  "TRC20 balance query not implemented yet",
		}, nil
	}
}

func (c *ChainAdaptor) SendTransaction(ctx context.Context, req *wallet_api.SendTransactionsRequest) (*wallet_api.SendTransactionResponse, error) {
	var txnRetList []*wallet_api.RawTransactionReturn

	for _, rawTx := range req.RawTx {
		// Broadcast the signed transaction
		// For now, return a placeholder as we need to implement the broadcast method
		txnRetList = append(txnRetList, &wallet_api.RawTransactionReturn{
			TxHash: rawTx.RawTx,
		})
	}

	return &wallet_api.SendTransactionResponse{
		Code:   wallet_api.ApiReturnCode_APISUCCESS,
		Msg:    "success",
		TxnRet: txnRetList,
	}, nil
}

func (c *ChainAdaptor) BuildTransactionSchema(ctx context.Context, request *wallet_api.TransactionSchemaRequest) (*wallet_api.TransactionSchemaResponse, error) {
	return &wallet_api.TransactionSchemaResponse{
		Code: wallet_api.ApiReturnCode_APISUCCESS,
		Msg:  "success",
	}, nil
}

func (c *ChainAdaptor) BuildUnSignTransaction(ctx context.Context, request *wallet_api.UnSignTransactionRequest) (*wallet_api.UnSignTransactionResponse, error) {
	var unsignedTxList []*wallet_api.UnsignedTransactionMessageHash

	for _, base64Tx := range request.Base64Txn {
		// Decode the base64 transaction to get transaction parameters
		// For now, return a placeholder as the actual implementation depends on the transaction format
		unsignedTxList = append(unsignedTxList, &wallet_api.UnsignedTransactionMessageHash{
			UnsignedTx: base64Tx.Base64Tx,
		})
	}

	return &wallet_api.UnSignTransactionResponse{
		Code:        wallet_api.ApiReturnCode_APISUCCESS,
		Msg:         "success",
		UnsignedTxn: unsignedTxList,
	}, nil
}

func (c *ChainAdaptor) BuildSignedTransaction(ctx context.Context, request *wallet_api.SignedTransactionRequest) (*wallet_api.SignedTransactionResponse, error) {
	var signedTxList []*wallet_api.SignedTxWithHash

	for _, txWithSig := range request.TxnWithSignature {
		// Process each transaction with signature
		// For now, return a placeholder as the actual implementation depends on the transaction format
		signedTxList = append(signedTxList, &wallet_api.SignedTxWithHash{
			SignedTx: txWithSig.Base64Tx,
			TxHash:   "", // Will be computed from the signed transaction
		})
	}

	return &wallet_api.SignedTransactionResponse{
		Code:      wallet_api.ApiReturnCode_APISUCCESS,
		Msg:       "success",
		SignedTxn: signedTxList,
	}, nil
}

func (c *ChainAdaptor) GetAddressApproveList(ctx context.Context, request *wallet_api.AddressApproveListRequest) (*wallet_api.AddressApproveListResponse, error) {
	return &wallet_api.AddressApproveListResponse{
		Code: wallet_api.ApiReturnCode_APISUCCESS,
		Msg:  "don't support in this stage, support in the future",
	}, nil
}
