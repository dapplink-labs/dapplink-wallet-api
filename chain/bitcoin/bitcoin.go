package bitcoin

import (
	"context"
	"encoding/hex"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/ethereum/go-ethereum/log"

	"github.com/dapplink-labs/dapplink-wallet-api/chain"
	base "github.com/dapplink-labs/dapplink-wallet-api/chain/bitcoinbase"
	"github.com/dapplink-labs/dapplink-wallet-api/config"
	wallet_api "github.com/dapplink-labs/dapplink-wallet-api/protobuf/wallet-api"
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

func (c ChainAdaptor) ConvertAddresses(ctx context.Context, req *wallet_api.ConvertAddressesRequest) (*wallet_api.ConvertAddressesResponse, error) {
	var addressList []*wallet_api.Addresses
	for _, publicKeyItem := range req.GetPublicKey() {
		var walletAddress wallet_api.Addresses
		compressedPubKeyBytes, _ := hex.DecodeString(publicKeyItem.PublicKey)
		pubKeyHash := btcutil.Hash160(compressedPubKeyBytes)
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
			pubKey, err := btcec.ParsePubKey(compressedPubKeyBytes)
			if err != nil {
				log.Error("parse p2tr public fail", "err", err)
				walletAddress.Address = ""
			}
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
	return &wallet_api.ConvertAddressesResponse{
		Code:    wallet_api.ReturnCode_SUCCESS,
		Msg:     "create address success",
		Address: addressList,
	}, nil
}

func (c ChainAdaptor) ValidAddresses(ctx context.Context, req *wallet_api.ValidAddressesRequest) (*wallet_api.ValidAddressesResponse, error) {
	var addressesValidList []*wallet_api.AddressesValid
	for _, addressItem := range req.GetAddresses() {
		var addrValid wallet_api.AddressesValid
		address, err := btcutil.DecodeAddress(addressItem.Address, &chaincfg.MainNetParams)
		addrValid.Address = addressItem.GetAddress()
		if err != nil || !address.IsForNet(&chaincfg.MainNetParams) {
			addrValid.Valid = false
		} else {
			addrValid.Valid = true
		}
		addressesValidList = append(addressesValidList, &addrValid)
	}
	return &wallet_api.ValidAddressesResponse{
		Code:         wallet_api.ReturnCode_SUCCESS,
		Msg:          "verify address success",
		AddressValid: addressesValidList,
	}, nil
}

func (c ChainAdaptor) GetLastestBlock(ctx context.Context, req *wallet_api.LastestBlockRequest) (*wallet_api.LastestBlockResponse, error) {
	return &wallet_api.LastestBlockResponse{
		Code: wallet_api.ReturnCode_ERROR,
		Msg:  "not implemented for Bitcoin",
	}, nil
}

func (c ChainAdaptor) GetBlock(ctx context.Context, req *wallet_api.BlockRequest) (*wallet_api.BlockResponse, error) {
	return &wallet_api.BlockResponse{
		Code: wallet_api.ReturnCode_ERROR,
		Msg:  "not implemented for Bitcoin",
	}, nil
}

func (c ChainAdaptor) GetTransactionByHash(ctx context.Context, req *wallet_api.TransactionByHashRequest) (*wallet_api.TransactionByHashResponse, error) {
	return &wallet_api.TransactionByHashResponse{
		Code: wallet_api.ReturnCode_ERROR,
		Msg:  "not implemented for Bitcoin",
	}, nil
}

func (c ChainAdaptor) GetTransactionByAddress(ctx context.Context, req *wallet_api.TransactionByAddressRequest) (*wallet_api.TransactionByAddressResponse, error) {
	return &wallet_api.TransactionByAddressResponse{
		Code: wallet_api.ReturnCode_ERROR,
		Msg:  "not implemented for Bitcoin",
	}, nil
}

func (c ChainAdaptor) GetAccountBalance(ctx context.Context, req *wallet_api.AccountBalanceRequest) (*wallet_api.AccountBalanceResponse, error) {
	return &wallet_api.AccountBalanceResponse{
		Code: wallet_api.ReturnCode_ERROR,
		Msg:  "not implemented for Bitcoin",
	}, nil
}

func (c ChainAdaptor) SendTransaction(ctx context.Context, req *wallet_api.SendTransactionsRequest) (*wallet_api.SendTransactionResponse, error) {
	return &wallet_api.SendTransactionResponse{
		Code: wallet_api.ReturnCode_ERROR,
		Msg:  "not implemented for Bitcoin",
	}, nil
}

func (c ChainAdaptor) BuildTransactionSchema(ctx context.Context, request *wallet_api.TransactionSchemaRequest) (*wallet_api.TransactionSchemaResponse, error) {
	return &wallet_api.TransactionSchemaResponse{
		Code: wallet_api.ReturnCode_ERROR,
		Msg:  "not implemented for Bitcoin",
	}, nil
}

func (c ChainAdaptor) BuildUnSignTransaction(ctx context.Context, request *wallet_api.UnSignTransactionRequest) (*wallet_api.UnSignTransactionResponse, error) {
	return &wallet_api.UnSignTransactionResponse{
		Code: wallet_api.ReturnCode_ERROR,
		Msg:  "not implemented for Bitcoin",
	}, nil
}

func (c ChainAdaptor) BuildSignedTransaction(ctx context.Context, request *wallet_api.SignedTransactionRequest) (*wallet_api.SignedTransactionResponse, error) {
	return &wallet_api.SignedTransactionResponse{
		Code: wallet_api.ReturnCode_ERROR,
		Msg:  "not implemented for Bitcoin",
	}, nil
}

func (c ChainAdaptor) GetAddressApproveList(ctx context.Context, request *wallet_api.AddressApproveListRequest) (*wallet_api.AddressApproveListResponse, error) {
	return &wallet_api.AddressApproveListResponse{
		Code:      wallet_api.ReturnCode_SUCCESS,
		Msg:       "not applicable for Bitcoin",
		Contracts: nil,
	}, nil
}
