package chaindispatcher

import (
	"context"
	"runtime/debug"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ethereum/go-ethereum/log"

	"github.com/dapplink-labs/dapplink-wallet-api/chain"
	"github.com/dapplink-labs/dapplink-wallet-api/chain/bitcoin"
	"github.com/dapplink-labs/dapplink-wallet-api/chain/ethereum"
	"github.com/dapplink-labs/dapplink-wallet-api/chain/solana"
	"github.com/dapplink-labs/dapplink-wallet-api/chain/tron"
	"github.com/dapplink-labs/dapplink-wallet-api/config"
	"github.com/dapplink-labs/dapplink-wallet-api/protobuf/common"
	"github.com/dapplink-labs/dapplink-wallet-api/protobuf/walletapi"
)

const GrpcToken = "DappLinkTheWeb3"

type CommonRequest interface {
	GetConsumerToken() string
}

type ChainRequest interface {
	GetChainId() string
}

type CommonReply = walletapi.CommonResponse

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
			Code: common.ReturnCode_ERROR,
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
			Code: common.ReturnCode_ERROR,
			Msg:  config.UnsupportedOperation,
		}
	}
	return nil
}

func (d *ChainDispatcher) GetSupportChains(ctx context.Context, request *walletapi.SupportChainRequest) (*walletapi.SupportChainResponse, error) {
	var supportChainList []*walletapi.SupportChain
	for _, chainItem := range d.conf.Chains {
		sc := &walletapi.SupportChain{
			ChainId:   chainItem.ChainId,
			ChainName: chainItem.ChainName,
			Network:   chainItem.Network,
		}
		supportChainList = append(supportChainList, sc)
	}
	return &walletapi.SupportChainResponse{
		Code:   common.ReturnCode_SUCCESS,
		Msg:    "success",
		Chains: supportChainList,
	}, nil
}

func (d *ChainDispatcher) ConvertAddresses(ctx context.Context, request *walletapi.ConvertAddressesRequest) (*walletapi.ConvertAddressesResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &walletapi.ConvertAddressesResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  "failed to convert addresses",
		}, nil
	}
	return d.registry[request.ChainId].ConvertAddresses(ctx, request)
}

func (d *ChainDispatcher) ValidAddresses(ctx context.Context, request *walletapi.ValidAddressesRequest) (*walletapi.ValidAddressesResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &walletapi.ValidAddressesResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  "failed to convert addresses",
		}, nil
	}
	return d.registry[request.ChainId].ValidAddresses(ctx, request)
}

func (d *ChainDispatcher) GetLastestBlock(ctx context.Context, request *walletapi.LastestBlockRequest) (*walletapi.LastestBlockResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &walletapi.LastestBlockResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  "get lastest block failed",
		}, nil
	}
	return d.registry[request.ChainId].GetLastestBlock(ctx, request)
}

func (d *ChainDispatcher) GetBlock(ctx context.Context, request *walletapi.BlockRequest) (*walletapi.BlockResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &walletapi.BlockResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  "get block info failed",
		}, nil
	}
	return d.registry[request.ChainId].GetBlock(ctx, request)
}

func (d *ChainDispatcher) GetTransactionByHash(ctx context.Context, request *walletapi.TransactionByHashRequest) (*walletapi.TransactionByHashResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &walletapi.TransactionByHashResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  "get transaction by hash failed",
		}, nil
	}
	return d.registry[request.ChainId].GetTransactionByHash(ctx, request)
}

func (d *ChainDispatcher) GetTransactionByAddress(ctx context.Context, request *walletapi.TransactionByAddressRequest) (*walletapi.TransactionByAddressResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &walletapi.TransactionByAddressResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  "get transaction by address failed",
		}, nil
	}
	return d.registry[request.ChainId].GetTransactionByAddress(ctx, request)
}

func (d *ChainDispatcher) GetAccountBalance(ctx context.Context, request *walletapi.AccountBalanceRequest) (*walletapi.AccountBalanceResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &walletapi.AccountBalanceResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  "get account balance failed",
		}, nil
	}
	return d.registry[request.ChainId].GetAccountBalance(ctx, request)
}

func (d *ChainDispatcher) SendTransaction(ctx context.Context, request *walletapi.SendTransactionsRequest) (*walletapi.SendTransactionResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &walletapi.SendTransactionResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  "send transaction failed",
		}, nil
	}
	return d.registry[request.ChainId].SendTransaction(ctx, request)
}

func (d *ChainDispatcher) BuildTransactionSchema(ctx context.Context, request *walletapi.TransactionSchemaRequest) (*walletapi.TransactionSchemaResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &walletapi.TransactionSchemaResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  "build transaction schema failed",
		}, nil
	}
	return d.registry[request.ChainId].BuildTransactionSchema(ctx, request)
}

func (d *ChainDispatcher) BuildUnSignTransaction(ctx context.Context, request *walletapi.UnSignTransactionRequest) (*walletapi.UnSignTransactionResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &walletapi.UnSignTransactionResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  "build unsigned transaction failed",
		}, nil
	}
	return d.registry[request.ChainId].BuildUnSignTransaction(ctx, request)
}

func (d *ChainDispatcher) BuildSignedTransaction(ctx context.Context, request *walletapi.SignedTransactionRequest) (*walletapi.SignedTransactionResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &walletapi.SignedTransactionResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  "build signed transaction failed",
		}, nil
	}
	return d.registry[request.ChainId].BuildSignedTransaction(ctx, request)
}

func (d *ChainDispatcher) GetAddressApproveList(ctx context.Context, request *walletapi.AddressApproveListRequest) (*walletapi.AddressApproveListResponse, error) {
	resp := d.preHandler(request)
	if resp != nil {
		return &walletapi.AddressApproveListResponse{
			Code: common.ReturnCode_ERROR,
			Msg:  "get address approve list failed",
		}, nil
	}
	return d.registry[request.ChainId].GetAddressApproveList(ctx, request)
}
