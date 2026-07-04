package bnbchain

import (
	"testing"

	common2 "github.com/dapplink-labs/dapplink-wallet-api/protobuf/common"
)

func TestBalanceQueryContractAddressTreatsBNBPseudoAddressAsNative(t *testing.T) {
	if got := balanceQueryContractAddress(NativeTokenAddress); got != "" {
		t.Fatalf("balanceQueryContractAddress(native sentinel) = %q, want empty native address", got)
	}
	if got := balanceQueryContractAddress("0x00"); got != "" {
		t.Fatalf("balanceQueryContractAddress(0x00) = %q, want empty native address", got)
	}

	usdt := "0x55d398326f99059fF775485246999027B3197955"
	if got := balanceQueryContractAddress(usdt); got != usdt {
		t.Fatalf("balanceQueryContractAddress(usdt) = %q, want %q", got, usdt)
	}
}

func TestAccountBalanceSuccessResponseUsesSuccessCode(t *testing.T) {
	resp := accountBalanceSuccessResponse("123")
	if resp.Code != common2.ReturnCode_SUCCESS {
		t.Fatalf("Code = %s, want SUCCESS", resp.Code)
	}
	if resp.Balance != "123" {
		t.Fatalf("Balance = %q, want 123", resp.Balance)
	}
}
