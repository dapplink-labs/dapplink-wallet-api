package substratebase

import (
	"bytes"
	"encoding/binary"
	"io"
	"math/big"

	"github.com/centrifuge/go-substrate-rpc-client/v4/scale"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

func EncodeCompact(v uint64) ([]byte, error) {
	var buf bytes.Buffer
	encoder := scale.NewEncoder(&buf)
	err := encoder.EncodeUintCompact(*big.NewInt(int64(v)))
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func EncodeBool(v bool) []byte {
	var buf bytes.Buffer
	encoder := scale.NewEncoder(&buf)
	encoder.EncodeOption(v, nil)
	return buf.Bytes()
}

func EncodeUint8(v uint8) []byte {
	var buf bytes.Buffer
	encoder := scale.NewEncoder(&buf)
	encoder.PushByte(v)
	return buf.Bytes()
}

func EncodeUint16(v uint16) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, v)
	return buf.Bytes()
}

func EncodeUint32(v uint32) []byte {
	var buf bytes.Buffer
	encoder := scale.NewEncoder(&buf)
	encoder.Encode(v)
	return buf.Bytes()
}

func EncodeUint64(v uint64) []byte {
	var buf bytes.Buffer
	encoder := scale.NewEncoder(&buf)
	encoder.Encode(v)
	return buf.Bytes()
}

func EncodeBytes(v []byte) ([]byte, error) {
	var buf bytes.Buffer
	encoder := scale.NewEncoder(&buf)
	err := encoder.Write(v)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func EncodeString(v string) ([]byte, error) {
	var buf bytes.Buffer
	encoder := scale.NewEncoder(&buf)
	err := encoder.Encode(v)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func EncodeU128(v *big.Int) ([]byte, error) {
	var buf bytes.Buffer
	encoder := scale.NewEncoder(&buf)
	u128 := types.NewU128(*v)
	err := encoder.Encode(u128)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func EncodeEra(era types.ExtrinsicEra) ([]byte, error) {
	var buf bytes.Buffer
	encoder := scale.NewEncoder(&buf)
	err := encoder.Encode(era)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func EncodeMultiAddress(addr types.MultiAddress) ([]byte, error) {
	var buf bytes.Buffer
	encoder := scale.NewEncoder(&buf)
	err := encoder.Encode(addr)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func DecodeCompact(r io.Reader) (uint64, error) {
	decoder := scale.NewDecoder(r)
	var v types.UCompact
	err := decoder.Decode(&v)
	if err != nil {
		return 0, err
	}
	return uint64(v.Int64()), nil
}
