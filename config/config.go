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
	RpcUrl       string `yaml:"rpc_url"`
	RpcUser      string `yaml:"rpc_user"`
	RpcPass      string `yaml:"rpc_pass"`
	DataApiUrl   string `yaml:"data_api_url"`
	DataApiKey   string `yaml:"data_api_key"`
	DataApiToken string `yaml:"data_api_token"`
	TimeOut      uint64 `yaml:"time_out"`
}

type WalletNode struct {
	Eth     Node `yaml:"eth"`
	Arbi    Node `yaml:"arbi"`
	Op      Node `yaml:"op"`
	Sol     Node `yaml:"solana"`
	Base    Node `yaml:"evmbase"`
	Polygon Node `yaml:"polygon"`
}

type Config struct {
	RpcServer      Server     `yaml:"rpc_server"`
	HttpServer     Server     `yaml:"http_server"`
	WalletNode     WalletNode `yaml:"wallet_node"`
	NetWork        string     `yaml:"network"`
	Chains         []string   `yaml:"chains"`
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
