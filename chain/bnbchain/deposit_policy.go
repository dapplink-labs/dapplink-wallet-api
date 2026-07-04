package bnbchain

import (
	"strings"

	"github.com/ethereum/go-ethereum/common"

	"github.com/dapplink-labs/dapplink-wallet-api/config"
)

type depositPolicyIndex struct {
	enabled       bool
	routerSources map[string]struct{}
	bridgeSources map[string]struct{}
}

func newDepositPolicyIndex(policy config.DepositPolicyConfig) depositPolicyIndex {
	index := depositPolicyIndex{
		enabled:       policy.Enabled,
		routerSources: make(map[string]struct{}),
		bridgeSources: make(map[string]struct{}),
	}
	if !policy.Enabled {
		return index
	}

	if policy.RouterReceipt.Enabled {
		addAllowedSources(index.routerSources, policy.RouterReceipt.AllowedSources)
	}
	if policy.BridgeReceipt.Enabled {
		addAllowedSources(index.bridgeSources, policy.BridgeReceipt.AllowedSources)
	}
	return index
}

func addAllowedSources(out map[string]struct{}, sources []config.NamedAddressPolicy) {
	for _, source := range sources {
		if isAcceptedReceiptSource(source) {
			out[normalizeAddress(source.Address)] = struct{}{}
		}
	}
}

func isAcceptedReceiptSource(source config.NamedAddressPolicy) bool {
	if !strings.EqualFold(strings.TrimSpace(source.Action), "accept") {
		return false
	}
	if !common.IsHexAddress(source.Address) {
		return false
	}
	return common.HexToAddress(source.Address) != (common.Address{})
}

func (i depositPolicyIndex) allowReceiptSource(address string) bool {
	if !i.enabled {
		return false
	}
	normalized := normalizeAddress(address)
	if _, ok := i.routerSources[normalized]; ok {
		return true
	}
	if _, ok := i.bridgeSources[normalized]; ok {
		return true
	}
	return false
}
