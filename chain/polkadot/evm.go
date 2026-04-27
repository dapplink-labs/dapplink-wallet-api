// Package polkadot 提供与 Polkadot 网络交互的链适配器实现，支持地址转换、余额查询、交易发送等功能。
package polkadot

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dapplink-labs/dapplink-wallet-api/config"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/status-im/keycard-go/hexutils"

	"github.com/dapplink-labs/chain-explorer-api/common/account"
	"github.com/dapplink-labs/dapplink-wallet-api/chain"
	"github.com/dapplink-labs/dapplink-wallet-api/chain/evmbase"
	"github.com/dapplink-labs/dapplink-wallet-api/common/util"
	wallet_api "github.com/dapplink-labs/dapplink-wallet-api/protobuf/wallet-api"
)

type EvmChainAdaptor struct {
	ethClient     evmbase.EthClient
	ethDataClient *evmbase.EthData
}

func NewEvmChainAdaptor(chainId string) func(conf *config.Config) (chain.IChainAdaptor, error) {
	return func(conf *config.Config) (chain.IChainAdaptor, error) {
		node := getNodeConfig(chainId, conf)
		return newEvmChainAdaptor(node.RpcUrl, node.DataApiUrl, node.DataApiKey)
	}
}

func newEvmChainAdaptor(rpcUrl, dataApiUrl, dataApiKey string) (chain.IChainAdaptor, error) {
	ethClient, err := evmbase.DialEthClient(context.Background(), rpcUrl)
	if err != nil {
		log.Error("Dial evm client fail", "err", err)
		return nil, err
	}
	ethDataClient, err := evmbase.NewEthDataClient(dataApiUrl, dataApiKey, time.Second*15)
	if err != nil {
		log.Error("new evm data client fail", "err", err)
		return nil, err
	}
	return &EvmChainAdaptor{
		ethClient:     ethClient,
		ethDataClient: ethDataClient,
	}, nil
}

func (c EvmChainAdaptor) ConvertAddresses(ctx context.Context, req *wallet_api.ConvertAddressesRequest) (*wallet_api.ConvertAddressesResponse, error) {
	var retAddressList []*wallet_api.Addresses
	for _, publicKeyItem := range req.PublicKey {
		var addressItem *wallet_api.Addresses
		publicKeyBytes, err := hex.DecodeString(strings.TrimPrefix(publicKeyItem.PublicKey, "0x"))
		if err != nil {
			addressItem = &wallet_api.Addresses{Address: ""}
			log.Error("decode public key fail", "err", err)
		} else {
			addressCommon := common.BytesToAddress(crypto.Keccak256(publicKeyBytes[1:])[12:])
			log.Info("convert evm addresses", "address", addressCommon.String())
			addressItem = &wallet_api.Addresses{Address: addressCommon.String()}
		}
		retAddressList = append(retAddressList, addressItem)
	}
	return &wallet_api.ConvertAddressesResponse{
		Code:    wallet_api.ApiReturnCode_APISUCCESS,
		Msg:     "success",
		Address: retAddressList,
	}, nil
}

func (c EvmChainAdaptor) ValidAddresses(ctx context.Context, req *wallet_api.ValidAddressesRequest) (*wallet_api.ValidAddressesResponse, error) {
	var retAddressesValid []*wallet_api.AddressesValid
	for _, addressItem := range req.Addresses {
		var addressesValidItem wallet_api.AddressesValid
		addressesValidItem.Address = addressItem.GetAddress()
		ok := regexp.MustCompile("^[0-9a-fA-F]{40}$").MatchString(addressItem.GetAddress()[2:])
		if len(addressItem.GetAddress()) != 42 || !strings.HasPrefix(addressItem.GetAddress(), "0x") || !ok {
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

func (c EvmChainAdaptor) GetLastestBlock(ctx context.Context, req *wallet_api.LastestBlockRequest) (*wallet_api.LastestBlockResponse, error) {
	latestBock, err := c.ethClient.BlockHeaderByNumber(nil)
	if err != nil {
		log.Error("Get latest block fail", "err", err)
		return nil, err
	}
	return &wallet_api.LastestBlockResponse{
		Code:   wallet_api.ApiReturnCode_APISUCCESS,
		Msg:    "get lastest block success",
		Hash:   latestBock.Hash().String(),
		Height: latestBock.Number.Uint64(),
	}, nil
}

func (c EvmChainAdaptor) GetBlock(ctx context.Context, req *wallet_api.BlockRequest) (*wallet_api.BlockResponse, error) {
	hashHeigh := req.HashHeight
	var isError bool
	var rpcBlock *evmbase.RpcBlock
	var err error
	if req.IsBlockHash {
		rpcBlock, err = c.ethClient.BlockByHash(common.HexToHash(hashHeigh))
		if err != nil {
			log.Error("Get block information fail", "err", err)
			isError = true
		}
	} else {
		blockNumber := new(big.Int)
		blockNumber.SetString(hashHeigh, 10)
		rpcBlock, err = c.ethClient.BlockByNumber(blockNumber)
		if err != nil {
			log.Error("Get block information fail", "err", err)
			isError = true
		}
	}
	var transactionList []*wallet_api.TransactionList
	if rpcBlock != nil {
		for _, bockItem := range rpcBlock.Transactions {
			amountDec := new(big.Int)
			if val, ok := new(big.Int).SetString(bockItem.Value, 0); ok {
				amountDec = val
			}
			amountStr := amountDec.String()
			var fromList []*wallet_api.FromAddress
			var toList []*wallet_api.ToAddress
			fromList = append(fromList, &wallet_api.FromAddress{
				Address: bockItem.From,
				Amount:  amountStr,
			})
			toList = append(toList, &wallet_api.ToAddress{
				Address: bockItem.To,
				Amount:  amountStr,
			})
			var fee string
			var txType uint32
			var contractAddress string
			var txStatus wallet_api.TxStatus
			receipt, err := c.ethClient.TxReceiptByHash(common.HexToHash(bockItem.Hash))
			if err == nil {
				gasUsed := receipt.GasUsed
				feeBig := new(big.Int).Mul(receipt.EffectiveGasPrice, new(big.Int).SetUint64(gasUsed))
				fee = feeBig.String()
				if receipt.Status == 1 {
					txStatus = wallet_api.TxStatus_Success
				} else {
					txStatus = wallet_api.TxStatus_Failed
				}
			}
			if bockItem.To == "" {
				txType = 0 // 合约创建
			} else {
				code, err := c.ethClient.EthGetCode(common.HexToAddress(bockItem.To))
				if err == nil {
					if code == "eoa" {
						txType = 1 // EOA转账
					} else {
						txType = 2 // 合约调用
						contractAddress = bockItem.To
						if len(bockItem.Input) >= 10 && bockItem.Input[:10] == "0xa9059cbb" {
							txType = 3 // ERC20转账
						}
					}
				}
			}
			txItem := &wallet_api.TransactionList{
				TxHash:          bockItem.Hash,
				Fee:             fee,
				Status:          uint32(txStatus),
				TxType:          txType,
				ContractAddress: contractAddress,
				From:            fromList,
				To:              toList,
			}
			transactionList = append(transactionList, txItem)
		}
	}
	if !isError {
		blockHeight, _ := rpcBlock.NumberUint64()
		return &wallet_api.BlockResponse{
			Code:         wallet_api.ApiReturnCode_APISUCCESS,
			Msg:          "get block success",
			Height:       strconv.FormatUint(blockHeight, 10),
			Hash:         rpcBlock.Hash.String(),
			Transactions: transactionList,
		}, nil
	}
	return &wallet_api.BlockResponse{
		Code: wallet_api.ApiReturnCode_APIERROR,
		Msg:  "get block failed",
	}, nil
}

func (c EvmChainAdaptor) GetTransactionByHash(ctx context.Context, req *wallet_api.TransactionByHashRequest) (*wallet_api.TransactionByHashResponse, error) {
	tx, err := c.ethClient.TxByHash(common.HexToHash(req.Hash))
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			return &wallet_api.TransactionByHashResponse{
				Code: wallet_api.ApiReturnCode_APIERROR,
				Msg:  "Polkadot Hub Tx NotFound",
			}, nil
		}
		log.Error("get transaction error", "err", err)
		return &wallet_api.TransactionByHashResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  "Polkadot Hub Tx Fetch Error",
		}, nil
	}
	receipt, err := c.ethClient.TxReceiptByHash(common.HexToHash(req.Hash))
	if err != nil {
		log.Error("get transaction receipt error", "err", err)
		return &wallet_api.TransactionByHashResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  "Get transaction receipt error",
		}, nil
	}
	var toAddress string
	var contractAddress string
	var txType uint32
	var txStatus wallet_api.TxStatus

	signer := ethtypes.LatestSignerForChainID(tx.ChainId())
	sender, err := ethtypes.Sender(signer, tx)
	if err != nil {
		log.Error("Recover sender from tx failed", "err", err)
		return &wallet_api.TransactionByHashResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  "Recover sender failed",
		}, nil
	}

	if tx.To() == nil {
		toAddress = ""
		txType = 0
	} else {
		code, err := c.ethClient.EthGetCode(*tx.To())
		if err != nil {
			log.Error("Get transaction code error", "err", err)
			return nil, err
		}
		if code == "0x" {
			txType = 1
			toAddress = tx.To().Hex()
		} else {
			txType = 2
			contractAddress = tx.To().Hex()
			method := tx.Data()[:4]
			if hexutils.BytesToHex(method) == "0xa9059cbb" {
				txType = 3
				toAddress = hexutils.BytesToHex(common.LeftPadBytes(tx.Data(), 32))
				amount := hexutils.BytesToHex(common.LeftPadBytes(tx.Data(), 32))
				fmt.Println("amount", amount)
			}
		}
	}

	if receipt.Status == 1 {
		txStatus = wallet_api.TxStatus_Success
	} else {
		txStatus = wallet_api.TxStatus_Failed
	}
	fee := new(big.Int).Mul(receipt.EffectiveGasPrice, big.NewInt(int64(receipt.GasUsed)))

	log.Info("tx information", "fee", fee.String(), "toAddress", toAddress, "txStatus", txStatus)
	var fromList []*wallet_api.FromAddress
	fromList = append(fromList, &wallet_api.FromAddress{
		Address: sender.Hex(),
		Amount:  tx.Value().String(),
	})

	var toList []*wallet_api.ToAddress
	if tx.To() != nil {
		toList = append(toList, &wallet_api.ToAddress{
			Address: tx.To().Hex(),
			Amount:  tx.Value().String(),
		})
	} else {
		toList = append(toList, &wallet_api.ToAddress{
			Address: receipt.ContractAddress.Hex(),
			Amount:  tx.Value().String(),
		})
	}

	return &wallet_api.TransactionByHashResponse{
		Code: wallet_api.ApiReturnCode_APISUCCESS,
		Msg:  "get transaction success",
		Transaction: &wallet_api.TransactionList{
			TxHash:          tx.Hash().Hex(),
			Fee:             fee.String(),
			Status:          uint32(txStatus),
			ContractAddress: contractAddress,
			TxType:          txType,
			From:            fromList,
			To:              toList,
		},
	}, nil
}

func (c EvmChainAdaptor) GetTransactionByAddress(ctx context.Context, req *wallet_api.TransactionByAddressRequest) (*wallet_api.TransactionByAddressResponse, error) {
	var resp *account.TransactionResponse[account.AccountTxResponse]
	var err error
	var txType uint32
	if req.ContractAddress != "0x00" && req.ContractAddress != "" {
		resp, err = c.ethDataClient.GetTxByAddress(uint64(req.Page), uint64(req.PageSize), req.Address, "tokentx")
		txType = 1
	} else {
		resp, err = c.ethDataClient.GetTxByAddress(uint64(req.Page), uint64(req.PageSize), req.Address, "txlist")
		txType = 0
	}
	if err != nil {
		log.Error("get GetTxByAddress error", "err", err)
		return &wallet_api.TransactionByAddressResponse{
			Code:        wallet_api.ApiReturnCode_APIERROR,
			Msg:         "get tx list fail",
			Transaction: nil,
		}, err
	} else {
		txs := resp.TransactionList
		list := make([]*wallet_api.TransactionList, 0, len(txs))
		for i := 0; i < len(txs); i++ {
			var fromList []*wallet_api.FromAddress
			var toList []*wallet_api.ToAddress
			fromList = append(fromList, &wallet_api.FromAddress{
				Address: txs[i].From,
				Amount:  txs[i].Amount,
			})
			toList = append(toList, &wallet_api.ToAddress{
				Address: txs[i].To,
				Amount:  txs[i].Amount,
			})
			list = append(list, &wallet_api.TransactionList{
				TxHash: txs[i].TxId,
				To:     toList,
				From:   fromList,
				Fee:    txs[i].TxFee,
				Status: 1,
				TxType: txType,
			})
		}
		return &wallet_api.TransactionByAddressResponse{
			Code:        wallet_api.ApiReturnCode_APISUCCESS,
			Msg:         "get tx list by address success",
			Transaction: list,
		}, nil
	}
}

func (c EvmChainAdaptor) GetAccountBalance(ctx context.Context, req *wallet_api.AccountBalanceRequest) (*wallet_api.AccountBalanceResponse, error) {
	// 如果是查询原生代币余额（没有合约地址），直接通过ETH客户端查询
	if req.ContractAddress == "" {
		balance, err := c.ethClient.GetBalance(common.HexToAddress(req.Address))
		if err != nil {
			log.Error("get native token balance fail", "err", err)
			return &wallet_api.AccountBalanceResponse{
				Code:    wallet_api.ApiReturnCode_APIERROR,
				Msg:     "get native token balance fail",
				Balance: "0",
			}, nil
		}
		return &wallet_api.AccountBalanceResponse{
			Code:    wallet_api.ApiReturnCode_APISUCCESS,
			Msg:     "get native token balance success",
			Balance: balance.String(),
		}, nil
	}
	// 否则查询ERC20代币余额
	balanceResult, err := c.ethDataClient.GetBalanceByAddress(req.ContractAddress, req.Address)
	if err != nil {
		return &wallet_api.AccountBalanceResponse{
			Code:    wallet_api.ApiReturnCode_APIERROR,
			Msg:     "get token balance fail",
			Balance: "0",
		}, nil
	}
	log.Info("balance result", "balance=", balanceResult.Balance, "balanceStr=", balanceResult.BalanceStr)
	balanceStr := "0"
	if balanceResult.Balance != nil && balanceResult.Balance.Int() != nil {
		balanceStr = balanceResult.Balance.Int().String()
	}
	return &wallet_api.AccountBalanceResponse{
		Code:    wallet_api.ApiReturnCode_APISUCCESS,
		Msg:     "get token balance success",
		Balance: balanceStr,
	}, nil
}

func (c EvmChainAdaptor) SendTransaction(ctx context.Context, req *wallet_api.SendTransactionsRequest) (*wallet_api.SendTransactionResponse, error) {
	var txListRet []*wallet_api.RawTransactionReturn
	for _, txItem := range req.RawTx {
		var txRet wallet_api.RawTransactionReturn
		transaction, err := c.ethClient.SendRawTransaction(txItem.RawTx)
		if err != nil {
			txRet = wallet_api.RawTransactionReturn{
				TxHash:    "",
				IsSuccess: false,
				Message:   "this tx send failed",
			}
		} else {
			txRet = wallet_api.RawTransactionReturn{
				TxHash:    transaction.String(),
				IsSuccess: true,
				Message:   "this tx send success",
			}
		}
		txListRet = append(txListRet, &txRet)
	}
	return &wallet_api.SendTransactionResponse{
		Code:   wallet_api.ApiReturnCode_APISUCCESS,
		Msg:    "send tx success",
		TxnRet: txListRet,
	}, nil
}

func (c EvmChainAdaptor) BuildTransactionSchema(ctx context.Context, request *wallet_api.TransactionSchemaRequest) (*wallet_api.TransactionSchemaResponse, error) {
	panic("implement me")
}

func (c EvmChainAdaptor) BuildUnSignTransaction(ctx context.Context, request *wallet_api.UnSignTransactionRequest) (*wallet_api.UnSignTransactionResponse, error) {
	var unsignTxnRet []*wallet_api.UnsignedTransactionMessageHash
	for _, unsignedTxItem := range request.Base64Txn {
		result, err := c.buildEvmUnSignTx(unsignedTxItem.Base64Tx)
		if err != nil {
			log.Error("buildEvmUnSignTx failed", "err", err)
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

func (c EvmChainAdaptor) BuildSignedTransaction(ctx context.Context, request *wallet_api.SignedTransactionRequest) (*wallet_api.SignedTransactionResponse, error) {
	var signedTransactionList []*wallet_api.SignedTxWithHash
	for _, txWithSignature := range request.TxnWithSignature {
		signedTx, err := c.buildEvmSignedTx(txWithSignature.Base64Tx, txWithSignature.Signature)
		if err != nil {
			log.Error("buildEvmSignedTx failed", "err", err)
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

func (c EvmChainAdaptor) GetAddressApproveList(ctx context.Context, request *wallet_api.AddressApproveListRequest) (*wallet_api.AddressApproveListResponse, error) {
	panic("implement me")
}

func (c EvmChainAdaptor) buildEvmUnSignTx(base64Tx string) (string, error) {
	dFeeTx, _, err := c.buildDynamicFeeTx(base64Tx)
	if err != nil {
		return "", err
	}
	log.Info("polkadot hub buildEvmUnSignTx", "dFeeTx", util.ToJSONString(dFeeTx))
	return evmbase.CreateEip1559UnSignTx(dFeeTx, dFeeTx.ChainID)
}

func (c EvmChainAdaptor) buildEvmSignedTx(base64Tx, signatureHex string) (*wallet_api.SignedTxWithHash, error) {
	dFeeTx, dynamicFeeTx, err := c.buildDynamicFeeTx(base64Tx)
	if err != nil {
		return nil, err
	}

	inputSignatureByteList, err := hex.DecodeString(signatureHex)
	if err != nil {
		return nil, fmt.Errorf("decode signature failed: %w", err)
	}

	signer, signedTx, rawTx, txHash, err := evmbase.CreateEip1559SignedTx(dFeeTx, inputSignatureByteList, dFeeTx.ChainID)
	if err != nil {
		return &wallet_api.SignedTxWithHash{IsSuccess: false, SignedTx: rawTx, TxHash: txHash}, nil
	}

	sender, err := ethtypes.Sender(signer, signedTx)
	if err != nil {
		return nil, fmt.Errorf("recover sender failed: %w", err)
	}

	if sender.Hex() != dynamicFeeTx.FromAddress {
		return nil, fmt.Errorf("sender address mismatch: expected %s, got %s", dynamicFeeTx.FromAddress, sender.Hex())
	}

	return &wallet_api.SignedTxWithHash{IsSuccess: true, SignedTx: rawTx, TxHash: txHash}, nil
}

func (c *EvmChainAdaptor) buildDynamicFeeTx(base64Tx string) (*ethtypes.DynamicFeeTx, *evmbase.Eip1559DynamicFeeTx, error) {
	txReqJSONByte, err := base64.StdEncoding.DecodeString(base64Tx)
	if err != nil {
		log.Error("decode string fail", "err", err)
		return nil, nil, err
	}

	var dynamicFeeTx evmbase.Eip1559DynamicFeeTx
	if err := json.Unmarshal(txReqJSONByte, &dynamicFeeTx); err != nil {
		log.Error("parse json fail", "err", err)
		return nil, nil, err
	}

	chainID := new(big.Int)
	maxPriorityFeePerGas := new(big.Int)
	maxFeePerGas := new(big.Int)
	amount := new(big.Int)

	if _, ok := chainID.SetString(dynamicFeeTx.ChainId, 10); !ok {
		return nil, nil, fmt.Errorf("invalid chain ID: %s", dynamicFeeTx.ChainId)
	}
	if _, ok := maxPriorityFeePerGas.SetString(dynamicFeeTx.MaxPriorityFeePerGas, 10); !ok {
		return nil, nil, fmt.Errorf("invalid max priority fee: %s", dynamicFeeTx.MaxPriorityFeePerGas)
	}
	if _, ok := maxFeePerGas.SetString(dynamicFeeTx.MaxFeePerGas, 10); !ok {
		return nil, nil, fmt.Errorf("invalid max fee: %s", dynamicFeeTx.MaxFeePerGas)
	}
	if _, ok := amount.SetString(dynamicFeeTx.Amount, 10); !ok {
		return nil, nil, fmt.Errorf("invalid amount: %s", dynamicFeeTx.Amount)
	}

	toAddress := common.HexToAddress(dynamicFeeTx.ToAddress)
	var finalToAddress common.Address
	var finalAmount *big.Int
	var buildData []byte
	log.Info("contract address check",
		"contractAddress", dynamicFeeTx.ContractAddress,
		"isEthTransfer", evmbase.IsEthTransfer(&dynamicFeeTx),
	)

	if evmbase.IsEthTransfer(&dynamicFeeTx) {
		finalToAddress = toAddress
		finalAmount = amount
	} else {
		contractAddress := common.HexToAddress(dynamicFeeTx.ContractAddress)
		buildData = evmbase.BuildErc20Data(toAddress, amount)
		finalToAddress = contractAddress
		finalAmount = big.NewInt(0)
	}

	dFeeTx := &ethtypes.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     dynamicFeeTx.Nonce,
		GasTipCap: maxPriorityFeePerGas,
		GasFeeCap: maxFeePerGas,
		Gas:       dynamicFeeTx.GasLimit,
		To:        &finalToAddress,
		Value:     finalAmount,
		Data:      buildData,
	}

	return dFeeTx, &dynamicFeeTx, nil
}
