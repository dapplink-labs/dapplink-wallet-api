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

	common2 "github.com/dapplink-labs/dapplink-wallet-api/protobuf/common"
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
	"github.com/dapplink-labs/dapplink-wallet-api/protobuf/walletapi"
)

const (
	ChainID              string = "DappLinkEthereum"
	NativeTokenGasLimit  uint64 = 21000
	Erc20TokenGasLimit   uint64 = 120000
	MaxFeePerGas         int64  = 105000000000
	MaxPriorityFeePerGas int64  = 75000000000
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

func (c ChainAdaptor) ConvertAddresses(ctx context.Context, req *walletapi.ConvertAddressesRequest) (*walletapi.ConvertAddressesResponse, error) {
	var retAddressList []*walletapi.Addresses
	for _, publicKeyItem := range req.PublicKey {
		var addressItem *walletapi.Addresses
		publicKeyBytes, err := hex.DecodeString(publicKeyItem.PublicKey)
		if err != nil {
			addressItem = &walletapi.Addresses{
				Address: "",
			}
			log.Error("decode public key fail", "err", err)
		} else {
			addressCommon := common.BytesToAddress(crypto.Keccak256(publicKeyBytes[1:])[12:])
			log.Info("convert addresses", "address", addressCommon.String())
			addressItem = &walletapi.Addresses{
				Address: addressCommon.String(),
			}
		}
		retAddressList = append(retAddressList, addressItem)
	}
	return &walletapi.ConvertAddressesResponse{
		Code:    common2.ReturnCode_SUCCESS,
		Msg:     "success",
		Address: retAddressList,
	}, nil
}

func (c ChainAdaptor) ValidAddresses(ctx context.Context, req *walletapi.ValidAddressesRequest) (*walletapi.ValidAddressesResponse, error) {
	var retAddressesValid []*walletapi.AddressesValid
	for _, addressItem := range req.Addresses {
		var addressesValidItem walletapi.AddressesValid
		addressesValidItem.Address = addressItem.GetAddress()
		ok := regexp.MustCompile("^[0-9a-fA-F]{40}$").MatchString(addressItem.GetAddress()[2:])
		if len(addressItem.GetAddress()) != 42 || !strings.HasPrefix(addressItem.GetAddress(), "0x") || !ok {
			addressesValidItem.Valid = false
		} else {
			addressesValidItem.Valid = true
		}
		retAddressesValid = append(retAddressesValid, &addressesValidItem)
	}
	return &walletapi.ValidAddressesResponse{
		Code:         common2.ReturnCode_SUCCESS,
		Msg:          "success",
		AddressValid: retAddressesValid,
	}, nil
}

func (c ChainAdaptor) GetLastestBlock(ctx context.Context, req *walletapi.LastestBlockRequest) (*walletapi.LastestBlockResponse, error) {
	latestBock, err := c.ethClient.BlockHeaderByNumber(nil)
	if err != nil {
		log.Error("Get latest block fail", "err", err)
		return nil, err
	}
	return &walletapi.LastestBlockResponse{
		Code:       common2.ReturnCode_SUCCESS,
		Msg:        "get lastest block success",
		Hash:       latestBock.Hash().String(),
		Height:     latestBock.Number.Uint64(),
		ParentHash: hex.EncodeToString(latestBock.ParentHash[:]),
		Timestamp:  latestBock.Time,
	}, nil
}

func (c ChainAdaptor) GetBlock(ctx context.Context, req *walletapi.BlockRequest) (*walletapi.BlockResponse, error) {
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
	var transactionList []*walletapi.TransactionList
	for _, bockItem := range rpcBlock.Transactions {
		var fromList []*walletapi.FromAddress
		var toList []*walletapi.ToAddress
		fromList = append(fromList, &walletapi.FromAddress{
			Address: bockItem.From,
			Amount:  bockItem.Value,
		})
		toList = append(toList, &walletapi.ToAddress{
			Address: bockItem.From,
			Amount:  bockItem.Value,
		})
		txItem := &walletapi.TransactionList{
			TxHash: bockItem.Hash,
			Fee:    bockItem.GasPrice,
			Status: 0,
			From:   fromList,
			To:     toList,
		}
		transactionList = append(transactionList, txItem)
	}
	if !isError {
		return &walletapi.BlockResponse{
			Code:         common2.ReturnCode_SUCCESS,
			Msg:          "get block success",
			Height:       rpcBlock.Number,
			Hash:         rpcBlock.Hash.String(),
			Transactions: transactionList,
		}, nil
	}
	return &walletapi.BlockResponse{
		Code: common2.ReturnCode_ERROR,
		Msg:  "get block failed",
	}, nil
}

func (c ChainAdaptor) GetTransactionByHash(ctx context.Context, req *walletapi.TransactionByHashRequest) (*walletapi.TransactionByHashResponse, error) {
	/*
	 * 目前该方法对于 native token 来说是可以的了，
	 * 但是对于 ERC20 和 ERC721 来说并不够，手续费还没有处理
	 */
	tx, err := c.ethClient.TxByHash(common.HexToHash(req.Hash))
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			return &walletapi.TransactionByHashResponse{
				Code: common2.ReturnCode_ERROR,
				Msg:  "Ethereum Tx NotFound",
			}, nil
		}
		log.Error("get transaction error", "err", err)
		return &walletapi.TransactionByHashResponse{
			Code: common2.ReturnCode_ERROR,
			Msg:  "Ethereum Tx Fetch Error",
		}, nil
	}
	receipt, err := c.ethClient.TxReceiptByHash(common.HexToHash(req.Hash))
	if err != nil {
		log.Error("get transaction receipt error", "err", err)
		return &walletapi.TransactionByHashResponse{
			Code: common2.ReturnCode_ERROR,
			Msg:  "Get transaction receipt error",
		}, nil
	}
	var toAddress string
	var contractAddress string
	var txType uint32
	var txStatus walletapi.TxStatus

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
		txStatus = walletapi.TxStatus_Success
	} else {
		txStatus = walletapi.TxStatus_Failed
	}
	fee := new(big.Int).Mul(receipt.EffectiveGasPrice, big.NewInt(int64(receipt.GasUsed)))

	log.Info("tx information", "fee", fee.String(), "toAddress", toAddress, "txStatus", txStatus)
	var fromList []*walletapi.FromAddress
	fromList = append(fromList, &walletapi.FromAddress{
		Address: tx.To().String(),
		Amount:  tx.Value().String(),
	})

	var toList []*walletapi.ToAddress
	toList = append(toList, &walletapi.ToAddress{
		Address: tx.To().String(),
		Amount:  tx.Value().String(),
	})

	return &walletapi.TransactionByHashResponse{
		Code: common2.ReturnCode_SUCCESS,
		Msg:  "get transaction success",
		Transaction: &walletapi.TransactionList{
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

func (c ChainAdaptor) GetTransactionByAddress(ctx context.Context, req *walletapi.TransactionByAddressRequest) (*walletapi.TransactionByAddressResponse, error) {
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
		return &walletapi.TransactionByAddressResponse{
			Code:        common2.ReturnCode_ERROR,
			Msg:         "get tx list fail",
			Transaction: nil,
		}, err
	} else {
		txs := resp.TransactionList
		list := make([]*walletapi.TransactionList, 0, len(txs))
		for i := 0; i < len(txs); i++ {
			var fromList []*walletapi.FromAddress
			var toList []*walletapi.ToAddress
			fromList = append(fromList, &walletapi.FromAddress{
				Address: txs[i].From,
				Amount:  txs[i].Amount,
			})
			toList = append(toList, &walletapi.ToAddress{
				Address: txs[i].To,
				Amount:  txs[i].Amount,
			})
			list = append(list, &walletapi.TransactionList{
				TxHash: txs[i].TxId,
				To:     toList,
				From:   fromList,
				Fee:    txs[i].TxFee,
				Status: 1,
				TxType: txType,
			})
		}
		return &walletapi.TransactionByAddressResponse{
			Code:        common2.ReturnCode_SUCCESS,
			Msg:         "get tx list by address success",
			Transaction: list,
		}, nil
	}
}

func (c ChainAdaptor) GetAccountBalance(ctx context.Context, req *walletapi.AccountBalanceRequest) (*walletapi.AccountBalanceResponse, error) {
	balanceResult, err := c.ethDataClient.GetBalanceByAddress(req.ContractAddress, req.Address)
	if err != nil {
		return &walletapi.AccountBalanceResponse{
			Code:    common2.ReturnCode_ERROR,
			Msg:     "get token balance fail",
			Balance: "0",
		}, nil
	}
	log.Info("balance result", "balance=", balanceResult.Balance, "balanceStr=", balanceResult.BalanceStr)
	balanceStr := "0"
	if balanceResult.Balance != nil && balanceResult.Balance.Int() != nil {
		balanceStr = balanceResult.Balance.Int().String()
	}
	return &walletapi.AccountBalanceResponse{
		Code:    common2.ReturnCode_ERROR,
		Msg:     "get token balance fail",
		Balance: balanceStr,
	}, nil
}

func (c ChainAdaptor) SendTransaction(ctx context.Context, req *walletapi.SendTransactionsRequest) (*walletapi.SendTransactionResponse, error) {
	var txListRet []*walletapi.RawTransactionReturn
	for _, txItem := range req.RawTx {
		var txRet walletapi.RawTransactionReturn
		transaction, err := c.ethClient.SendRawTransaction(txItem.RawTx)
		if err != nil {
			txRet = walletapi.RawTransactionReturn{
				TxHash:    "",
				IsSuccess: false,
				Message:   "this tx send failed",
			}
		} else {
			txRet = walletapi.RawTransactionReturn{
				TxHash:    transaction.String(),
				IsSuccess: true,
				Message:   "this tx send success",
			}
		}
		txListRet = append(txListRet, &txRet)
	}
	return &walletapi.SendTransactionResponse{
		Code:   common2.ReturnCode_SUCCESS,
		Msg:    "send tx success",
		TxnRet: txListRet,
	}, nil
}

func (c ChainAdaptor) BuildTransactionSchema(ctx context.Context, request *walletapi.TransactionSchemaRequest) (*walletapi.TransactionSchemaResponse, error) {
	eip1559TxJson := evmbase.Eip1559DynamicFeeTx{}
	return &walletapi.TransactionSchemaResponse{
		Code:   common2.ReturnCode_SUCCESS,
		Msg:    "build transaction schema success",
		Schema: util.ToJSONString(eip1559TxJson),
	}, nil
}

func (c ChainAdaptor) BuildUnSignTransaction(ctx context.Context, request *walletapi.UnSignTransactionRequest) (*walletapi.UnSignTransactionResponse, error) {
	var unsignTxnRet []*walletapi.UnsignedTransactionMessageHash
	for _, unsignedTxItem := range request.Base64Txn {
		var unsignTx walletapi.UnsignedTransactionMessageHash
		dFeeTx, _, err := c.buildDynamicFeeTx(unsignedTxItem.Base64Tx)
		if err != nil {
			log.Error("buildDynamicFeeTx failed", "err", err)
			unsignTx = walletapi.UnsignedTransactionMessageHash{
				UnsignedTx: "",
			}
		}
		log.Info("ethereum BuildUnSignTransaction", "dFeeTx", util.ToJSONString(dFeeTx))
		rawTx, err := evmbase.CreateEip1559UnSignTx(dFeeTx, dFeeTx.ChainID)
		if err != nil {
			log.Error("CreateEip1559UnSignTx failed", "err", err)
			unsignTx = walletapi.UnsignedTransactionMessageHash{
				UnsignedTx: "",
			}
		}
		unsignTx = walletapi.UnsignedTransactionMessageHash{
			UnsignedTx: rawTx,
		}
		unsignTxnRet = append(unsignTxnRet, &unsignTx)
	}
	return &walletapi.UnSignTransactionResponse{
		Code:        common2.ReturnCode_SUCCESS,
		Msg:         "build unsign transaction success",
		UnsignedTxn: unsignTxnRet,
	}, nil
}

func (c ChainAdaptor) BuildSignedTransaction(ctx context.Context, request *walletapi.SignedTransactionRequest) (*walletapi.SignedTransactionResponse, error) {
	var signedTransactionList []*walletapi.SignedTxWithHash
	for _, txWithSignature := range request.TxnWithSignature {
		var signedTransaction walletapi.SignedTxWithHash
		dFeeTx, dynamicFeeTx, err := c.buildDynamicFeeTx(txWithSignature.Base64Tx)
		if err != nil {
			log.Error("buildDynamicFeeTx failed", "err", err)
		}
		log.Info("ethereum BuildSignedTransaction", "dFeeTx", util.ToJSONString(dFeeTx))
		log.Info("ethereum BuildSignedTransaction", "dynamicFeeTx", util.ToJSONString(dynamicFeeTx))
		log.Info("ethereum BuildSignedTransaction", "req.Signature", txWithSignature.Signature)

		inputSignatureByteList, err := hex.DecodeString(txWithSignature.Signature)
		if err != nil {
			log.Error("decode signature failed", "err", err)
		}

		signer, signedTx, rawTx, txHash, err := evmbase.CreateEip1559SignedTx(dFeeTx, inputSignatureByteList, dFeeTx.ChainID)
		if err != nil {
			log.Error("create signed tx fail", "err", err)
			signedTransaction = walletapi.SignedTxWithHash{
				IsSuccess: false,
				SignedTx:  rawTx,
				TxHash:    txHash,
			}
		} else {
			signedTransaction = walletapi.SignedTxWithHash{
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

	return &walletapi.SignedTransactionResponse{
		Code:      common2.ReturnCode_SUCCESS,
		Msg:       "build signed transaction success",
		SignedTxn: signedTransactionList,
	}, nil
}

func (c *ChainAdaptor) buildDynamicFeeTx(base64Tx string) (*types.DynamicFeeTx, *evmbase.Eip1559DynamicFeeTx, error) {
	txReqJsonByte, err := base64.StdEncoding.DecodeString(base64Tx)
	if err != nil {
		log.Error("decode string fail", "err", err)
		return nil, nil, err
	}

	var dynamicFeeTx evmbase.Eip1559DynamicFeeTx
	if err := json.Unmarshal(txReqJsonByte, &dynamicFeeTx); err != nil {
		log.Error("parse json fail", "err", err)
		return nil, nil, err
	}

	chainID := new(big.Int)
	amount := new(big.Int)

	if _, ok := chainID.SetString(dynamicFeeTx.ChainId, 10); !ok {
		return nil, nil, fmt.Errorf("invalid chain ID: %s", dynamicFeeTx.ChainId)
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

	var GasLimit uint64
	if evmbase.IsEthTransfer(&dynamicFeeTx) {
		finalToAddress = toAddress
		finalAmount = amount
		GasLimit = NativeTokenGasLimit
	} else {
		contractAddress := common.HexToAddress(dynamicFeeTx.ContractAddress)
		buildData = evmbase.BuildErc20Data(toAddress, amount)
		finalToAddress = contractAddress
		finalAmount = big.NewInt(0)
		GasLimit = Erc20TokenGasLimit
	}

	txNonce, err := c.ethClient.GetTransactionAccount(common.HexToAddress(dynamicFeeTx.FromAddress))
	if err != nil {
		log.Error("get address nonce fail", "err", err)
		return nil, nil, err
	}

	dFeeTx := &types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     txNonce.Uint64(),
		GasTipCap: big.NewInt(int64(MaxPriorityFeePerGas)),
		GasFeeCap: big.NewInt(int64(MaxFeePerGas)),
		Gas:       GasLimit,
		To:        &finalToAddress,
		Value:     finalAmount,
		Data:      buildData,
	}
	return dFeeTx, &dynamicFeeTx, nil
}

func (c ChainAdaptor) GetAddressApproveList(ctx context.Context, request *walletapi.AddressApproveListRequest) (*walletapi.AddressApproveListResponse, error) {
	return &walletapi.AddressApproveListResponse{
		Code: common2.ReturnCode_SUCCESS,
		Msg:  "don't support in this stage, support in the future",
	}, nil
}
