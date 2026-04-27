package substratebase

import (
	"crypto/sha512"
	"encoding/hex"
	"errors"

	"github.com/btcsuite/btcd/btcutil/base58"
	"golang.org/x/crypto/blake2b"
)

var ss58Prefix = []byte("SS58PRE")

func SS58Encode(pubKey []byte, prefix uint8) string {
	var encoded []byte
	encoded = append(encoded, prefix)
	if len(pubKey) == 33 {
		encoded = append(encoded, 0x02)
	}
	encoded = append(encoded, pubKey...)

	checksum := ss58Checksum(encoded)
	encoded = append(encoded, checksum[:2]...)

	return base58.Encode(encoded)
}

func SS58Decode(address string) ([]byte, uint8, error) {
	decoded := base58.Decode(address)
	if len(decoded) < 3 {
		return nil, 0, errors.New("invalid ss58 address: too short")
	}

	prefix := decoded[0]
	body := decoded[:len(decoded)-2]
	checksum := decoded[len(decoded)-2:]

	expectedChecksum := ss58Checksum(body)
	if checksum[0] != expectedChecksum[0] || checksum[1] != expectedChecksum[1] {
		return nil, 0, errors.New("invalid ss58 address: checksum mismatch")
	}

	var pubKey []byte
	switch len(body) {
	case 33:
		pubKey = body[1:]
	case 34:
		format := body[1]
		switch format {
		case 0x00, 0x01:
			pubKey = body[2:]
		default:
			return nil, 0, errors.New("invalid ss58 address: unknown format byte")
		}
	case 35:
		format := body[1]
		if format != 0x02 {
			return nil, 0, errors.New("invalid ss58 address: expected secp256k1 format byte 0x02")
		}
		pubKey = body[2:]
	default:
		return nil, 0, errors.New("invalid ss58 address: invalid body length")
	}

	return pubKey, prefix, nil
}

func SS58Validate(address string, expectedPrefix uint8) bool {
	_, prefix, err := SS58Decode(address)
	if err != nil {
		return false
	}
	return prefix == expectedPrefix
}

func SS58DecodeToHex(address string) (string, error) {
	pubKey, _, err := SS58Decode(address)
	if err != nil {
		return "", err
	}
	return "0x" + hex.EncodeToString(pubKey), nil
}

func ss58Checksum(data []byte) []byte {
	h := blake2b.Sum512(append(ss58Prefix, data...))
	return h[:]
}

func Blake2b256(data []byte) [32]byte {
	return blake2b.Sum256(data)
}

func Blake2b512(data []byte) [64]byte {
	return blake2b.Sum512(data)
}

func Sha512(data []byte) [64]byte {
	return sha512.Sum512(data)
}
