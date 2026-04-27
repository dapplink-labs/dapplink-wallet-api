package chaindispatcher

import (
	"context"
	"runtime/debug"
	"strings"

	"github.com/dapplink-labs/dapplink-wallet-api/chain/solana"
	"github.com/dapplink-labs/dapplink-wallet-api/chain/tron"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ethereum/go-ethereum/log"

	"github.com/dapplink-labs/dapplink-wallet-api/chain"
	"github.com/dapplink-labs/dapplink-wallet-api/chain/bitcoin"
	"github.com/dapplink-labs/dapplink-wallet-api/chain/ethereum"
	"github.com/dapplink-labs/dapplink-wallet-api/config"
	wallet_api "github.com/dapplink-labs/dapplink-wallet-api/protobuf/wallet-api"
)

const GrpcToken = "DappLinkTheWeb3"

type CommonRequest interface {
	GetConsumerToken() string
}

type ChainRequest interface {
	GetChainId() string
}

type CommonReply = wallet_api.CommonResponse

type ChainId = string

type ChainDispatcher struct {
	conf     *config.Config
	registry map[ChainId]chain.IChainAdaptor
}

func NewChainDispatcher(conf *config.Config) (*ChainDispatcher, error) {
	dispatcher := ChainDispatcher{
		conf:     conf,
		registry: make(map[ChainId]chain.IChainAdaptor),
	}

	chainAdaptorFactoryMap := map[string]func(conf *config.Config) (chain.IChainAdaptor, error){
		ethereum.ChainID: ethereum.NewChainAdaptor,
		bitcoin.ChainID:  bitcoin.NewChainAdaptor,
		solana.ChainID:   solana.NewChainAdaptor,
		tron.ChainID:     tron.NewChainAdaptor,
	}
	supportedChains := []string{
		ethereum.ChainID,
		bitcoin.ChainID,
		solana.ChainID,
		tron.ChainID,
	}

	for _, c := range conf.Chains {
		if factory, ok := chainAdaptorFactoryMap[c.ChainId]; ok {
			adaptor, err := factory(conf)
			if err != nil {
				log.Crit("failed to setup chain", "chain", c, "error", err)
			}
			dispatcher.registry[c.ChainId] = adaptor
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

	pos := strings.LastIndex(info.FullMethod, "/")
	method := info.FullMethod[pos+1:]
	consumerToken := req.(CommonRequest).GetConsumerToken()
	if consumerToken != GrpcToken {
		return CommonReply{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  "Consumer token is not valid",
		}, status.Error(codes.PermissionDenied, "access denied")
	}
	log.Info(method, "consumerToken", consumerToken, "req", req)
	resp, err = handler(ctx, req)
	log.Debug("Finish handling", "resp", resp, "err", err)
	return
}

func (d *ChainDispatcher) preHandler(req interface{}) (resp *CommonReply) {
	chainId := req.(ChainRequest).GetChainId()
	log.Debug("chain", chainId, "req", req)
	if _, ok := d.registry[chainId]; !ok {
		return &CommonReply{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  config.UnsupportedOperation,
		}
	}
	return nil
}

func (d *ChainDispatcher) GetSupportChains(ctx context.Context, request *wallet_api.SupportChainRequest) (*wallet_api.SupportChainResponse, error) {
	var supportChainList []*wallet_api.SupportChain
	for _, chainItem := range d.conf.Chains {
		sc := &wallet_api.SupportChain{
			ChainId:   chainItem.ChainId,
			ChainName: chainItem.ChainName,
			Network:   chainItem.Network,
		}
		supportChainList = append(supportChainList, sc)
	}
	return &wallet_api.SupportChainResponse{
		Code:   wallet_api.ApiReturnCode_APISUCCESS,
		Msg:    "success",
		Chains: supportChainList,
	}, nil
}

func (d *ChainDispatcher) ConvertAddresses(ctx context.Context, request *wallet_api.ConvertAddressesRequest) (*wallet_api.ConvertAddressesResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &wallet_api.ConvertAddressesResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  "failed to convert addresses",
		}, nil
	}
	return d.registry[request.ChainId].ConvertAddresses(ctx, request)
}

func (d *ChainDispatcher) ValidAddresses(ctx context.Context, request *wallet_api.ValidAddressesRequest) (*wallet_api.ValidAddressesResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &wallet_api.ValidAddressesResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  "failed to convert addresses",
		}, nil
	}
	return d.registry[request.ChainId].ValidAddresses(ctx, request)
}

func (d *ChainDispatcher) GetLastestBlock(ctx context.Context, request *wallet_api.LastestBlockRequest) (*wallet_api.LastestBlockResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &wallet_api.LastestBlockResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  "get lastest block failed",
		}, nil
	}
	return d.registry[request.ChainId].GetLastestBlock(ctx, request)
}

func (d *ChainDispatcher) GetBlock(ctx context.Context, request *wallet_api.BlockRequest) (*wallet_api.BlockResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &wallet_api.BlockResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  "get block info failed",
		}, nil
	}
	return d.registry[request.ChainId].GetBlock(ctx, request)
}

func (d *ChainDispatcher) GetTransactionByHash(ctx context.Context, request *wallet_api.TransactionByHashRequest) (*wallet_api.TransactionByHashResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &wallet_api.TransactionByHashResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  "get transaction by hash failed",
		}, nil
	}
	return d.registry[request.ChainId].GetTransactionByHash(ctx, request)
}

func (d *ChainDispatcher) GetTransactionByAddress(ctx context.Context, request *wallet_api.TransactionByAddressRequest) (*wallet_api.TransactionByAddressResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &wallet_api.TransactionByAddressResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  "get transaction by address failed",
		}, nil
	}
	return d.registry[request.ChainId].GetTransactionByAddress(ctx, request)
}

func (d *ChainDispatcher) GetAccountBalance(ctx context.Context, request *wallet_api.AccountBalanceRequest) (*wallet_api.AccountBalanceResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &wallet_api.AccountBalanceResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  "get account balance failed",
		}, nil
	}
	return d.registry[request.ChainId].GetAccountBalance(ctx, request)
}

func (d *ChainDispatcher) SendTransaction(ctx context.Context, request *wallet_api.SendTransactionsRequest) (*wallet_api.SendTransactionResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &wallet_api.SendTransactionResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  "send transaction failed",
		}, nil
	}
	return d.registry[request.ChainId].SendTransaction(ctx, request)
}

func (d *ChainDispatcher) BuildTransactionSchema(ctx context.Context, request *wallet_api.TransactionSchemaRequest) (*wallet_api.TransactionSchemaResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &wallet_api.TransactionSchemaResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  "build transaction schema failed",
		}, nil
	}
	return d.registry[request.ChainId].BuildTransactionSchema(ctx, request)
}

func (d *ChainDispatcher) BuildUnSignTransaction(ctx context.Context, request *wallet_api.UnSignTransactionRequest) (*wallet_api.UnSignTransactionResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &wallet_api.UnSignTransactionResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  "build unsigned transaction failed",
		}, nil
	}
	return d.registry[request.ChainId].BuildUnSignTransaction(ctx, request)
}

func (d *ChainDispatcher) BuildSignedTransaction(ctx context.Context, request *wallet_api.SignedTransactionRequest) (*wallet_api.SignedTransactionResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &wallet_api.SignedTransactionResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  "build signed transaction failed",
		}, nil
	}
	return d.registry[request.ChainId].BuildSignedTransaction(ctx, request)
}

func (d *ChainDispatcher) GetAddressApproveList(ctx context.Context, request *wallet_api.AddressApproveListRequest) (*wallet_api.AddressApproveListResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &wallet_api.AddressApproveListResponse{
			Code: wallet_api.ApiReturnCode_APIERROR,
			Msg:  "get address approve list failed",
		}, nil
	}
	return d.registry[request.ChainId].GetAddressApproveList(ctx, request)
}
