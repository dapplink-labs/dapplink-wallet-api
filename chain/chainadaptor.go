package chain

import (
	"context"

	"github.com/dapplink-labs/dapplink-wallet-api/protobuf/walletapi"
)

type IChainAdaptor interface {
	// GetSupportChains(ctx context.Context, req *walletapi.SupportChainRequest) (*walletapi.SupportChainResponse, error)

	ConvertAddresses(ctx context.Context, req *walletapi.ConvertAddressesRequest) (*walletapi.ConvertAddressesResponse, error)
	ValidAddresses(ctx context.Context, req *walletapi.ValidAddressesRequest) (*walletapi.ValidAddressesResponse, error)

	GetLastestBlock(ctx context.Context, req *walletapi.LastestBlockRequest) (*walletapi.LastestBlockResponse, error)
	GetBlock(ctx context.Context, req *walletapi.BlockRequest) (*walletapi.BlockResponse, error)
	GetBatchBlock(ctx context.Context, req *walletapi.BatchBlockRequest) (*walletapi.BatchBlockResponse, error)

	GetTransactionByHash(ctx context.Context, req *walletapi.TransactionByHashRequest) (*walletapi.TransactionByHashResponse, error)
	GetTransactionByAddress(ctx context.Context, req *walletapi.TransactionByAddressRequest) (*walletapi.TransactionByAddressResponse, error)
	GetAccountBalance(ctx context.Context, req *walletapi.AccountBalanceRequest) (*walletapi.AccountBalanceResponse, error)

	SendTransaction(ctx context.Context, req *walletapi.SendTransactionsRequest) (*walletapi.SendTransactionResponse, error)

	BuildTransactionSchema(ctx context.Context, request *walletapi.TransactionSchemaRequest) (*walletapi.TransactionSchemaResponse, error)

	BuildUnSignTransaction(ctx context.Context, request *walletapi.UnSignTransactionRequest) (*walletapi.UnSignTransactionResponse, error)

	BuildSignedTransaction(ctx context.Context, request *walletapi.SignedTransactionRequest) (*walletapi.SignedTransactionResponse, error)

	BuildSponsoredTransfer(ctx context.Context, request *walletapi.SponsoredTransferRequest) (*walletapi.SponsoredTransferBuildResponse, error)
	SendSponsoredTransfer(ctx context.Context, request *walletapi.SponsoredTransferSendRequest) (*walletapi.SendTransactionResponse, error)

	GetAddressApproveList(ctx context.Context, request *walletapi.AddressApproveListRequest) (*walletapi.AddressApproveListResponse, error)

	CallContract(ctx context.Context, request *walletapi.CallContractRequest) (*walletapi.CallContractResponse, error)
}
