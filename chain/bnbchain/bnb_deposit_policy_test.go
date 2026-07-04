package bnbchain

import (
	"os"
	"testing"

	"github.com/dapplink-labs/dapplink-wallet-api/config"
	"gopkg.in/yaml.v3"
)

func TestDepositPolicyAllowsConfiguredRouterSource(t *testing.T) {
	router := "0x1111111111111111111111111111111111111111"
	policy := config.DepositPolicyConfig{
		Enabled: true,
		RouterReceipt: config.ReceiptSourcePolicy{
			Enabled: true,
			AllowedSources: []config.NamedAddressPolicy{{
				Name:    "PancakeSwap Router",
				Address: router,
				Action:  "accept",
			}},
		},
	}

	index := newDepositPolicyIndex(policy)
	if !index.allowReceiptSource(router) {
		t.Fatal("expected configured router to be allowed")
	}
	if !index.allowReceiptSource("0x1111111111111111111111111111111111111111") {
		t.Fatal("expected address matching to be case-insensitive and normalized")
	}
	if index.allowReceiptSource("0x2222222222222222222222222222222222222222") {
		t.Fatal("unexpected unknown router allowed")
	}
}

func TestDepositPolicyRejectsZeroAndNonAcceptSources(t *testing.T) {
	policy := config.DepositPolicyConfig{
		Enabled: true,
		RouterReceipt: config.ReceiptSourcePolicy{
			Enabled: true,
			AllowedSources: []config.NamedAddressPolicy{
				{
					Name:    "Zero",
					Address: "0x0000000000000000000000000000000000000000",
					Action:  "accept",
				},
				{
					Name:    "Disabled",
					Address: "0x3333333333333333333333333333333333333333",
					Action:  "reject",
				},
			},
		},
	}

	index := newDepositPolicyIndex(policy)
	if index.allowReceiptSource("0x0000000000000000000000000000000000000000") {
		t.Fatal("zero address must not be allowed as a receipt source")
	}
	if index.allowReceiptSource("0x3333333333333333333333333333333333333333") {
		t.Fatal("non-accept receipt source must not be allowed")
	}
}

func TestDepositPolicyExampleRoutersAreAllowed(t *testing.T) {
	data, err := os.ReadFile("../../config/bnb_deposit_policy.example.yml")
	if err != nil {
		t.Fatalf("read example policy: %v", err)
	}

	var example struct {
		DepositPolicy config.DepositPolicyConfig `yaml:"deposit_policy"`
	}
	if err := yaml.Unmarshal(data, &example); err != nil {
		t.Fatalf("parse example policy: %v", err)
	}

	index := newDepositPolicyIndex(example.DepositPolicy)
	for _, source := range example.DepositPolicy.RouterReceipt.AllowedSources {
		if !index.allowReceiptSource(source.Address) {
			t.Fatalf("expected example router source %s %s to be allowed", source.Name, source.Address)
		}
	}
}

func TestDepositPolicyExampleIncludesUSDCInTokenWhitelist(t *testing.T) {
	data, err := os.ReadFile("../../config/bnb_deposit_policy.example.yml")
	if err != nil {
		t.Fatalf("read example policy: %v", err)
	}

	var example config.Node
	if err := yaml.Unmarshal(data, &example); err != nil {
		t.Fatalf("parse example policy: %v", err)
	}

	index := newContractAddrIndex(example.ContractAddr)
	if _, ok := index[normalizeAddress("0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d")]; !ok {
		t.Fatal("expected example parser token whitelist to include BSC USDC")
	}
}
