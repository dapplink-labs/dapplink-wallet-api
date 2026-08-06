package tron

import (
	"encoding/hex"
	"strings"
)

const tronSignatureHexLen = 130 // 65-byte r||s||v

func formatTronSignatureHex(signature string) string {
	signature = strings.TrimPrefix(strings.TrimSpace(signature), "0x")
	if len(signature) == tronSignatureHexLen-2 {
		// Legacy 64-byte signatures are invalid for JSON broadcast; keep as-is so callers can detect.
		return signature
	}
	if len(signature) > tronSignatureHexLen {
		return signature[:tronSignatureHexLen]
	}
	return signature
}

func formatTronSignatures(signatures []string) []string {
	if len(signatures) == 0 {
		return signatures
	}
	out := make([]string, len(signatures))
	for i, sig := range signatures {
		out[i] = formatTronSignatureHex(sig)
	}
	return out
}

func broadcastTransactionPayload(signedTx *Transaction) map[string]interface{} {
	if signedTx == nil {
		return map[string]interface{}{}
	}
	payload := map[string]interface{}{
		"visible": true,
	}
	if signedTx.TxID != "" {
		payload["txID"] = signedTx.TxID
	}
	if len(signedTx.RawData.Contract) > 0 {
		payload["raw_data"] = signedTx.RawData
	}
	if signedTx.RawDataHex != "" {
		payload["raw_data_hex"] = signedTx.RawDataHex
	}
	if len(signedTx.Signature) > 0 {
		payload["signature"] = formatTronSignatures(signedTx.Signature)
	}
	return payload
}

func verifyTronSignatureLength(signatureHex string) bool {
	sig := formatTronSignatureHex(signatureHex)
	decoded, err := hex.DecodeString(sig)
	return err == nil && len(decoded) == 65
}
