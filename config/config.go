package config

import (
	"os"

	"gopkg.in/yaml.v2"

	"github.com/ethereum/go-ethereum/log"
)

type Server struct {
	Port string `yaml:"port"`
	Host string `yaml:"host"`
}

type Node struct {
	RpcUrl        string              `yaml:"rpc_url"`
	RpcUser       string              `yaml:"rpc_user"`
	RpcPass       string              `yaml:"rpc_pass"`
	DataApiUrl    string              `yaml:"data_api_url"`
	DataApiKey    string              `yaml:"data_api_key"`
	DataApiToken  string              `yaml:"data_api_token"`
	ContractAddr  []string            `yaml:"contract_addr"`
	TpApiUrl      string              `yaml:"tp_api_url"`
	TimeOut       uint64              `yaml:"time_out"`
	AA            AAConfig            `yaml:"aa"`
	DepositPolicy DepositPolicyConfig `yaml:"deposit_policy"`
}

type AAConfig struct {
	Enabled             bool   `yaml:"enabled"`
	EntryPoint          string `yaml:"entry_point"`
	Delegate            string `yaml:"delegate"`
	Paymaster           string `yaml:"paymaster"`
	SponsorAddress      string `yaml:"sponsor_address"`
	SponsorPrivateKey   string `yaml:"-"`
	VerifyingPrivateKey string `yaml:"-"`
	ChainIDNumeric      string `yaml:"chain_id_numeric"`
}

type DepositPolicyConfig struct {
	Enabled              bool                `yaml:"enabled"`
	RejectFailedReceipts bool                `yaml:"reject_failed_receipts"`
	DirectTransfer       PolicyToggle        `yaml:"direct_transfer"`
	AAHandleOps          AAHandleOpsPolicy   `yaml:"aa_handle_ops"`
	RouterReceipt        ReceiptSourcePolicy `yaml:"router_receipt"`
	BridgeReceipt        ReceiptSourcePolicy `yaml:"bridge_receipt"`
}

type PolicyToggle struct {
	Enabled bool `yaml:"enabled"`
}

type AAHandleOpsPolicy struct {
	Enabled     bool     `yaml:"enabled"`
	EntryPoints []string `yaml:"entry_points"`
}

type ReceiptSourcePolicy struct {
	Enabled        bool                 `yaml:"enabled"`
	AllowedSources []NamedAddressPolicy `yaml:"allowed_sources"`
}

type NamedAddressPolicy struct {
	Name    string `yaml:"name"`
	Address string `yaml:"address"`
	Action  string `yaml:"action"`
}

type WalletNode struct {
	Btc     Node `yaml:"btc"`
	Eth     Node `yaml:"eth"`
	BNB     Node `yaml:"bnb"`
	Arbi    Node `yaml:"arbi"`
	Op      Node `yaml:"op"`
	Sol     Node `yaml:"solana"`
	Base    Node `yaml:"evmbase"`
	Polygon Node `yaml:"polygon"`
	Tron    Node `yaml:"tron"`
}

type Chain struct {
	ChainName string `yaml:"chain_name"`
	ChainId   string `yaml:"chain_id"`
	Network   string `yaml:"network"`
}

type Config struct {
	RpcServer      Server     `yaml:"rpc_server"`
	HttpServer     Server     `yaml:"http_server"`
	WalletNode     WalletNode `yaml:"wallet_node"`
	NetWork        string     `yaml:"network"`
	Chains         []Chain    `yaml:"chains"`
	EnableApiCache bool       `yaml:"enable_api_cache"`
	AccessToken    string     `yaml:"access_token"`
}

func NewConfig(path, sponsorPrivateKey, verifyingPrivateKey string) (*Config, error) {
	var config = new(Config)
	h := log.NewTerminalHandler(os.Stdout, true)
	log.SetDefault(log.NewLogger(h))

	data, err := os.ReadFile(path)
	if err != nil {
		log.Error("read config file error", "err", err)
		return nil, err
	}

	err = yaml.Unmarshal(data, config)
	if err != nil {
		log.Error("unmarshal config file error", "err", err)
		return nil, err
	}

	config.WalletNode.BNB.AA.SponsorPrivateKey = sponsorPrivateKey
	config.WalletNode.BNB.AA.VerifyingPrivateKey = verifyingPrivateKey
	return config, nil
}

const UnsupportedChain = "Unsupport chain"
const UnsupportedOperation = UnsupportedChain
