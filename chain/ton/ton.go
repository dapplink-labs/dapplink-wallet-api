package ton

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/ton/wallet"
	"github.com/xssnick/tonutils-go/toncenter"

	"github.com/dapplink-labs/dapplink-wallet-api/chain"
	"github.com/dapplink-labs/dapplink-wallet-api/config"
	wallet_api "github.com/dapplink-labs/dapplink-wallet-api/protobuf/wallet-api"
)

const ChainID = "DappLinkTon"

// Masterchain shard id (0x8000000000000000) as int64.
const masterchainShard int64 = -9223372036854775808

type ChainAdaptor struct {
	api *toncenter.Client
}

// NewChainAdaptor builds a TON adaptor using a Toncenter-compatible HTTP API (base URL, e.g. https://toncenter.com).
// Optional API key is read from conf.WalletNode.Ton.DataApiKey (X-API-Key).
func NewChainAdaptor(conf *config.Config) (chain.IChainAdaptor, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(conf.WalletNode.Ton.RpcUrl), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("ton rpc_url is empty")
	}
	timeout := time.Duration(conf.WalletNode.Ton.TimeOut) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	opts := []toncenter.Option{
		toncenter.WithTimeout(timeout),
	}
	if k := strings.TrimSpace(conf.WalletNode.Ton.DataApiKey); k != "" {
		opts = append(opts, toncenter.WithAPIKey(k))
	}
	cli := toncenter.New(baseURL, opts...)
	return &ChainAdaptor{api: cli}, nil
}

func (c ChainAdaptor) ConvertAddresses(ctx context.Context, req *wallet_api.ConvertAddressesRequest) (*wallet_api.ConvertAddressesResponse, error) {
	var out []*wallet_api.Addresses
	for _, pk := range req.GetPublicKey() {
		item := &wallet_api.Addresses{Address: ""}
		raw := strings.TrimPrefix(strings.TrimSpace(pk.GetPublicKey()), "0x")
		pubBytes, err := hex.DecodeString(raw)
		if err != nil || len(pubBytes) != 32 {
			log.Error("ton convert address: invalid ed25519 public key hex", "err", err, "len", len(pubBytes))
			out = append(out, item)
			continue
		}
		addr, err := wallet.AddressFromPubKey(ed25519.PublicKey(pubBytes), wallet.V4R2, wallet.DefaultSubwallet)
		if err != nil {
			log.Error("ton AddressFromPubKey failed", "err", err)
			out = append(out, item)
			continue
		}
		item.Address = addr.String()
		out = append(out, item)
	}
	return &wallet_api.ConvertAddressesResponse{
		Code:    wallet_api.ReturnCode_SUCCESS,
		Msg:     "success",
		Address: out,
	}, nil
}

func (c ChainAdaptor) ValidAddresses(ctx context.Context, req *wallet_api.ValidAddressesRequest) (*wallet_api.ValidAddressesResponse, error) {
	var list []*wallet_api.AddressesValid
	for _, a := range req.GetAddresses() {
		v := &wallet_api.AddressesValid{Address: a.GetAddress()}
		_, err := address.ParseAddr(strings.TrimSpace(a.GetAddress()))
		v.Valid = err == nil
		list = append(list, v)
	}
	return &wallet_api.ValidAddressesResponse{
		Code:         wallet_api.ReturnCode_SUCCESS,
		Msg:          "success",
		AddressValid: list,
	}, nil
}

func (c ChainAdaptor) GetLastestBlock(ctx context.Context, req *wallet_api.LastestBlockRequest) (*wallet_api.LastestBlockResponse, error) {
	info, err := c.api.V2().GetMasterchainInfo(ctx)
	if err != nil {
		log.Error("ton GetMasterchainInfo failed", "err", err)
		return &wallet_api.LastestBlockResponse{
			Code: wallet_api.ReturnCode_ERROR,
			Msg:  err.Error(),
		}, nil
	}
	last := info.Last
	hash := hex.EncodeToString(last.RootHash)
	return &wallet_api.LastestBlockResponse{
		Code:   wallet_api.ReturnCode_SUCCESS,
		Msg:    "success",
		Height: last.Seqno,
		Hash:   hash,
	}, nil
}

func (c ChainAdaptor) GetBlock(ctx context.Context, req *wallet_api.BlockRequest) (*wallet_api.BlockResponse, error) {
	if req.GetIsBlockHash() {
		return &wallet_api.BlockResponse{
			Code: wallet_api.ReturnCode_ERROR,
			Msg:  "TON masterchain block query by seqno is supported when is_block_hash is false (hash_height = seqno)",
		}, nil
	}
	seqno, err := strconv.ParseUint(strings.TrimSpace(req.GetHashHeight()), 10, 64)
	if err != nil {
		return &wallet_api.BlockResponse{
			Code: wallet_api.ReturnCode_ERROR,
			Msg:  "invalid masterchain seqno in hash_height",
		}, nil
	}
	count := 64
	bl, err := c.api.V2().GetBlockTransactions(ctx, int32(-1), masterchainShard, seqno, &toncenter.GetBlockTransactionsV2Options{
		Count: &count,
	})
	if err != nil {
		log.Error("ton GetBlockTransactions failed", "err", err)
		return &wallet_api.BlockResponse{
			Code: wallet_api.ReturnCode_ERROR,
			Msg:  err.Error(),
		}, nil
	}
	var txs []*wallet_api.TransactionList
	for _, t := range bl.Transactions {
		txs = append(txs, &wallet_api.TransactionList{
			TxHash: hex.EncodeToString(t.Hash),
			Fee:    "0",
			Status: 0,
			From:   []*wallet_api.FromAddress{{Address: t.Account, Amount: "0"}},
			To:     []*wallet_api.ToAddress{{Address: t.Account, Amount: "0"}},
		})
	}
	heightStr := strconv.FormatUint(bl.ID.Seqno, 10)
	return &wallet_api.BlockResponse{
		Code:         wallet_api.ReturnCode_SUCCESS,
		Msg:          "success",
		Height:       heightStr,
		Hash:         hex.EncodeToString(bl.ID.RootHash),
		Transactions: txs,
	}, nil
}

func (c ChainAdaptor) GetTransactionByHash(ctx context.Context, req *wallet_api.TransactionByHashRequest) (*wallet_api.TransactionByHashResponse, error) {
	return &wallet_api.TransactionByHashResponse{
		Code: wallet_api.ReturnCode_ERROR,
		Msg:  "TON requires account context to locate a transaction; use GetTransactionByAddress or explorer APIs",
	}, nil
}

func (c ChainAdaptor) GetTransactionByAddress(ctx context.Context, req *wallet_api.TransactionByAddressRequest) (*wallet_api.TransactionByAddressResponse, error) {
	addr, err := address.ParseAddr(strings.TrimSpace(req.GetAddress()))
	if err != nil {
		return &wallet_api.TransactionByAddressResponse{
			Code: wallet_api.ReturnCode_ERROR,
			Msg:  "invalid TON address",
		}, nil
	}
	page := req.GetPage()
	if page == 0 {
		page = 1
	}
	ps := req.GetPageSize()
	if ps == 0 {
		ps = 10
	}
	limit := int(page * ps)
	if limit > 100 {
		limit = 100
	}
	txs, err := c.api.V2().GetTransactions(ctx, addr, &toncenter.GetTransactionsV2Opts{
		Limit: &limit,
	})
	if err != nil {
		log.Error("ton GetTransactions failed", "err", err)
		return &wallet_api.TransactionByAddressResponse{
			Code: wallet_api.ReturnCode_ERROR,
			Msg:  err.Error(),
		}, nil
	}
	start := int((page - 1) * ps)
	if start >= len(txs) {
		return &wallet_api.TransactionByAddressResponse{
			Code:        wallet_api.ReturnCode_SUCCESS,
			Msg:         "success",
			Transaction: nil,
		}, nil
	}
	end := start + int(ps)
	if end > len(txs) {
		end = len(txs)
	}
	slice := txs[start:end]
	var out []*wallet_api.TransactionList
	for _, tx := range slice {
		fee := tx.Fee.MustCoins(toncenter.TonDecimals).String()
		st := uint32(0)
		if !tx.Aborted {
			st = 1
		}
		tl := &wallet_api.TransactionList{
			TxHash: hex.EncodeToString(tx.TransactionID.Hash),
			Fee:    fee,
			Status: st,
		}
		if tx.InMsg != nil {
			if tx.InMsg.Source != nil && tx.InMsg.Source.Addr != nil {
				tl.From = append(tl.From, &wallet_api.FromAddress{
					Address: tx.InMsg.Source.Addr.String(),
					Amount:  tx.InMsg.Value.MustCoins(toncenter.TonDecimals).String(),
				})
			}
			if tx.InMsg.Destination != nil && tx.InMsg.Destination.Addr != nil {
				tl.To = append(tl.To, &wallet_api.ToAddress{
					Address: tx.InMsg.Destination.Addr.String(),
					Amount:  tx.InMsg.Value.MustCoins(toncenter.TonDecimals).String(),
				})
			}
		}
		out = append(out, tl)
	}
	return &wallet_api.TransactionByAddressResponse{
		Code:        wallet_api.ReturnCode_SUCCESS,
		Msg:         "success",
		Transaction: out,
	}, nil
}

func (c ChainAdaptor) GetAccountBalance(ctx context.Context, req *wallet_api.AccountBalanceRequest) (*wallet_api.AccountBalanceResponse, error) {
	addr, err := address.ParseAddr(strings.TrimSpace(req.GetAddress()))
	if err != nil {
		return &wallet_api.AccountBalanceResponse{
			Code: wallet_api.ReturnCode_ERROR,
			Msg:  "invalid TON address",
		}, nil
	}
	bal, err := c.api.V2().GetAddressBalance(ctx, addr)
	if err != nil {
		log.Error("ton GetAddressBalance failed", "err", err)
		return &wallet_api.AccountBalanceResponse{
			Code: wallet_api.ReturnCode_ERROR,
			Msg:  err.Error(),
		}, nil
	}
	s := bal.MustCoins(toncenter.TonDecimals).String()
	return &wallet_api.AccountBalanceResponse{
		Code:    wallet_api.ReturnCode_SUCCESS,
		Msg:     "success",
		Network: req.GetNetwork(),
		Balance: s,
	}, nil
}

func (c ChainAdaptor) SendTransaction(ctx context.Context, req *wallet_api.SendTransactionsRequest) (*wallet_api.SendTransactionResponse, error) {
	var rets []*wallet_api.RawTransactionReturn
	for _, raw := range req.GetRawTx() {
		rr := &wallet_api.RawTransactionReturn{IsSuccess: false}
		boc, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw.GetRawTx()))
		if err != nil {
			boc, err = base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw.GetRawTx()))
		}
		if err != nil {
			rr.Message = "invalid base64 BOC: " + err.Error()
			rets = append(rets, rr)
			continue
		}
		if err := c.api.V2().SendBoc(ctx, boc); err != nil {
			rr.Message = err.Error()
			rets = append(rets, rr)
			continue
		}
		rr.IsSuccess = true
		rr.Message = "sent"
		rets = append(rets, rr)
	}
	return &wallet_api.SendTransactionResponse{
		Code:   wallet_api.ReturnCode_SUCCESS,
		Msg:    "done",
		TxnRet: rets,
	}, nil
}

func (c ChainAdaptor) BuildTransactionSchema(ctx context.Context, request *wallet_api.TransactionSchemaRequest) (*wallet_api.TransactionSchemaResponse, error) {
	return &wallet_api.TransactionSchemaResponse{
		Code:   wallet_api.ReturnCode_SUCCESS,
		Msg:    "success",
		Schema: TransactionSchemaJSON(),
	}, nil
}

func (c ChainAdaptor) BuildUnSignTransaction(ctx context.Context, request *wallet_api.UnSignTransactionRequest) (*wallet_api.UnSignTransactionResponse, error) {
	var out []*wallet_api.UnsignedTransactionMessageHash
	for _, item := range request.GetBase64Txn() {
		empty := &wallet_api.UnsignedTransactionMessageHash{UnsignedTx: ""}
		p, err := decodeIntentFromBase64Tx(item.GetBase64Tx())
		if err != nil {
			log.Error("ton BuildUnSignTransaction: decode intent", "err", err)
			out = append(out, empty)
			continue
		}
		pub, err := pubKeyFromHex(p.PublicKey)
		if err != nil {
			log.Error("ton BuildUnSignTransaction: public key", "err", err)
			out = append(out, empty)
			continue
		}
		fromAddr, err := wallet.AddressFromPubKey(pub, wallet.V4R2, wallet.DefaultSubwallet)
		if err != nil {
			log.Error("ton BuildUnSignTransaction: wallet address", "err", err)
			out = append(out, empty)
			continue
		}
		to, err := address.ParseAddr(strings.TrimSpace(p.Destination))
		if err != nil {
			log.Error("ton BuildUnSignTransaction: destination", "err", err)
			out = append(out, empty)
			continue
		}
		seq, err := resolveSeqno(ctx, c.api, fromAddr, p.Seqno)
		if err != nil {
			log.Error("ton BuildUnSignTransaction: seqno", "err", err)
			out = append(out, empty)
			continue
		}
		tmsg, err := buildTransferMessage(to, p.AmountTon, p.Bounce, p.Comment)
		if err != nil {
			log.Error("ton BuildUnSignTransaction: transfer", "err", err)
			out = append(out, empty)
			continue
		}
		vu := resolveValidUntil(p)
		payload, err := buildV4R2PayloadCell(wallet.DefaultSubwallet, vu, seq, []*wallet.Message{tmsg})
		if err != nil {
			log.Error("ton BuildUnSignTransaction: payload", "err", err)
			out = append(out, empty)
			continue
		}
		out = append(out, &wallet_api.UnsignedTransactionMessageHash{
			UnsignedTx: hex.EncodeToString(payload.Hash()),
		})
	}
	return &wallet_api.UnSignTransactionResponse{
		Code:        wallet_api.ReturnCode_SUCCESS,
		Msg:         "success",
		UnsignedTxn: out,
	}, nil
}

func (c ChainAdaptor) BuildSignedTransaction(ctx context.Context, request *wallet_api.SignedTransactionRequest) (*wallet_api.SignedTransactionResponse, error) {
	var list []*wallet_api.SignedTxWithHash
	for _, tx := range request.GetTxnWithSignature() {
		failed := &wallet_api.SignedTxWithHash{IsSuccess: false}
		sigHex := strings.TrimSpace(tx.GetSignature())
		sig, err := hex.DecodeString(sigHex)
		if err != nil || len(sig) != 64 {
			log.Error("ton BuildSignedTransaction: bad signature hex", "err", err)
			list = append(list, failed)
			continue
		}
		p, err := decodeIntentFromBase64Tx(tx.GetBase64Tx())
		if err != nil {
			log.Error("ton BuildSignedTransaction: decode intent", "err", err)
			list = append(list, failed)
			continue
		}
		pubHex := strings.TrimPrefix(strings.TrimSpace(tx.GetPublicKey()), "0x")
		if pubHex == "" {
			pubHex = p.PublicKey
		}
		pub, err := pubKeyFromHex(pubHex)
		if err != nil {
			log.Error("ton BuildSignedTransaction: public key", "err", err)
			list = append(list, failed)
			continue
		}
		fromAddr, err := wallet.AddressFromPubKey(pub, wallet.V4R2, wallet.DefaultSubwallet)
		if err != nil {
			log.Error("ton BuildSignedTransaction: from address", "err", err)
			list = append(list, failed)
			continue
		}
		to, err := address.ParseAddr(strings.TrimSpace(p.Destination))
		if err != nil {
			log.Error("ton BuildSignedTransaction: destination", "err", err)
			list = append(list, failed)
			continue
		}
		seq, err := resolveSeqno(ctx, c.api, fromAddr, p.Seqno)
		if err != nil {
			log.Error("ton BuildSignedTransaction: seqno", "err", err)
			list = append(list, failed)
			continue
		}
		tmsg, err := buildTransferMessage(to, p.AmountTon, p.Bounce, p.Comment)
		if err != nil {
			log.Error("ton BuildSignedTransaction: transfer", "err", err)
			list = append(list, failed)
			continue
		}
		vu := resolveValidUntil(p)
		payload, err := buildV4R2PayloadCell(wallet.DefaultSubwallet, vu, seq, []*wallet.Message{tmsg})
		if err != nil {
			log.Error("ton BuildSignedTransaction: payload", "err", err)
			list = append(list, failed)
			continue
		}
		if !ed25519.Verify(pub, payload.Hash(), sig) {
			log.Error("ton BuildSignedTransaction: ed25519 verify failed")
			list = append(list, failed)
			continue
		}
		body, err := walletBodyWithSignature(sig, payload)
		if err != nil {
			log.Error("ton BuildSignedTransaction: body", "err", err)
			list = append(list, failed)
			continue
		}
		var stateInit *tlb.StateInit
		wi, err := c.api.V2().GetWalletInformation(ctx, fromAddr)
		if err == nil && strings.EqualFold(wi.AccountState, "uninitialized") {
			si, err := wallet.GetStateInit(pub, wallet.V4R2, wallet.DefaultSubwallet)
			if err != nil {
				log.Error("ton BuildSignedTransaction: state init", "err", err)
				list = append(list, failed)
				continue
			}
			stateInit = si
		}
		boc, err := externalMessageBOC(fromAddr, stateInit, body)
		if err != nil {
			log.Error("ton BuildSignedTransaction: external boc", "err", err)
			list = append(list, failed)
			continue
		}
		list = append(list, &wallet_api.SignedTxWithHash{
			SignedTx:  boc,
			TxHash:    hex.EncodeToString(body.Hash()),
			IsSuccess: true,
		})
	}
	return &wallet_api.SignedTransactionResponse{
		Code:      wallet_api.ReturnCode_SUCCESS,
		Msg:       "success",
		SignedTxn: list,
	}, nil
}

func (c ChainAdaptor) GetAddressApproveList(ctx context.Context, request *wallet_api.AddressApproveListRequest) (*wallet_api.AddressApproveListResponse, error) {
	if _, err := address.ParseAddr(strings.TrimSpace(request.GetAddress())); err != nil {
		return &wallet_api.AddressApproveListResponse{
			Code: wallet_api.ReturnCode_ERROR,
			Msg:  "invalid TON address",
		}, nil
	}
	// TON has no ERC-20 style allowance registry; Jetton uses per-jetton wallet contracts.
	return &wallet_api.AddressApproveListResponse{
		Code:      wallet_api.ReturnCode_SUCCESS,
		Msg:       "TON has no ERC-20 style approve/allowance list. Jetton token balances use separate jetton wallet contracts; none are listed here.",
		Contracts: nil,
	}, nil
}
