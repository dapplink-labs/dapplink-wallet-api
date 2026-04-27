package substratebase

import (
	"bytes"
	"fmt"

	"github.com/centrifuge/go-substrate-rpc-client/v4/scale"
	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/ethereum/go-ethereum/log"

	"golang.org/x/crypto/blake2b"
)

type ExtrinsicBuilder struct {
	Meta        *types.Metadata
	GenesisHash types.Hash
	Runtime     *types.RuntimeVersion
}

func NewExtrinsicBuilder(meta *types.Metadata, genesisHash types.Hash, runtime *types.RuntimeVersion) *ExtrinsicBuilder {
	return &ExtrinsicBuilder{
		Meta:        meta,
		GenesisHash: genesisHash,
		Runtime:     runtime,
	}
}

func (b *ExtrinsicBuilder) BuildTransferCall(meta *types.Metadata, to types.MultiAddress, amount types.UCompact) (types.Call, error) {
	call, err := types.NewCall(meta, "Balances.transfer_keep_alive", to, amount)
	if err != nil {
		log.Error("BuildTransferCall failed", "err", err)
		return types.Call{}, fmt.Errorf("create transfer call failed: %w", err)
	}
	return call, nil
}

func (b *ExtrinsicBuilder) BuildTransferAllowDeathCall(meta *types.Metadata, to types.MultiAddress, amount types.UCompact) (types.Call, error) {
	call, err := types.NewCall(meta, "Balances.transfer_allow_death", to, amount)
	if err != nil {
		log.Error("BuildTransferAllowDeathCall failed", "err", err)
		return types.Call{}, fmt.Errorf("create transfer_allow_death call failed: %w", err)
	}
	return call, nil
}

func (b *ExtrinsicBuilder) BuildExtrinsic(call types.Call) types.Extrinsic {
	ext := types.NewExtrinsic(call)
	return ext
}

func (b *ExtrinsicBuilder) SignExtrinsic(ext *types.Extrinsic, keyringPair signature.KeyringPair, era types.ExtrinsicEra, nonce uint64, tip uint64, blockHash types.Hash) error {
	o := types.SignatureOptions{
		Era:                era,
		Nonce:              types.NewUCompactFromUInt(nonce),
		Tip:                types.NewUCompactFromUInt(tip),
		SpecVersion:        b.Runtime.SpecVersion,
		GenesisHash:        b.GenesisHash,
		BlockHash:          blockHash,
		TransactionVersion: b.Runtime.TransactionVersion,
	}

	err := ext.Sign(keyringPair, o)
	if err != nil {
		log.Error("SignExtrinsic failed", "err", err)
		return fmt.Errorf("sign extrinsic failed: %w", err)
	}
	return nil
}

func (b *ExtrinsicBuilder) BuildSigningPayload(call types.Call, era types.ExtrinsicEra, nonce uint64, tip uint64, blockHash types.Hash) ([]byte, error) {
	ext := types.NewExtrinsic(call)
	ext.Signature = types.ExtrinsicSignatureV4{
		Era:   era,
		Nonce: types.NewUCompactFromUInt(nonce),
		Tip:   types.NewUCompactFromUInt(tip),
	}

	var buf bytes.Buffer
	encoder := scale.NewEncoder(&buf)
	err := encoder.Encode(ext)
	if err != nil {
		return nil, fmt.Errorf("encode extrinsic failed: %w", err)
	}

	additionalEncoded := b.encodeAdditionalData(era, blockHash)
	payload := append(buf.Bytes(), additionalEncoded...)

	if len(payload) > 256 {
		hash := blake2b.Sum256(payload)
		return hash[:], nil
	}

	return payload, nil
}

func (b *ExtrinsicBuilder) encodeAdditionalData(era types.ExtrinsicEra, blockHash types.Hash) []byte {
	var buf bytes.Buffer
	encoder := scale.NewEncoder(&buf)
	encoder.Encode(b.Runtime.SpecVersion)
	encoder.Encode(b.Runtime.TransactionVersion)
	encoder.Encode(b.GenesisHash)
	encoder.Encode(blockHash)
	return buf.Bytes()
}

func HashSigningPayload(payload []byte) types.Hash {
	hash := blake2b.Sum256(payload)
	return types.NewHash(hash[:])
}
