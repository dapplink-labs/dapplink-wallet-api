package ton

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/tvm/cell"
	"github.com/xssnick/tonutils-go/ton/wallet"
	"github.com/xssnick/tonutils-go/toncenter"
)

// tonTransferIntent is the JSON inside base64_tx for BuildUnSignTransaction / BuildSignedTransaction.
// Encode the JSON as UTF-8, then base64.StdEncoding for the API field base64_tx.
type tonTransferIntent struct {
	WalletVersion string `json:"wallet_version"` // must be "v4r2"
	PublicKey     string `json:"public_key"`     // 64 hex chars, ed25519 public key
	Destination   string `json:"destination"`    // user-friendly TON address
	AmountTon     string `json:"amount_ton"`     // decimal TON, e.g. "0.05"
	Bounce        bool   `json:"bounce"`
	Comment       string `json:"comment,omitempty"`
	Seqno         *uint32 `json:"seqno,omitempty"`        // if omitted, fetched via Toncenter GetWalletInformation
	ValidUntil    *uint32 `json:"valid_until,omitempty"` // unix seconds; default now+3m (wallet internal TTL)
}

func parseTransferIntentJSON(raw []byte) (*tonTransferIntent, error) {
	var p tonTransferIntent
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	p.WalletVersion = strings.TrimSpace(strings.ToLower(p.WalletVersion))
	if p.WalletVersion != "" && p.WalletVersion != "v4r2" {
		return nil, fmt.Errorf("wallet_version must be v4r2 or empty")
	}
	p.WalletVersion = "v4r2"
	p.PublicKey = strings.TrimPrefix(strings.TrimSpace(p.PublicKey), "0x")
	if len(p.PublicKey) != 64 {
		return nil, fmt.Errorf("public_key must be 64 hex characters (32 bytes ed25519)")
	}
	if strings.TrimSpace(p.Destination) == "" {
		return nil, fmt.Errorf("destination is required")
	}
	if strings.TrimSpace(p.AmountTon) == "" {
		return nil, fmt.Errorf("amount_ton is required")
	}
	return &p, nil
}

func decodeIntentFromBase64Tx(b64 string) (*tonTransferIntent, error) {
	raw, err := decodeBase64Payload(b64)
	if err != nil {
		return nil, err
	}
	return parseTransferIntentJSON(raw)
}

func decodeBase64Payload(b64 string) ([]byte, error) {
	s := strings.TrimSpace(b64)
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		data, err = base64.RawURLEncoding.DecodeString(s)
	}
	return data, err
}

func pubKeyFromHex(s string) (ed25519.PublicKey, error) {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid ed25519 public key")
	}
	return ed25519.PublicKey(b), nil
}

func buildV4R2PayloadCell(subwallet uint32, validUntil uint32, seq uint32, messages []*wallet.Message) (*cell.Cell, error) {
	if len(messages) == 0 || len(messages) > 4 {
		return nil, fmt.Errorf("need 1..4 internal messages")
	}
	b := cell.BeginCell().
		MustStoreUInt(uint64(subwallet), 32).
		MustStoreUInt(uint64(validUntil), 32).
		MustStoreUInt(uint64(seq), 32).
		MustStoreInt(0, 8)
	for i, message := range messages {
		intMsg, err := tlb.ToCell(message.InternalMessage)
		if err != nil {
			return nil, fmt.Errorf("internal message %d: %w", i, err)
		}
		b.MustStoreUInt(uint64(message.Mode), 8).MustStoreRef(intMsg)
	}
	return b.EndCell(), nil
}

func walletBodyWithSignature(sig []byte, payload *cell.Cell) (*cell.Cell, error) {
	if len(sig) != 64 {
		return nil, fmt.Errorf("signature must be 64 bytes (ed25519)")
	}
	return cell.BeginCell().
		MustStoreSlice(sig, 512).
		MustStoreBuilder(payload.ToBuilder()).
		EndCell(), nil
}

func externalMessageBOC(dst *address.Address, stateInit *tlb.StateInit, body *cell.Cell) (string, error) {
	ext := &tlb.ExternalMessage{
		SrcAddr:   nil,
		DstAddr:   dst,
		ImportFee: tlb.ZeroCoins,
		StateInit: stateInit,
		Body:      body,
	}
	c, err := tlb.ToCell(ext)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(c.ToBOCWithFlags(false)), nil
}

func resolveSeqno(ctx context.Context, api *toncenter.Client, walletAddr *address.Address, hint *uint32) (uint32, error) {
	if hint != nil {
		return *hint, nil
	}
	wi, err := api.V2().GetWalletInformation(ctx, walletAddr)
	if err != nil {
		return 0, err
	}
	return uint32(wi.Seqno), nil
}

func resolveValidUntil(p *tonTransferIntent) uint32 {
	if p.ValidUntil != nil {
		return *p.ValidUntil
	}
	return uint32(time.Now().Add(3 * time.Minute).UTC().Unix())
}

func buildTransferMessage(dest *address.Address, amountTon string, bounce bool, comment string) (*wallet.Message, error) {
	amt := tlb.MustFromTON(strings.TrimSpace(amountTon))
	var body *cell.Cell
	var err error
	if strings.TrimSpace(comment) != "" {
		body, err = wallet.CreateCommentCell(comment)
		if err != nil {
			return nil, err
		}
	}
	return &wallet.Message{
		Mode: wallet.PayGasSeparately + wallet.IgnoreErrors,
		InternalMessage: &tlb.InternalMessage{
			IHRDisabled: true,
			Bounce:      bounce,
			DstAddr:     dest,
			Amount:      amt,
			Body:        body,
		},
	}, nil
}

// TransactionSchemaJSON returns the JSON schema for base64_tx (tonTransferIntent).
func TransactionSchemaJSON() string {
	s := map[string]interface{}{
		"$schema":              "http://json-schema.org/draft-07/schema#",
		"title":                "TON wallet transfer (v4r2)",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"public_key", "destination", "amount_ton"},
		"properties": map[string]interface{}{
			"wallet_version": map[string]interface{}{"type": "string", "enum": []string{"v4r2"}},
			"public_key":     map[string]interface{}{"type": "string", "description": "64 hex chars, ed25519 public key"},
			"destination":    map[string]interface{}{"type": "string", "description": "User-friendly TON address (EQ/UQ/...)"},
			"amount_ton":     map[string]interface{}{"type": "string", "description": "Decimal TON amount, e.g. 0.05"},
			"bounce":         map[string]interface{}{"type": "boolean"},
			"comment":        map[string]interface{}{"type": "string"},
			"seqno":          map[string]interface{}{"type": "integer", "description": "Optional; fetched from chain if omitted"},
			"valid_until":    map[string]interface{}{"type": "integer", "description": "Optional unix seconds; default now+180s"},
		},
		"description": "UTF-8 JSON of this object is base64-encoded into UnSignTransactionRequest.base64_txn[].base64_tx",
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	return string(b)
}
