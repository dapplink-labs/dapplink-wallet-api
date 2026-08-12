package solana

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/log"
	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/token"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/dapplink-labs/dapplink-wallet-api/chain"
	"github.com/dapplink-labs/dapplink-wallet-api/config"
	"github.com/dapplink-labs/dapplink-wallet-api/protobuf/common"
	"github.com/dapplink-labs/dapplink-wallet-api/protobuf/walletapi"
)

const (
	ChainID string = "DappLinkSolana"
)

type ChainAdaptor struct {
	solCli    SolClient
	sdkClient *rpc.Client
	solData   *SolData
}

func NewChainAdaptor(conf *config.Config) (chain.IChainAdaptor, error) {
	rpcUrl := conf.WalletNode.Sol.RpcUrl

	solHttpCli, err := NewSolHttpClient(rpcUrl)
	if err != nil {
		log.Error("Dial solana client fail", "err", err)
		return nil, err
	}
	dataApiUrl := conf.WalletNode.Sol.DataApiUrl
	dataApiKey := conf.WalletNode.Sol.DataApiKey
	dataApiTimeOut := conf.WalletNode.Sol.TimeOut
	solData, err := NewSolScanClient(dataApiUrl, dataApiKey, time.Duration(dataApiTimeOut))
	if err != nil {
		log.Error("new solana data client fail", "err", err)
		return nil, err
	}

	sdkClient := rpc.New(rpcUrl)

	return &ChainAdaptor{
		solCli:    solHttpCli,
		sdkClient: sdkClient,
		solData:   solData,
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
			pubKey := solana.PublicKeyFromBytes(publicKeyBytes)
			log.Info("convert addresses", "address", pubKey.String())
			addressItem = &walletapi.Addresses{
				Address:   pubKey.String(),
				PublicKey: publicKeyItem.PublicKey,
				Type:      publicKeyItem.Type,
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
	for _, address := range req.Addresses {
		_, err := solana.PublicKeyFromBase58(address.Address)
		if err != nil {
			retAddressList = append(retAddressList, &walletapi.AddressesValid{
				Address: address.Address,
				Valid:   false,
			})
		} else {
			retAddressList = append(retAddressList, &walletapi.AddressesValid{
				Address: address.Address,
				Valid:   true,
			})
		}
	}
	return &walletapi.ValidAddressesResponse{
		Code:         common.ReturnCode_SUCCESS,
		Msg:          "success",
		AddressValid: retAddressList,
	}, nil
}

func (c *ChainAdaptor) GetLastestBlock(ctx context.Context, req *walletapi.LastestBlockRequest) (*walletapi.LastestBlockResponse, error) {
	slot, err := c.solCli.GetSlot(Finalized)
	if err != nil {
		log.Error("get latest slot fail", "err", err)
		return &walletapi.LastestBlockResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  err.Error(),
		}, err
	}

	return &walletapi.LastestBlockResponse{
		Code:   common.ReturnCode_SUCCESS,
		Msg:    "success",
		Height: slot,
	}, nil
}

func (c *ChainAdaptor) GetBlock(ctx context.Context, req *walletapi.BlockRequest) (*walletapi.BlockResponse, error) {
	slot, err := strconv.ParseUint(req.HashHeight, 10, 64)
	if err != nil {
		log.Error("parse slot fail", "err", err)
		return &walletapi.BlockResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  err.Error(),
		}, err
	}

	blockResult, err := c.solCli.GetBlockBySlot(slot, Full)
	if err != nil {
		log.Error("get block by slot fail", "err", err)
		return &walletapi.BlockResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  err.Error(),
		}, err
	}

	var txList []*walletapi.TransactionList
	if blockResult.Transactions != nil {
		for _, tx := range blockResult.Transactions {
			txList = append(txList, &walletapi.TransactionList{
				TxHash: tx.Signature,
			})
		}
	}

	return &walletapi.BlockResponse{
		Code:         common.ReturnCode_SUCCESS,
		Msg:          "success",
		Height:       strconv.FormatUint(slot, 10),
		Hash:         blockResult.BlockHash,
		Transactions: txList,
	}, nil
}

func (c *ChainAdaptor) GetBatchBlock(ctx context.Context, req *walletapi.BatchBlockRequest) (*walletapi.BatchBlockResponse, error) {
	startHeight, err := strconv.ParseUint(req.NextHeight, 10, 64)
	if err != nil {
		return &walletapi.BatchBlockResponse{Code: common.ReturnCode_ERROR, Msg: "invalid next height"}, nil
	}
	endHeight, err := strconv.ParseUint(req.EndHeight, 10, 64)
	if err != nil {
		return &walletapi.BatchBlockResponse{Code: common.ReturnCode_ERROR, Msg: "invalid end height"}, nil
	}
	if startHeight > endHeight {
		return &walletapi.BatchBlockResponse{Code: common.ReturnCode_ERROR, Msg: "next height is greater than end height"}, nil
	}

	if !req.WithTransactions {
		headers := make([]*walletapi.BlockHeaderInfo, 0, endHeight-startHeight+1)
		for slot := startHeight; slot <= endHeight; slot++ {
			blockResult, blockErr := c.solCli.GetBlockBySlot(slot, None)
			if blockErr != nil {
				// Empty/skipped slots are common on Solana; continue.
				log.Debug("skip empty slot header", "slot", slot, "err", blockErr)
				continue
			}
			headers = append(headers, &walletapi.BlockHeaderInfo{
				Height:     strconv.FormatUint(slot, 10),
				Hash:       blockResult.BlockHash,
				ParentHash: blockResult.PreviousBlockhash,
				Timestamp:  uint64(blockResult.BlockTime),
			})
		}
		return &walletapi.BatchBlockResponse{
			Code:    common.ReturnCode_SUCCESS,
			Msg:     "get batch block success",
			Headers: headers,
		}, nil
	}

	blocks := make([]*walletapi.BlockWithTransactions, 0, endHeight-startHeight+1)
	for slot := startHeight; slot <= endHeight; slot++ {
		// Use signatures detail for reliable signature lists, then hydrate transfers.
		blockResult, blockErr := c.solCli.GetBlockBySlot(slot, Signatures)
		if blockErr != nil {
			log.Debug("skip empty slot", "slot", slot, "err", blockErr)
			continue
		}
		blocks = append(blocks, &walletapi.BlockWithTransactions{
			Height:       strconv.FormatUint(slot, 10),
			Hash:         blockResult.BlockHash,
			ParentHash:   blockResult.PreviousBlockhash,
			Timestamp:    uint64(blockResult.BlockTime),
			Transactions: c.parseBlockTransactions(blockResult),
		})
	}
	return &walletapi.BatchBlockResponse{
		Code:   common.ReturnCode_SUCCESS,
		Msg:    "get batch block success",
		Blocks: blocks,
	}, nil
}

func (c *ChainAdaptor) parseBlockTransactions(blockResult *BlockResult) []*walletapi.TransactionList {
	if blockResult == nil {
		return nil
	}
	sigs := collectBlockSignatures(blockResult)
	if len(sigs) == 0 {
		return nil
	}
	txResults, err := c.solCli.GetTransactionRange(sigs)
	if err != nil {
		log.Warn("get transaction range for block failed, fallback to hashes only", "err", err, "count", len(sigs))
		out := make([]*walletapi.TransactionList, 0, len(sigs))
		for _, hash := range sigs {
			out = append(out, &walletapi.TransactionList{TxHash: hash})
		}
		return out
	}
	out := make([]*walletapi.TransactionList, 0, len(txResults))
	for _, txResult := range txResults {
		if txResult == nil {
			continue
		}
		hash := ""
		if len(txResult.Transaction.Signatures) > 0 {
			hash = txResult.Transaction.Signatures[0]
		}
		transfer := parseTransactionResult(txResult)
		out = append(out, toWalletTransaction(hash, transfer, txResult.Meta.Err))
	}
	return out
}

func collectBlockSignatures(blockResult *BlockResult) []string {
	if len(blockResult.Signatures) > 0 {
		return blockResult.Signatures
	}
	out := make([]string, 0, len(blockResult.Transactions))
	for _, tx := range blockResult.Transactions {
		if tx.Signature != "" {
			out = append(out, tx.Signature)
			continue
		}
		// json encoding often nests signatures under transaction.signatures
		raw, _ := json.Marshal(tx.Message)
		_ = raw
	}
	return out
}

func (c *ChainAdaptor) GetTransactionByHash(ctx context.Context, req *walletapi.TransactionByHashRequest) (*walletapi.TransactionByHashResponse, error) {
	txResult, err := c.solCli.GetTransaction(req.Hash)
	if err != nil {
		log.Error("get transaction fail", "err", err)
		return &walletapi.TransactionByHashResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  err.Error(),
		}, err
	}
	transfer := parseTransactionResult(txResult)
	return &walletapi.TransactionByHashResponse{
		Code:        common.ReturnCode_SUCCESS,
		Msg:         "success",
		Transaction: toWalletTransaction(req.Hash, transfer, txResult.Meta.Err),
	}, nil
}

func (c *ChainAdaptor) GetTransactionByAddress(ctx context.Context, req *walletapi.TransactionByAddressRequest) (*walletapi.TransactionByAddressResponse, error) {
	pageSize := req.PageSize
	if pageSize == 0 {
		pageSize = 10
	}

	signatures, err := c.solCli.GetTxForAddress(req.Address, Finalized, pageSize, "", "")
	if err != nil {
		log.Error("get transactions for address fail", "err", err)
		return &walletapi.TransactionByAddressResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  err.Error(),
		}, err
	}

	var txList []*walletapi.TransactionList
	for _, sig := range signatures {
		txList = append(txList, &walletapi.TransactionList{
			TxHash: sig.Signature,
		})
	}

	return &walletapi.TransactionByAddressResponse{
		Code:        common.ReturnCode_SUCCESS,
		Msg:         "success",
		Transaction: txList,
	}, nil
}

func (c *ChainAdaptor) GetAccountBalance(ctx context.Context, req *walletapi.AccountBalanceRequest) (*walletapi.AccountBalanceResponse, error) {
	if req.ContractAddress == "" || req.ContractAddress == "So11111111111111111111111111111111111111112" {
		balance, err := c.solCli.GetBalance(req.Address)
		if err != nil {
			log.Error("get balance fail", "err", err)
			return &walletapi.AccountBalanceResponse{
				Code: common.ReturnCode_ERROR,
				Msg:  err.Error(),
			}, err
		}

		return &walletapi.AccountBalanceResponse{
			Code:    common.ReturnCode_SUCCESS,
			Msg:     "success",
			Balance: strconv.FormatUint(balance, 10),
		}, nil
	} else {
		ownerPubkey, err := solana.PublicKeyFromBase58(req.Address)
		if err != nil {
			log.Error("invalid address", "err", err)
			return &walletapi.AccountBalanceResponse{
				Code: common.ReturnCode_ERROR,
				Msg:  err.Error(),
			}, err
		}

		mintPubkey, err := solana.PublicKeyFromBase58(req.ContractAddress)
		if err != nil {
			log.Error("invalid contract address", "err", err)
			return &walletapi.AccountBalanceResponse{
				Code: common.ReturnCode_ERROR,
				Msg:  err.Error(),
			}, err
		}

		ata, _, err := solana.FindAssociatedTokenAddress(ownerPubkey, mintPubkey)
		if err != nil {
			log.Error("find associated token address fail", "err", err)
			return &walletapi.AccountBalanceResponse{
				Code: common.ReturnCode_ERROR,
				Msg:  err.Error(),
			}, err
		}

		accountInfo, err := GetAccountInfo(c.sdkClient, ata)
		if err != nil {
			// Distinguish missing ATA from zero-balance ATA so fee-payer collection can create it.
			return &walletapi.AccountBalanceResponse{
				Code: common.ReturnCode_ERROR,
				Msg:  "associated token account not found",
			}, fmt.Errorf("associated token account not found: %w", err)
		}

		var tokenAccount token.Account
		decoder := bin.NewBinDecoder(accountInfo.GetBinary())
		err = tokenAccount.UnmarshalWithDecoder(decoder)
		if err != nil {
			log.Error("unmarshal token account fail", "err", err)
			return &walletapi.AccountBalanceResponse{
				Code: common.ReturnCode_ERROR,
				Msg:  err.Error(),
			}, err
		}

		return &walletapi.AccountBalanceResponse{
			Code:    common.ReturnCode_SUCCESS,
			Msg:     "success",
			Balance: strconv.FormatUint(tokenAccount.Amount, 10),
		}, nil
	}
}

func (c *ChainAdaptor) SendTransaction(ctx context.Context, req *walletapi.SendTransactionsRequest) (*walletapi.SendTransactionResponse, error) {
	var txnRetList []*walletapi.RawTransactionReturn

	for _, rawTx := range req.RawTx {
		config := &SendTransactionRequest{
			Encoding:            "base64",
			SkipPreflight:       false,
			PreflightCommitment: string(Finalized),
			MaxRetries:          3,
			MinContextSlot:      0,
		}

		txHash, err := c.solCli.SendTransaction(rawTx.RawTx, config)
		if err != nil {
			log.Error("send transaction fail", "err", err)
			txnRetList = append(txnRetList, &walletapi.RawTransactionReturn{
				TxHash:    "",
				IsSuccess: false,
				Message:   err.Error(),
			})
		} else {
			txnRetList = append(txnRetList, &walletapi.RawTransactionReturn{
				TxHash:    txHash,
				IsSuccess: true,
				Message:   "success",
			})
		}
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
	recentBlockhash, err := c.solCli.GetLatestBlockhash(Confirmed)
	if err != nil {
		log.Error("get latest blockhash fail", "err", err)
		return &walletapi.UnSignTransactionResponse{Code: common.ReturnCode_ERROR, Msg: err.Error()}, err
	}

	var unsignedTxList []*walletapi.UnsignedTransactionMessageHash
	for _, base64Txn := range request.Base64Txn {
		jsonBytes, err := base64.StdEncoding.DecodeString(base64Txn.Base64Tx)
		if err != nil {
			log.Error("decode base64 payload fail", "err", err)
			return &walletapi.UnSignTransactionResponse{Code: common.ReturnCode_ERROR, Msg: err.Error()}, err
		}
		var payload SolanaTransferTx
		if err := json.Unmarshal(jsonBytes, &payload); err != nil {
			log.Error("unmarshal solana transfer payload fail", "err", err)
			return &walletapi.UnSignTransactionResponse{Code: common.ReturnCode_ERROR, Msg: err.Error()}, err
		}
		tx, err := buildSolanaTransferTransaction(payload, recentBlockhash)
		if err != nil {
			log.Error("build solana transfer fail", "err", err)
			return &walletapi.UnSignTransactionResponse{Code: common.ReturnCode_ERROR, Msg: err.Error()}, err
		}
		env, err := envelopeFromTransaction(tx)
		if err != nil {
			log.Error("envelope from transaction fail", "err", err)
			return &walletapi.UnSignTransactionResponse{Code: common.ReturnCode_ERROR, Msg: err.Error()}, err
		}
		encoded, err := encodeEnvelope(env)
		if err != nil {
			return &walletapi.UnSignTransactionResponse{Code: common.ReturnCode_ERROR, Msg: err.Error()}, err
		}
		// UnsignedTx is the envelope; MessageHash for signing is env.MessageHex (extracted by wallet-service).
		unsignedTxList = append(unsignedTxList, &walletapi.UnsignedTransactionMessageHash{
			UnsignedTx: encoded,
		})
	}

	return &walletapi.UnSignTransactionResponse{
		Code:        common.ReturnCode_SUCCESS,
		Msg:         "success",
		UnsignedTxn: unsignedTxList,
	}, nil
}

func (c *ChainAdaptor) BuildSignedTransaction(ctx context.Context, request *walletapi.SignedTransactionRequest) (*walletapi.SignedTransactionResponse, error) {
	// Group by Base64Tx so multiple signer signatures can be merged for fee-payer flows.
	type pending struct {
		env *UnsignedEnvelope
	}
	grouped := make(map[string]*pending)
	order := make([]string, 0)

	for _, txnWithSig := range request.TxnWithSignature {
		env, err := decodeEnvelope(txnWithSig.Base64Tx)
		if err != nil {
			log.Error("decode envelope fail", "err", err)
			continue
		}
		p, ok := grouped[txnWithSig.Base64Tx]
		if !ok {
			p = &pending{env: env}
			grouped[txnWithSig.Base64Tx] = p
			order = append(order, txnWithSig.Base64Tx)
		}
		signerKey := strings.TrimSpace(txnWithSig.PublicKey)
		if signerKey != "" {
			if addr, err := publicKeyHexToBase58(signerKey); err == nil {
				signerKey = addr
			} else if _, err2 := solana.PublicKeyFromBase58(signerKey); err2 != nil {
				log.Error("invalid signer public key", "publicKey", txnWithSig.PublicKey, "err", err)
				continue
			}
		}
		if signerKey == "" && len(p.env.SignerKeys) == 1 {
			signerKey = p.env.SignerKeys[0]
		}
		if signerKey == "" {
			log.Error("missing signer public key for solana signed tx")
			continue
		}
		p.env.Signatures[signerKey] = strings.TrimSpace(txnWithSig.Signature)
	}

	var signedTxList []*walletapi.SignedTxWithHash
	for _, key := range order {
		p := grouped[key]
		tx, txHash, err := assembleSignedTransaction(p.env)
		if err != nil {
			log.Error("assemble signed solana tx fail", "err", err)
			signedTxList = append(signedTxList, &walletapi.SignedTxWithHash{
				SignedTx:  "",
				TxHash:    "",
				IsSuccess: false,
			})
			continue
		}
		signedB64, err := serializeSignedTxBase64(tx)
		if err != nil {
			signedTxList = append(signedTxList, &walletapi.SignedTxWithHash{
				SignedTx:  "",
				TxHash:    "",
				IsSuccess: false,
			})
			continue
		}
		signedTxList = append(signedTxList, &walletapi.SignedTxWithHash{
			SignedTx:  signedB64,
			TxHash:    txHash,
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
	return &walletapi.CallContractResponse{Code: common.ReturnCode_ERROR, Msg: "callContract not supported on Solana"}, nil
}

func PubKeyHexToAddress(pubKeyHex string) (string, error) {
	pubKeyHex = strings.TrimPrefix(pubKeyHex, "0x")
	pubKeyBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return "", err
	}
	pubKey := solana.PublicKeyFromBytes(pubKeyBytes)
	return pubKey.String(), nil
}

func isSolTransfer(coinAddress string) bool {
	return coinAddress == "" ||
		coinAddress == "So11111111111111111111111111111111111111112"
}

func getPrivateKey(keyStr string) (solana.PrivateKey, error) {
	if prikey, err := solana.PrivateKeyFromBase58(keyStr); err == nil {
		return prikey, nil
	}
	if bytes, err := hex.DecodeString(keyStr); err == nil {
		return solana.PrivateKey(bytes), nil
	}
	return nil, fmt.Errorf("invalid private key format")
}
