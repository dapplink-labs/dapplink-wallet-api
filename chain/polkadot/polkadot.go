package polkadot

import (
	"github.com/dapplink-labs/dapplink-wallet-api/chain/substratebase"
	"github.com/dapplink-labs/dapplink-wallet-api/config"
)

const (
	ChainIDEvm       string = "DappLinkPolkadotHub"
	ChainIDSubstrate string = "DappLinkPolkadotSubstrate"

	KusamaChainIDSubstrate string = "DappLinkKusamaSubstrate"
)

func getNodeConfig(chainId string, conf *config.Config) config.Node {
	switch chainId {
	case ChainIDEvm, ChainIDSubstrate:
		return conf.WalletNode.Dot
	case KusamaChainIDSubstrate:
		return conf.WalletNode.Ksm
	default:
		return conf.WalletNode.Dot
	}
}

func getSS58Prefix(chainId string) uint8 {
	switch chainId {
	case ChainIDEvm, ChainIDSubstrate:
		return uint8(substratebase.PolkadotSS58Prefix)
	case KusamaChainIDSubstrate:
		return uint8(substratebase.KusamaSS58Prefix)
	default:
		return uint8(substratebase.PolkadotSS58Prefix)
	}
}
