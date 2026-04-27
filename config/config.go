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
	RpcUrl          string `yaml:"rpc_url"`
	RpcUser         string `yaml:"rpc_user"`
	RpcPass         string `yaml:"rpc_pass"`
	DataApiUrl      string `yaml:"data_api_url"`
	DataApiKey      string `yaml:"data_api_key"`
	DataApiToken    string `yaml:"data_api_token"`
	TpApiUrl        string `yaml:"tp_api_url"`
	TimeOut         uint64 `yaml:"time_out"`
	SubstrateRpcUrl string `yaml:"substrate_rpc_url"`
}

type WalletNode struct {
	Btc     Node `yaml:"btc"`
	Eth     Node `yaml:"eth"`
	Arbi    Node `yaml:"arbi"`
	Op      Node `yaml:"op"`
	Sol     Node `yaml:"solana"`
	Base    Node `yaml:"evmbase"`
	Polygon Node `yaml:"polygon"`
	Tron    Node `yaml:"tron"`
	Dot     Node `yaml:"dot"`
	Ksm     Node `yaml:"ksm"`
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
}

func NewConfig(path string) (*Config, error) {
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
	return config, nil
}

const UnsupportedChain = "Unsupport chain"
const UnsupportedOperation = UnsupportedChain
