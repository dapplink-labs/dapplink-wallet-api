package substratebase

import (
	"context"
	"fmt"
	"time"

	"github.com/centrifuge/go-substrate-rpc-client/v4/client"
	"github.com/centrifuge/go-substrate-rpc-client/v4/rpc"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/ethereum/go-ethereum/log"

	"github.com/dapplink-labs/dapplink-wallet-api/common/retry"
)

const (
	defaultSubstrateDialTimeout  = 2 * time.Minute
	defaultSubstrateDialAttempts = 3
)

type SubstrateClient interface {
	GetBlockHash(blockNumber uint64) (types.Hash, error)
	GetBlock(blockHash types.Hash) (*types.SignedBlock, error)
	GetBlockLatest() (*types.SignedBlock, error)
	GetHeader(blockHash types.Hash) (*types.Header, error)
	GetHeaderLatest() (*types.Header, error)
	GetMetadataLatest() (*types.Metadata, error)
	GetRuntimeVersionLatest() (*types.RuntimeVersion, error)
	GetStorageKeys(prefix types.StorageKey, blockHash types.Hash) ([]types.StorageKey, error)
	GetStorageRaw(key types.StorageKey, blockHash types.Hash) (*types.StorageDataRaw, error)
	GetGenesisHash() (types.Hash, error)
	GetChainProperties() (*types.ChainProperties, error)
	GetAccountInfoLatest(address string) (*types.AccountInfo, error)
	SubmitExtrinsic(ext types.Extrinsic) (types.Hash, error)
	GetChainName() (string, error)
	Close()
}

type substrateClient struct {
	api *rpc.RPC
	cl  client.Client
}

func NewSubstrateClient(rpcUrl string) (SubstrateClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultSubstrateDialTimeout)
	defer cancel()

	bOff := retry.Exponential()
	var subRPC *rpc.RPC
	var cl client.Client
	var err error

	_, err = retry.Do(ctx, defaultSubstrateDialAttempts, bOff, func() (*rpc.RPC, error) {
		c, e := client.Connect(rpcUrl)
		if e != nil {
			return nil, fmt.Errorf("failed to connect substrate rpc (%s): %w", rpcUrl, e)
		}
		r, e := rpc.NewRPC(c)
		if e != nil {
			return nil, fmt.Errorf("failed to create substrate rpc (%s): %w", rpcUrl, e)
		}
		subRPC = r
		cl = c
		return r, nil
	})
	if err != nil {
		return nil, err
	}

	return &substrateClient{
		api: subRPC,
		cl:  cl,
	}, nil
}

func (c *substrateClient) GetBlockHash(blockNumber uint64) (types.Hash, error) {
	hash, err := c.api.Chain.GetBlockHash(blockNumber)
	if err != nil {
		log.Error("GetBlockHash failed", "blockNumber", blockNumber, "err", err)
		return types.Hash{}, err
	}
	return hash, nil
}

func (c *substrateClient) GetBlock(blockHash types.Hash) (*types.SignedBlock, error) {
	block, err := c.api.Chain.GetBlock(blockHash)
	if err != nil {
		log.Error("GetBlock failed", "blockHash", blockHash.Hex(), "err", err)
		return nil, err
	}
	return block, nil
}

func (c *substrateClient) GetBlockLatest() (*types.SignedBlock, error) {
	block, err := c.api.Chain.GetBlockLatest()
	if err != nil {
		log.Error("GetBlockLatest failed", "err", err)
		return nil, err
	}
	return block, nil
}

func (c *substrateClient) GetHeader(blockHash types.Hash) (*types.Header, error) {
	header, err := c.api.Chain.GetHeader(blockHash)
	if err != nil {
		log.Error("GetHeader failed", "blockHash", blockHash.Hex(), "err", err)
		return nil, err
	}
	return header, nil
}

func (c *substrateClient) GetHeaderLatest() (*types.Header, error) {
	header, err := c.api.Chain.GetHeaderLatest()
	if err != nil {
		log.Error("GetHeaderLatest failed", "err", err)
		return nil, err
	}
	return header, nil
}

func (c *substrateClient) GetMetadataLatest() (*types.Metadata, error) {
	meta, err := c.api.State.GetMetadataLatest()
	if err != nil {
		log.Error("GetMetadataLatest failed", "err", err)
		return nil, err
	}
	return meta, nil
}

func (c *substrateClient) GetRuntimeVersionLatest() (*types.RuntimeVersion, error) {
	rv, err := c.api.State.GetRuntimeVersionLatest()
	if err != nil {
		log.Error("GetRuntimeVersionLatest failed", "err", err)
		return nil, err
	}
	return rv, nil
}

func (c *substrateClient) GetStorageKeys(prefix types.StorageKey, blockHash types.Hash) ([]types.StorageKey, error) {
	keys, err := c.api.State.GetKeys(prefix, blockHash)
	if err != nil {
		log.Error("GetStorageKeys failed", "prefix", prefix.Hex(), "blockHash", blockHash.Hex(), "err", err)
		return nil, err
	}
	return keys, nil
}

func (c *substrateClient) GetStorageRaw(key types.StorageKey, blockHash types.Hash) (*types.StorageDataRaw, error) {
	data, err := c.api.State.GetStorageRaw(key, blockHash)
	if err != nil {
		log.Error("GetStorageRaw failed", "blockHash", blockHash.Hex(), "err", err)
		return nil, err
	}
	return data, nil
}

func (c *substrateClient) GetGenesisHash() (types.Hash, error) {
	hash, err := c.api.Chain.GetBlockHash(0)
	if err != nil {
		log.Error("GetGenesisHash failed", "err", err)
		return types.Hash{}, err
	}
	return hash, nil
}

func (c *substrateClient) GetChainProperties() (*types.ChainProperties, error) {
	props, err := c.api.System.Properties()
	if err != nil {
		log.Error("GetChainProperties failed", "err", err)
		return nil, err
	}
	return &props, nil
}

func (c *substrateClient) GetAccountInfoLatest(address string) (*types.AccountInfo, error) {
	meta, err := c.GetMetadataLatest()
	if err != nil {
		return nil, err
	}

	accountID, err := types.NewAccountIDFromHexString(address)
	if err != nil {
		return nil, fmt.Errorf("invalid account address: %w", err)
	}

	key, err := types.CreateStorageKey(meta, "System", "Account", accountID.ToBytes())
	if err != nil {
		return nil, fmt.Errorf("create storage key failed: %w", err)
	}

	var accountInfo types.AccountInfo
	_, err = c.api.State.GetStorageLatest(key, &accountInfo)
	if err != nil {
		return nil, fmt.Errorf("get account info failed: %w", err)
	}

	return &accountInfo, nil
}

func (c *substrateClient) SubmitExtrinsic(ext types.Extrinsic) (types.Hash, error) {
	hash, err := c.api.Author.SubmitExtrinsic(ext)
	if err != nil {
		log.Error("SubmitExtrinsic failed", "err", err)
		return types.Hash{}, err
	}
	log.Info("SubmitExtrinsic success", "hash", hash.Hex())
	return hash, nil
}

func (c *substrateClient) GetChainName() (string, error) {
	name, err := c.api.System.Chain()
	if err != nil {
		log.Error("GetChainName failed", "err", err)
		return "", err
	}
	return string(name), nil
}

func (c *substrateClient) Close() {
	if c.cl != nil {
		c.cl.Close()
	}
}
