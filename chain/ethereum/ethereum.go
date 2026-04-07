package ethereum

import (
	"context"

	"github.com/dapplink-labs/dapplink-wallet-api/chain"
	"github.com/dapplink-labs/dapplink-wallet-api/config"
	wallet_api "github.com/dapplink-labs/dapplink-wallet-api/protobuf/wallet-api"
)

const (
	ChainName string = "Ethereum"
)

type ChainAdaptor struct {
}

func (c ChainAdaptor) GetSupportChains(ctx context.Context, req *wallet_api.SupportChainRequest) (*wallet_api.SupportChainResponse, error) {
	//TODO implement me
	panic("implement me")
}

func NewChainAdaptor(conf *config.Config) (chain.IChainAdaptor, error) {
	return &ChainAdaptor{}, nil
}
