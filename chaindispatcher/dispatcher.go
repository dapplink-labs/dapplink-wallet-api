package chaindispatcher

import (
	"context"
	"runtime/debug"

	"github.com/dapplink-labs/dapplink-wallet-api/chain"
	"github.com/dapplink-labs/dapplink-wallet-api/chain/ethereum"
	"github.com/dapplink-labs/dapplink-wallet-api/config"
	wallet_api "github.com/dapplink-labs/dapplink-wallet-api/protobuf/wallet-api"
	"github.com/ethereum/go-ethereum/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CommonRequest interface {
}

type CommonReply = wallet_api.SupportChainResponse

type ChainType = string

type ChainDispatcher struct {
	registry map[ChainType]chain.IChainAdaptor
}

func NewChainDispatcher(conf *config.Config) (*ChainDispatcher, error) {
	dispatcher := ChainDispatcher{
		registry: make(map[ChainType]chain.IChainAdaptor),
	}

	chainAdaptorFactoryMap := map[string]func(conf *config.Config) (chain.IChainAdaptor, error){
		ethereum.ChainName: ethereum.NewChainAdaptor,
	}
	supportedChains := []string{
		ethereum.ChainName,
	}

	for _, c := range conf.Chains {
		if factory, ok := chainAdaptorFactoryMap[c]; ok {
			adaptor, err := factory(conf)
			if err != nil {
				log.Crit("failed to setup chain", "chain", c, "error", err)
			}
			dispatcher.registry[c] = adaptor
		} else {
			log.Error("unsupported chain", "chain", c, "supportedChains", supportedChains)
		}
	}
	return &dispatcher, nil
}

func (d *ChainDispatcher) Interceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	defer func() {
		if e := recover(); e != nil {
			log.Error("panic error", "msg", e)
			log.Debug(string(debug.Stack()))
			err = status.Errorf(codes.Internal, "Panic err: %v", e)
		}
	}()

	//pos := strings.LastIndex(info.FullMethod, "/")
	//method := info.FullMethod[pos+1:]

	resp, err = handler(ctx, req)
	log.Debug("Finish handling", "resp", resp, "err", err)
	return
}

func (d *ChainDispatcher) preHandler(req interface{}) (resp *CommonReply) {
	// chainName := "Ethereum" // req.(CommonRequest).GetChainName()
	//log.Debug("chain", chainName, "req", req)
	//if _, ok := d.registry[chainName]; !ok {
	//	return &CommonReply{
	//		Code:    wallet_api.ReturnCode_ERROR,
	//		Message: config.UnsupportedOperation,
	//	}
	//}
	return &CommonReply{}
}

func (d *ChainDispatcher) GetSupportChains(ctx context.Context, request *wallet_api.SupportChainRequest) (*wallet_api.SupportChainResponse, error) {
	// resp := d.preHandler(request)
	var supportChainList []*wallet_api.SupportChain
	supportChainItem := &wallet_api.SupportChain{
		ChainName: "ethereum",
		ChainId:   "1",
		Network:   "mainnet",
	}
	supportChainList = append(supportChainList, supportChainItem)
	return &wallet_api.SupportChainResponse{
		Code:    wallet_api.ReturnCode_SUCCESS,
		Message: "success",
		Chains:  supportChainList,
	}, nil
}
