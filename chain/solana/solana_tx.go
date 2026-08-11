package solana

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	associatedtokenaccount "github.com/gagliardetto/solana-go/programs/associated-token-account"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/programs/token"
	"github.com/mr-tron/base58"
)

const wrappedSOLMint = "So11111111111111111111111111111111111111112"

// SolanaTransferTx is the JSON payload encoded in Base64Tx for buildUnSignTransaction.
type SolanaTransferTx struct {
	FromAddress     string `json:"from_address"`
	ToAddress       string `json:"to_address"`
	ContractAddress string `json:"contract_address"`
	Value           string `json:"value"` // integer string in smallest units
	FeePayer        string `json:"fee_payer,omitempty"`
	CreateAta       bool   `json:"create_ata,omitempty"`
	RecentBlockhash string `json:"recent_blockhash,omitempty"`
	Decimals        uint8  `json:"decimals,omitempty"`
}

// UnsignedEnvelope is returned (base64 JSON) as UnsignedTx and passed back to BuildSignedTransaction.
type UnsignedEnvelope struct {
	MessageHex            string            `json:"message"`
	NumRequiredSignatures int               `json:"num_required_signatures"`
	SignerKeys            []string          `json:"signer_keys"` // base58 pubkeys in signature order
	Signatures            map[string]string `json:"signatures"`  // base58 signer -> hex signature
	RecentBlockhash       string            `json:"recent_blockhash"`
}

func isNativeSOL(contract string) bool {
	c := strings.TrimSpace(contract)
	return c == "" || c == wrappedSOLMint
}

func parseUintAmount(raw string) (uint64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty amount")
	}
	v := new(big.Int)
	if _, ok := v.SetString(raw, 10); !ok {
		return 0, fmt.Errorf("invalid amount %q", raw)
	}
	if v.Sign() < 0 || !v.IsUint64() {
		return 0, fmt.Errorf("amount out of range: %s", raw)
	}
	return v.Uint64(), nil
}

func buildSolanaTransferTransaction(payload SolanaTransferTx, recentBlockhash string) (*solana.Transaction, error) {
	fromPubkey, err := solana.PublicKeyFromBase58(strings.TrimSpace(payload.FromAddress))
	if err != nil {
		return nil, fmt.Errorf("invalid from address: %w", err)
	}
	toPubkey, err := solana.PublicKeyFromBase58(strings.TrimSpace(payload.ToAddress))
	if err != nil {
		return nil, fmt.Errorf("invalid to address: %w", err)
	}

	feePayerStr := strings.TrimSpace(payload.FeePayer)
	if feePayerStr == "" {
		feePayerStr = payload.FromAddress
	}
	feePayer, err := solana.PublicKeyFromBase58(feePayerStr)
	if err != nil {
		return nil, fmt.Errorf("invalid fee payer: %w", err)
	}

	amount, err := parseUintAmount(payload.Value)
	if err != nil {
		return nil, err
	}

	blockhash := strings.TrimSpace(payload.RecentBlockhash)
	if blockhash == "" {
		blockhash = strings.TrimSpace(recentBlockhash)
	}
	if blockhash == "" {
		return nil, fmt.Errorf("recent blockhash is required")
	}
	recentHash := solana.MustHashFromBase58(blockhash)

	var instructions []solana.Instruction
	if isNativeSOL(payload.ContractAddress) {
		instructions = append(instructions, system.NewTransferInstruction(amount, fromPubkey, toPubkey).Build())
	} else {
		mintPubkey, err := solana.PublicKeyFromBase58(strings.TrimSpace(payload.ContractAddress))
		if err != nil {
			return nil, fmt.Errorf("invalid mint: %w", err)
		}
		fromATA, _, err := solana.FindAssociatedTokenAddress(fromPubkey, mintPubkey)
		if err != nil {
			return nil, fmt.Errorf("from ATA: %w", err)
		}
		toATA, _, err := solana.FindAssociatedTokenAddress(toPubkey, mintPubkey)
		if err != nil {
			return nil, fmt.Errorf("to ATA: %w", err)
		}
		if payload.CreateAta {
			instructions = append(instructions, associatedtokenaccount.NewCreateInstruction(
				feePayer,
				toPubkey,
				mintPubkey,
			).Build())
		}
		if payload.Decimals > 0 {
			instructions = append(instructions, token.NewTransferCheckedInstruction(
				amount,
				payload.Decimals,
				fromATA,
				mintPubkey,
				toATA,
				fromPubkey,
				[]solana.PublicKey{},
			).Build())
		} else {
			instructions = append(instructions, token.NewTransferInstruction(
				amount,
				fromATA,
				toATA,
				fromPubkey,
				[]solana.PublicKey{},
			).Build())
		}
	}

	tx, err := solana.NewTransaction(
		instructions,
		recentHash,
		solana.TransactionPayer(feePayer),
	)
	if err != nil {
		return nil, fmt.Errorf("new transaction: %w", err)
	}
	return tx, nil
}

func envelopeFromTransaction(tx *solana.Transaction) (*UnsignedEnvelope, error) {
	msgBytes, err := tx.Message.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal message: %w", err)
	}
	numSigs := int(tx.Message.Header.NumRequiredSignatures)
	signerKeys := make([]string, 0, numSigs)
	for i := 0; i < numSigs && i < len(tx.Message.AccountKeys); i++ {
		signerKeys = append(signerKeys, tx.Message.AccountKeys[i].String())
	}
	sigs := make(map[string]string, numSigs)
	for _, k := range signerKeys {
		sigs[k] = ""
	}
	return &UnsignedEnvelope{
		MessageHex:            hex.EncodeToString(msgBytes),
		NumRequiredSignatures: numSigs,
		SignerKeys:            signerKeys,
		Signatures:            sigs,
		RecentBlockhash:       tx.Message.RecentBlockhash.String(),
	}, nil
}

func encodeEnvelope(env *UnsignedEnvelope) (string, error) {
	raw, err := json.Marshal(env)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

func decodeEnvelope(base64Tx string) (*UnsignedEnvelope, error) {
	raw, err := base64.StdEncoding.DecodeString(base64Tx)
	if err != nil {
		return nil, fmt.Errorf("decode envelope base64: %w", err)
	}
	var env UnsignedEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}
	if env.Signatures == nil {
		env.Signatures = make(map[string]string)
	}
	return &env, nil
}

func publicKeyHexToBase58(pubKeyHex string) (string, error) {
	pubKeyHex = strings.TrimPrefix(strings.TrimSpace(pubKeyHex), "0x")
	pubBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return "", err
	}
	if len(pubBytes) != 32 {
		return "", fmt.Errorf("solana pubkey must be 32 bytes, got %d", len(pubBytes))
	}
	return solana.PublicKeyFromBytes(pubBytes).String(), nil
}

func assembleSignedTransaction(env *UnsignedEnvelope) (*solana.Transaction, string, error) {
	msgBytes, err := hex.DecodeString(strings.TrimPrefix(env.MessageHex, "0x"))
	if err != nil {
		return nil, "", fmt.Errorf("decode message hex: %w", err)
	}
	var message solana.Message
	if err := message.UnmarshalWithDecoder(bin.NewBinDecoder(msgBytes)); err != nil {
		return nil, "", fmt.Errorf("unmarshal message: %w", err)
	}

	signatures := make([]solana.Signature, env.NumRequiredSignatures)
	for i, signer := range env.SignerKeys {
		sigHex := strings.TrimSpace(env.Signatures[signer])
		if sigHex == "" {
			return nil, "", fmt.Errorf("missing signature for signer %s", signer)
		}
		sigBytes, err := hex.DecodeString(strings.TrimPrefix(sigHex, "0x"))
		if err != nil {
			return nil, "", fmt.Errorf("decode signature for %s: %w", signer, err)
		}
		if len(sigBytes) != 64 {
			return nil, "", fmt.Errorf("signature for %s must be 64 bytes", signer)
		}
		copy(signatures[i][:], sigBytes)
	}

	tx := &solana.Transaction{
		Signatures: signatures,
		Message:    message,
	}
	txHash := base58.Encode(signatures[0][:])
	return tx, txHash, nil
}

func serializeSignedTxBase64(tx *solana.Transaction) (string, error) {
	txBytes, err := tx.MarshalBinary()
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(txBytes), nil
}

func amountString(v uint64) string {
	return strconv.FormatUint(v, 10)
}
