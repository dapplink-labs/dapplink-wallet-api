package bitcoin

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/dapplink-labs/dapplink-wallet-api/chain/bitcoin/types"
	"github.com/dapplink-labs/dapplink-wallet-api/protobuf/common"
	"github.com/ethereum/go-ethereum/log"

	"github.com/dapplink-labs/dapplink-wallet-api/chain"
	base "github.com/dapplink-labs/dapplink-wallet-api/chain/bitcoinbase"
	"github.com/dapplink-labs/dapplink-wallet-api/config"
	"github.com/dapplink-labs/dapplink-wallet-api/protobuf/walletapi"
)

const ChainID = "DappLinkBitcoin"

type ChainAdaptor struct {
	btcClient       *base.BaseClient
	btcDataClient   *base.BaseDataClient
	thirdPartClient *BcClient
}

func NewChainAdaptor(conf *config.Config) (chain.IChainAdaptor, error) {
	baseClient, err := base.NewBaseClient(conf.WalletNode.Btc.RpcUrl, conf.WalletNode.Btc.RpcUser, conf.WalletNode.Btc.RpcPass)
	if err != nil {
		log.Error("new bitcoin rpc client fail", "err", err)
		return nil, err
	}
	baseDataClient, err := base.NewBaseDataClient(conf.WalletNode.Btc.DataApiUrl, conf.WalletNode.Btc.DataApiKey, "BTC", "Bitcoin")
	if err != nil {
		log.Error("new bitcoin data client fail", "err", err)
		return nil, err
	}
	bcClient, err := NewBlockChainClient(conf.WalletNode.Btc.TpApiUrl)
	if err != nil {
		log.Error("new blockchain client fail", "err", err)
		return nil, err
	}
	return &ChainAdaptor{
		btcClient:       baseClient,
		btcDataClient:   baseDataClient,
		thirdPartClient: bcClient,
	}, nil
}

func (c ChainAdaptor) ConvertAddresses(ctx context.Context, req *walletapi.ConvertAddressesRequest) (*walletapi.ConvertAddressesResponse, error) {
	var addressList []*walletapi.Addresses
	for _, publicKeyItem := range req.GetPublicKey() {
		var walletAddress walletapi.Addresses
		pubKeyBytes, err := hex.DecodeString(strings.TrimPrefix(publicKeyItem.PublicKey, "0x"))
		if err != nil {
			log.Error("decode public key fail", "err", err, "publicKey", publicKeyItem.PublicKey)
			addressList = append(addressList, &walletapi.Addresses{
				Type:      publicKeyItem.Type,
				PublicKey: publicKeyItem.PublicKey,
			})
			continue
		}
		pubKey, err := btcec.ParsePubKey(pubKeyBytes)
		if err != nil {
			log.Error("parse public key fail", "err", err, "publicKey", publicKeyItem.PublicKey)
			addressList = append(addressList, &walletapi.Addresses{
				Type:      publicKeyItem.Type,
				PublicKey: publicKeyItem.PublicKey,
			})
			continue
		}
		pubKeyHash := btcutil.Hash160(pubKey.SerializeCompressed())

		walletAddress.Type = publicKeyItem.Type
		walletAddress.PublicKey = publicKeyItem.PublicKey
		
		switch req.GetAddressFormat() {
		case "p2pkh":
			p2pkhAddr, err := btcutil.NewAddressPubKeyHash(pubKeyHash, &chaincfg.MainNetParams)
			if err != nil {
				log.Error("create p2pkh address fail", "err", err, "pubKeyHash", hex.EncodeToString(pubKeyHash))
				walletAddress.Address = ""
			} else {
				walletAddress.Address = p2pkhAddr.EncodeAddress()
			}
			break
		case "p2wpkh":
			witnessAddr, err := btcutil.NewAddressWitnessPubKeyHash(pubKeyHash, &chaincfg.MainNetParams)
			if err != nil {
				log.Error("create p2wpkh fail", "err", err, "pubKeyHash", pubKeyHash)
				walletAddress.Address = ""
			} else {
				walletAddress.Address = witnessAddr.EncodeAddress()
			}
			break
		case "p2sh":
			witnessAddr, _ := btcutil.NewAddressWitnessPubKeyHash(pubKeyHash, &chaincfg.MainNetParams)
			script, err := txscript.PayToAddrScript(witnessAddr)
			if err != nil {
				log.Error("pay to address script fail", "err", err, "publickey", publicKeyItem.PublicKey)
				walletAddress.Address = ""
			}
			p2shAddr, err := btcutil.NewAddressScriptHash(script, &chaincfg.MainNetParams)
			if err != nil {
				log.Error("create p2sh address fail", "err", err, "publickey", publicKeyItem.PublicKey)
				walletAddress.Address = ""
			} else {
				walletAddress.Address = p2shAddr.EncodeAddress()
			}
			break
		case "p2tr":
			taprootPubKey := schnorr.SerializePubKey(pubKey)
			taprootAddr, err := btcutil.NewAddressTaproot(taprootPubKey, &chaincfg.MainNetParams)
			if err != nil {
				log.Error("create p2tr address fail", "err", err, "pubkey", pubKey)
				walletAddress.Address = ""
			} else {
				walletAddress.Address = taprootAddr.EncodeAddress()
			}
			break
		default:
			log.Error("unsupported address format", "format", req.GetAddressFormat())
			walletAddress.Address = ""
		}

		addressList = append(addressList, &walletAddress)
	}
	return &walletapi.ConvertAddressesResponse{
		Code:    common.ReturnCode_SUCCESS,
		Msg:     "create address success",
		Address: addressList,
	}, nil
}

func (c ChainAdaptor) ValidAddresses(ctx context.Context, req *walletapi.ValidAddressesRequest) (*walletapi.ValidAddressesResponse, error) {
	var addressesValidList []*walletapi.AddressesValid
	for _, addressItem := range req.GetAddresses() {
		var addrValid walletapi.AddressesValid
		address, err := btcutil.DecodeAddress(addressItem.Address, &chaincfg.MainNetParams)
		addrValid.Address = addressItem.GetAddress()
		if err != nil || !address.IsForNet(&chaincfg.MainNetParams) {
			addrValid.Valid = false
		} else {
			addrValid.Valid = true
		}
	}
	return &walletapi.ValidAddressesResponse{
		Code:         common.ReturnCode_SUCCESS,
		Msg:          "verify address success",
		AddressValid: addressesValidList,
	}, nil
}

func (c ChainAdaptor) GetLastestBlock(ctx context.Context, req *walletapi.LastestBlockRequest) (*walletapi.LastestBlockResponse, error) {
	blockInfo, err := c.btcClient.GetBlockChainInfo()
	if err != nil {
		log.Error("Get blockchain info fail", "err", err)
		return nil, err
	}
	return &walletapi.LastestBlockResponse{
		Code:   common.ReturnCode_SUCCESS,
		Msg:    "Get lastest block success",
		Height: uint64(blockInfo.Headers),
	}, nil
}

func (c ChainAdaptor) GetBlock(ctx context.Context, req *walletapi.BlockRequest) (*walletapi.BlockResponse, error) {
	var params []json.RawMessage
	numBlocksJSON, _ := json.Marshal(req.HashHeight)
	params = []json.RawMessage{numBlocksJSON}
	block, _ := c.btcClient.Client.RawRequest("getblock", params)
	var resultBlock types.BlockData
	err := json.Unmarshal(block, &resultBlock)
	if err != nil {
		log.Error("Unmarshal json fail", "err", err)
	}
	var transactionList []*walletapi.TransactionList
	for _, txid := range resultBlock.Tx {
		txIdJson, _ := json.Marshal(txid)
		boolJSON, _ := json.Marshal(true)
		dataJSON := []json.RawMessage{txIdJson, boolJSON}
		tx, err := c.btcClient.Client.RawRequest("getrawtransaction", dataJSON)
		if err != nil {
			fmt.Println("get raw transaction fail", "err", err)
		}
		var rawTx types.RawTransactionData
		err = json.Unmarshal(tx, &rawTx)
		if err != nil {
			log.Error("json unmarshal fail", "err", err)
			return nil, err
		}

		var fromList []*walletapi.FromAddress
		for _, vin := range rawTx.Vin {
			fromItem := &walletapi.FromAddress{
				Amount:  strconv.Itoa(10),
				Address: vin.ScriptSig.Asm,
			}
			fromList = append(fromList, fromItem)
		}
		var toList []*walletapi.ToAddress
		for _, vout := range rawTx.Vout {
			toItem := &walletapi.ToAddress{
				Address: vout.ScriptPubKey.Address,
				Amount:  strconv.FormatInt(int64(vout.Value), 10),
			}
			toList = append(toList, toItem)
		}
		txItem := &walletapi.TransactionList{
			TxHash: rawTx.Hash,
			Fee:    strconv.Itoa(0),
			Status: 0,
			From:   fromList,
			To:     toList,
		}
		transactionList = append(transactionList, txItem)
	}
	return &walletapi.BlockResponse{
		Code:         common.ReturnCode_SUCCESS,
		Msg:          "get block by number success",
		Height:       strconv.FormatUint(resultBlock.Height, 10),
		Hash:         req.HashHeight,
		Transactions: transactionList,
	}, nil
}

func (c ChainAdaptor) GetBatchBlock(ctx context.Context, req *walletapi.BatchBlockRequest) (*walletapi.BatchBlockResponse, error) {
	return &walletapi.BatchBlockResponse{
		Code: common.ReturnCode_ERROR,
		Msg:  "batch block query is not supported",
	}, nil
}

func (c ChainAdaptor) GetTransactionByHash(ctx context.Context, req *walletapi.TransactionByHashRequest) (*walletapi.TransactionByHashResponse, error) {
	txInfo, err := c.thirdPartClient.GetTransactionsByHash(req.Hash)
	if err != nil {
		return &walletapi.TransactionByHashResponse{
			Code:        common.ReturnCode_ERROR,
			Msg:         "get transaction list fail",
			Transaction: nil,
		}, err
	}
	var fromAddrs []*walletapi.FromAddress
	var toAddrs []*walletapi.ToAddress
	var direction int32
	for _, inputs := range txInfo.Inputs {
		fromAddrs = append(fromAddrs, &walletapi.FromAddress{Address: inputs.PrevOut.Addr, Amount: inputs.PrevOut.Value.String()})
	}
	txFee := txInfo.Fee
	for _, out := range txInfo.Out {
		toAddrs = append(toAddrs, &walletapi.ToAddress{Address: out.Addr, Amount: out.Value.String()})
	}
	tx := &walletapi.TransactionList{
		TxHash: txInfo.Hash,
		From:   fromAddrs,
		To:     toAddrs,
		Fee:    txFee.String(),
		Status: uint32(walletapi.TxStatus_Success),
		TxType: uint32(direction),
	}
	return &walletapi.TransactionByHashResponse{
		Code:        common.ReturnCode_SUCCESS,
		Msg:         "get transaction by hash success",
		Transaction: tx,
	}, nil
}

func (c ChainAdaptor) GetTransactionByAddress(ctx context.Context, req *walletapi.TransactionByAddressRequest) (*walletapi.TransactionByAddressResponse, error) {
	transaction, err := c.thirdPartClient.GetTransactionsByAddress(req.Address, strconv.Itoa(int(req.Page)), strconv.Itoa(int(req.PageSize)))
	if err != nil {
		return &walletapi.TransactionByAddressResponse{
			Code:        common.ReturnCode_ERROR,
			Msg:         "get transaction list fail",
			Transaction: nil,
		}, err
	}
	var txList []*walletapi.TransactionList
	for _, ttxs := range transaction.Txs {
		var fromAddrs []*walletapi.FromAddress
		var toAddrs []*walletapi.ToAddress
		var direction int32
		for _, inputs := range ttxs.Inputs {
			fromAddrs = append(fromAddrs, &walletapi.FromAddress{Address: inputs.PrevOut.Addr, Amount: inputs.PrevOut.Value.String()})
		}
		txFee := ttxs.Fee
		for _, out := range ttxs.Out {
			toAddrs = append(toAddrs, &walletapi.ToAddress{Address: out.Addr, Amount: out.Value.String()})
		}
		if strings.EqualFold(req.Address, fromAddrs[0].Address) {
			direction = 0
		} else {
			direction = 1
		}
		tx := &walletapi.TransactionList{
			TxHash: ttxs.Hash,
			From:   fromAddrs,
			To:     toAddrs,
			Fee:    txFee.String(),
			Status: uint32(walletapi.TxStatus_Success),
			TxType: uint32(direction),
		}
		txList = append(txList, tx)
	}
	return &walletapi.TransactionByAddressResponse{
		Code:        common.ReturnCode_SUCCESS,
		Msg:         "get transaction list success",
		Transaction: txList,
	}, nil
}

func (c ChainAdaptor) GetAccountBalance(ctx context.Context, req *walletapi.AccountBalanceRequest) (*walletapi.AccountBalanceResponse, error) {
	balance, err := c.thirdPartClient.GetAccountBalance(req.Address)
	if err != nil {
		return &walletapi.AccountBalanceResponse{
			Code:    common.ReturnCode_ERROR,
			Msg:     "get btc balance fail",
			Balance: "0",
		}, err
	}
	return &walletapi.AccountBalanceResponse{
		Code:    common.ReturnCode_SUCCESS,
		Msg:     "get btc balance success",
		Balance: balance,
	}, nil
}

func (c ChainAdaptor) SendTransaction(ctx context.Context, req *walletapi.SendTransactionsRequest) (*walletapi.SendTransactionResponse, error) {
	var txRetList []*walletapi.RawTransactionReturn
	for _, txItem := range req.RawTx {
		var txRetItem walletapi.RawTransactionReturn
		r := bytes.NewReader([]byte(txItem.RawTx))
		var msgTx wire.MsgTx
		err := msgTx.Deserialize(r)
		if err != nil {
			log.Error("msgTx.Deserialize fail")
			txRetItem = walletapi.RawTransactionReturn{}
			txRetList = append(txRetList, &txRetItem)
			continue
		}
		txHash, err := c.btcClient.SendRawTransaction(&msgTx, true)
		if err != nil {
			log.Error("btcClient.SendRawTransaction fail")
			txRetItem = walletapi.RawTransactionReturn{}
			txRetList = append(txRetList, &txRetItem)
			continue
		}
		if strings.Compare(msgTx.TxHash().String(), txHash.String()) != 0 {
			log.Error("broadcast transaction, tx hash mismatch", "local hash", msgTx.TxHash().String(), "hash from net", txHash.String(), "signedTx", req.RawTx)
			txRetItem = walletapi.RawTransactionReturn{}
			txRetList = append(txRetList, &txRetItem)
			continue
		}
		txRetItem.TxHash = txHash.String()
		txRetItem.IsSuccess = true
		txRetItem.Message = "send transaction success"
		txRetList = append(txRetList, &txRetItem)
	}

	return &walletapi.SendTransactionResponse{
		Code:   common.ReturnCode_SUCCESS,
		Msg:    "send tx success",
		TxnRet: txRetList,
	}, nil
}

func (c ChainAdaptor) BuildTransactionSchema(ctx context.Context, request *walletapi.TransactionSchemaRequest) (*walletapi.TransactionSchemaResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (c ChainAdaptor) BuildUnSignTransaction(ctx context.Context, request *walletapi.UnSignTransactionRequest) (*walletapi.UnSignTransactionResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (c ChainAdaptor) BuildSignedTransaction(ctx context.Context, request *walletapi.SignedTransactionRequest) (*walletapi.SignedTransactionResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (c ChainAdaptor) GetAddressApproveList(ctx context.Context, request *walletapi.AddressApproveListRequest) (*walletapi.AddressApproveListResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (c ChainAdaptor) BuildSponsoredTransfer(ctx context.Context, request *walletapi.SponsoredTransferRequest) (*walletapi.SponsoredTransferBuildResponse, error) {
	panic("implement me")
}

func (c ChainAdaptor) SendSponsoredTransfer(ctx context.Context, request *walletapi.SponsoredTransferSendRequest) (*walletapi.SendTransactionResponse, error) {
	panic("implement me")
}
