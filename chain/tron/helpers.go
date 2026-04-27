package tron

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"strings"

	base582 "github.com/btcsuite/btcutil/base58"
	"github.com/fbsobreira/gotron-sdk/pkg/address"
	"github.com/mr-tron/base58"
)

const (
	AddressPrefix = "41"
)

// Base58ToHex Convert TRON address from base58 to hexadecimal

// Base58 转 Hex
func Base58ToHex(base58Addr string) (string, error) {

	// 解码 base58 地址
	dec, _ := base58.Decode(base58Addr)

	// 检查解码后的长度是否为 25 字节
	if len(dec) != 25 {
		panic("无效的长度")
	}

	// 提取初始地址（前 21 字节）
	initialAddress := dec[:21]

	// 计算验证代码
	expectedVerificationCode := make([]byte, 4)
	hash := sha256.Sum256(initialAddress)
	hash2 := sha256.Sum256(hash[:])
	copy(expectedVerificationCode, hash2[:4])

	// 验证验证代码
	if !bytes.Equal(dec[21:], expectedVerificationCode) {
		panic("无效的验证代码")
	}

	// 将初始地址转换为 hex 字符串，并添加 "0x" 前缀
	hexAddress := "0x" + hex.EncodeToString(initialAddress)
	return hexAddress, nil
}

// PadLeftZero Fill the left side of the hexadecimal string with zero to the specified length
func PadLeftZero(hexStr string, length int) string {
	return strings.Repeat("0", length-len(hexStr)) + hexStr
}

// ParseTRC20TransferData Extract the 'to' address and 'amount' from ABI encoded data`
func ParseTRC20TransferData(data string) (string, *big.Int) {
	// Extract the receiving address (10-20 bytes, 2 characters per byte in hexadecimal, positions 20 to 40)
	toAddressHex := data[32:72]
	toAddress, _ := address.HexToAddress(AddressPrefix + toAddressHex) // TRON addresses usually start with '41'
	valueHex := data[72:136]                                           // Get amount
	value := new(big.Int)
	value.SetString(valueHex, 16) // Parse hexadecimal to integer
	return toAddress.String(), value
}

// Helper functions
func HexToTronAddress(hexAddr string) string {
	hexAddr = strings.TrimPrefix(hexAddr, "0x")
	addrBytes, err := hex.DecodeString(hexAddr)
	if err != nil {
		return ""
	}
	return base582.CheckEncode(addrBytes[1:], addrBytes[0])
}

func TronAddressToHex(addr string) string {
	decoded, version, err := base582.CheckDecode(addr)
	if err != nil {
		return ""
	}
	return "0x" + hex.EncodeToString(append([]byte{version}, decoded...))
}

func FormatTronAddress(address string) string {
	if strings.HasPrefix(address, "T") {
		return "0x" + hex.EncodeToString(base582.Decode(address))
	}
	if !strings.HasPrefix(address, "0x") {
		return "0x" + address
	}
	return address
}
