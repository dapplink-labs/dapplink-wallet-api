package ethereum

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/status-im/keycard-go/hexutils"

	"github.com/dapplink-labs/chain-explorer-api/common/account"
	"github.com/dapplink-labs/dapplink-wallet-api/chain"
	"github.com/dapplink-labs/dapplink-wallet-api/chain/evmbase"
	"github.com/dapplink-labs/dapplink-wallet-api/common/util"
	"github.com/dapplink-labs/dapplink-wallet-api/config"
	wallet_api "github.com/dapplink-labs/dapplink-wallet-api/protobuf/wallet-api"
)

const (
	ChainID string = "DappLinkEthereum"
)

type ChainAdaptor struct {
	ethClient     evmbase.EthClient
	ethDataClient *evmbase.EthData
}

func NewChainAdaptor(conf *config.Config) (chain.IChainAdaptor, error) {
	ethClient, err := evmbase.DialEthClient(context.Background(), conf.WalletNode.Eth.RpcUrl)
	if err != nil {
		log.Error("Dial eth client fail", "err", err)
		return nil, err
	}
	ethDataClient, err := evmbase.NewEthDataClient(conf.WalletNode.Eth.DataApiUrl, conf.WalletNode.Eth.DataApiKey, time.Second*15)
	if err != nil {
		log.Error("new eth data client fail", "err", err)
		return nil, err
	}
	return &ChainAdaptor{
		ethClient:     ethClient,
		ethDataClient: ethDataClient,
	}, nil
}

func (c ChainAdaptor) ConvertAddresses(ctx context.Context, req *wallet_api.ConvertAddressesRequest) (*wallet_api.ConvertAddressesResponse, error) {
	var retAddressList []*wallet_api.Addresses
	for _, publicKeyItem := range req.PublicKey {
		var addressItem *wallet_api.Addresses
		publicKeyBytes, err := hex.DecodeString(publicKeyItem.PublicKey)
		if err != nil {
			addressItem = &wallet_api.Addresses{
				Address: "",
			}
			log.Error("decode public key fail", "err", err)
		} else {
			addressCommon := common.BytesToAddress(crypto.Keccak256(publicKeyBytes[1:])[12:])
			log.Info("convert addresses", "address", addressCommon.String())
			addressItem = &wallet_api.Addresses{
				Address: addressCommon.String(),
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

func (c ChainAdaptor) ValidAddresses(ctx context.Context, req *wallet_api.ValidAddressesRequest) (*wallet_api.ValidAddressesResponse, error) {
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

func (c ChainAdaptor) GetLastestBlock(ctx context.Context, req *wallet_api.LastestBlockRequest) (*wallet_api.LastestBlockResponse, error) {
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

func (c ChainAdaptor) GetBlock(ctx context.Context, req *wallet_api.BlockRequest) (*wallet_api.BlockResponse, error) {
	/*
	 * 目前该方法对于 native token 来说是可以的了，
	 * 但是对于 ERC20 和 ERC721 来说并不够，手续费还没有处理
	 */
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
	for _, bockItem := range rpcBlock.Transactions {
		var fromList []*wallet_api.FromAddress
		var toList []*wallet_api.ToAddress
		fromList = append(fromList, &wallet_api.FromAddress{
			Address: bockItem.From,
			Amount:  bockItem.Value,
		})
		toList = append(toList, &wallet_api.ToAddress{
			Address: bockItem.From,
			Amount:  bockItem.Value,
		})
		txItem := &wallet_api.TransactionList{
			TxHash: bockItem.Hash,
			Fee:    bockItem.GasPrice,
			Status: 0,
			From:   fromList,
			To:     toList,
		}
		transactionList = append(transactionList, txItem)
	}
	if !isError {
		return &wallet_api.BlockResponse{
			Code:         wallet_api.ApiReturnCode_APISUCCESS,
			Msg:          "get block success",
			Height:       rpcBlock.Number,
			Hash:         rpcBlock.Hash.String(),
			Transactions: transactionList,
		}, nil
	}
	return &wallet_api.BlockResponse{
		Code: wallet_api.ApiReturnCode_APIERROR,
		Msg:  "get block failed",
	}, nil
}

func (c ChainAdaptor) GetTransactionByHash(ctx context.Context, req *wallet_api.TransactionByHashRequest) (*wallet_api.TransactionByHashResponse, error) {
	/*
	 * 目前该方法对于 native token 来说是可以的了，
	 * 但是对于 ERC20 和 ERC721 来说并不够，手续费还没有处理
	 */
	tx, err := c.ethClient.TxByHash(common.HexToHash(req.Hash))
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			return &wallet_api.TransactionByHashResponse{
				Code: wallet_api.ApiReturnCode_APIERROR,
				Msg:  "Ethereum Tx NotFound",
			}, nil
		}
		log.Error("get transaction error", "err", err)
		return &wallet_api.TransactionByHashResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  "Ethereum Tx Fetch Error",
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

	if tx.To() == nil {
		toAddress = tx.To().Hex()
		txType = 0 // 创建合约交易
	} else {
		code, err := c.ethClient.EthGetCode(*tx.To())
		if err != nil {
			log.Error("Get transaction code error", "err", err)
			return nil, err
		}
		if code == "0x" {
			txType = 1 // native token 转账
			toAddress = tx.To().Hex()
		} else {
			/*
			 * 判断 calldata 里面前 8 个字节的属于 erc20 还是 erc721 的转账的方法，是可以识别是否这些类型的转账
			 */
			txType = 2
			contractAddress = tx.To().Hex()
			method := tx.Data()[:4]
			if hexutils.BytesToHex(method) == "0xa9059cbb" {
				txType = 3 // ERC20 转账
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
		Address: tx.To().String(),
		Amount:  tx.Value().String(),
	})

	var toList []*wallet_api.ToAddress
	toList = append(toList, &wallet_api.ToAddress{
		Address: tx.To().String(),
		Amount:  tx.Value().String(),
	})

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

func (c ChainAdaptor) GetTransactionByAddress(ctx context.Context, req *wallet_api.TransactionByAddressRequest) (*wallet_api.TransactionByAddressResponse, error) {
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

func (c ChainAdaptor) GetAccountBalance(ctx context.Context, req *wallet_api.AccountBalanceRequest) (*wallet_api.AccountBalanceResponse, error) {
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
		Code:    wallet_api.ApiReturnCode_APIERROR,
		Msg:     "get token balance fail",
		Balance: balanceStr,
	}, nil
}

func (c ChainAdaptor) SendTransaction(ctx context.Context, req *wallet_api.SendTransactionsRequest) (*wallet_api.SendTransactionResponse, error) {
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

func (c ChainAdaptor) BuildTransactionSchema(ctx context.Context, request *wallet_api.TransactionSchemaRequest) (*wallet_api.TransactionSchemaResponse, error) {
	eip1559TxJson := evmbase.Eip1559DynamicFeeTx{}
	return &wallet_api.TransactionSchemaResponse{
		Code:   wallet_api.ApiReturnCode_APISUCCESS,
		Msg:    "build transaction schema success",
		Schema: util.ToJSONString(eip1559TxJson),
	}, nil
}

func (c ChainAdaptor) BuildUnSignTransaction(ctx context.Context, request *wallet_api.UnSignTransactionRequest) (*wallet_api.UnSignTransactionResponse, error) {
	var unsignTxnRet []*wallet_api.UnsignedTransactionMessageHash
	for _, unsignedTxItem := range request.Base64Txn {
		var unsignTx wallet_api.UnsignedTransactionMessageHash
		dFeeTx, _, err := c.buildDynamicFeeTx(unsignedTxItem.Base64Tx)
		if err != nil {
			log.Error("buildDynamicFeeTx failed", "err", err)
			unsignTx = wallet_api.UnsignedTransactionMessageHash{
				UnsignedTx: "",
			}
		}
		log.Info("ethereum BuildUnSignTransaction", "dFeeTx", util.ToJSONString(dFeeTx))
		rawTx, err := evmbase.CreateEip1559UnSignTx(dFeeTx, dFeeTx.ChainID)
		if err != nil {
			log.Error("CreateEip1559UnSignTx failed", "err", err)
			unsignTx = wallet_api.UnsignedTransactionMessageHash{
				UnsignedTx: "",
			}
		}
		unsignTx = wallet_api.UnsignedTransactionMessageHash{
			UnsignedTx: rawTx,
		}
		unsignTxnRet = append(unsignTxnRet, &unsignTx)
	}
	return &wallet_api.UnSignTransactionResponse{
		Code:        wallet_api.ApiReturnCode_APISUCCESS,
		Msg:         "build unsign transaction success",
		UnsignedTxn: unsignTxnRet,
	}, nil
}

func (c ChainAdaptor) BuildSignedTransaction(ctx context.Context, request *wallet_api.SignedTransactionRequest) (*wallet_api.SignedTransactionResponse, error) {
	var signedTransactionList []*wallet_api.SignedTxWithHash
	for _, txWithSignature := range request.TxnWithSignature {
		var signedTransaction wallet_api.SignedTxWithHash
		dFeeTx, dynamicFeeTx, err := c.buildDynamicFeeTx(txWithSignature.Base64Tx)
		if err != nil {
			log.Error("buildDynamicFeeTx failed", "err", err)
		}
		log.Info("ethereum BuildSignedTransaction", "dFeeTx", util.ToJSONString(dFeeTx))
		log.Info("ethereum BuildSignedTransaction", "dynamicFeeTx", util.ToJSONString(dynamicFeeTx))
		log.Info("ethereum BuildSignedTransaction", "req.Signature", txWithSignature.Signature)

		// Decode signature and create signed transaction
		inputSignatureByteList, err := hex.DecodeString(txWithSignature.Signature)
		if err != nil {
			log.Error("decode signature failed", "err", err)
		}

		signer, signedTx, rawTx, txHash, err := evmbase.CreateEip1559SignedTx(dFeeTx, inputSignatureByteList, dFeeTx.ChainID)
		if err != nil {
			log.Error("create signed tx fail", "err", err)
			signedTransaction = wallet_api.SignedTxWithHash{
				IsSuccess: false,
				SignedTx:  rawTx,
				TxHash:    txHash,
			}
		} else {
			signedTransaction = wallet_api.SignedTxWithHash{
				IsSuccess: true,
				SignedTx:  rawTx,
				TxHash:    txHash,
			}
		}

		log.Info("ethereum BuildSignedTransaction", "rawTx", rawTx)

		sender, err := types.Sender(signer, signedTx)
		if err != nil {
			log.Error("recover sender failed", "err", err)
			return nil, fmt.Errorf("recover sender failed: %w", err)
		}

		if sender.Hex() != dynamicFeeTx.FromAddress {
			log.Error("sender mismatch",
				"expected", dynamicFeeTx.FromAddress,
				"got", sender.Hex(),
			)
			return nil, fmt.Errorf("sender address mismatch: expected %s, got %s",
				dynamicFeeTx.FromAddress,
				sender.Hex(),
			)
		}
		log.Info("ethereum BuildSignedTransaction", "sender", sender.Hex())
		signedTransactionList = append(signedTransactionList, &signedTransaction)
	}

	return &wallet_api.SignedTransactionResponse{
		Code:      wallet_api.ApiReturnCode_APISUCCESS,
		Msg:       "build signed transaction success",
		SignedTxn: signedTransactionList,
	}, nil
}

func (c *ChainAdaptor) buildDynamicFeeTx(base64Tx string) (*types.DynamicFeeTx, *evmbase.Eip1559DynamicFeeTx, error) {
	// 1. Decode base64 string
	txReqJsonByte, err := base64.StdEncoding.DecodeString(base64Tx)
	if err != nil {
		log.Error("decode string fail", "err", err)
		return nil, nil, err
	}

	// 2. Unmarshal JSON to struct
	var dynamicFeeTx evmbase.Eip1559DynamicFeeTx
	if err := json.Unmarshal(txReqJsonByte, &dynamicFeeTx); err != nil {
		log.Error("parse json fail", "err", err)
		return nil, nil, err
	}

	// 3. Convert string values to big.Int
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

	// 4. Handle addresses and data
	toAddress := common.HexToAddress(dynamicFeeTx.ToAddress)
	var finalToAddress common.Address
	var finalAmount *big.Int
	var buildData []byte
	log.Info("contract address check",
		"contractAddress", dynamicFeeTx.ContractAddress,
		"isEthTransfer", evmbase.IsEthTransfer(&dynamicFeeTx),
	)

	// 5. Handle contract interaction vs direct transfer
	if evmbase.IsEthTransfer(&dynamicFeeTx) {
		finalToAddress = toAddress
		finalAmount = amount
	} else {
		contractAddress := common.HexToAddress(dynamicFeeTx.ContractAddress)
		buildData = evmbase.BuildErc20Data(toAddress, amount)
		finalToAddress = contractAddress
		finalAmount = big.NewInt(0)
	}

	// 6. Create dynamic fee transaction
	dFeeTx := &types.DynamicFeeTx{
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

func (c ChainAdaptor) GetAddressApproveList(ctx context.Context, request *wallet_api.AddressApproveListRequest) (*wallet_api.AddressApproveListResponse, error) {
	return &wallet_api.AddressApproveListResponse{
		Code: wallet_api.ApiReturnCode_APISUCCESS,
		Msg:  "don't support in this stage, support in the future",
	}, nil
}
