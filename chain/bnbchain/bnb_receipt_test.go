package bnbchain

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestParseUserOpBEP20TransfersFromReceiptReturnsAllWhitelistedTransfers(t *testing.T) {
	usdt := common.HexToAddress("0x55d398326f99059fF775485246999027B3197955")
	yolo := common.HexToAddress("0x1111111111111111111111111111111111111111")
	ignored := common.HexToAddress("0x2222222222222222222222222222222222222222")
	fromA := common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	toA := common.HexToAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	fromB := common.HexToAddress("0xcccccccccccccccccccccccccccccccccccccccc")
	toB := common.HexToAddress("0xdddddddddddddddddddddddddddddddddddddddd")

	c := &ChainAdaptor{contractAddrIndex: newContractAddrIndex([]string{usdt.Hex(), yolo.Hex()})}

	transfers := c.parseUserOpBEP20TransfersFromReceipt([]*types.Log{
		transferLog(usdt, fromA, toA, big.NewInt(100), 4),
		transferLog(ignored, fromA, toA, big.NewInt(999), 5),
		transferLog(yolo, fromB, toB, big.NewInt(200), 6),
	})

	if len(transfers) != 2 {
		t.Fatalf("len(transfers) = %d, want 2", len(transfers))
	}
	if transfers[0].Contract != usdt.Hex() || transfers[0].From != fromA.Hex() || transfers[0].To != toA.Hex() || transfers[0].Amount != "100" || transfers[0].Index != 4 {
		t.Fatalf("first transfer mismatch: %#v", transfers[0])
	}
	if transfers[1].Contract != yolo.Hex() || transfers[1].From != fromB.Hex() || transfers[1].To != toB.Hex() || transfers[1].Amount != "200" || transfers[1].Index != 6 {
		t.Fatalf("second transfer mismatch: %#v", transfers[1])
	}
}

func TestParseUserOpBEP20TransfersFromReceiptRejectsAllWhenTokenWhitelistEmpty(t *testing.T) {
	usdt := common.HexToAddress("0x55d398326f99059fF775485246999027B3197955")
	from := common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	to := common.HexToAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	c := &ChainAdaptor{}
	transfers := c.parseUserOpBEP20TransfersFromReceipt([]*types.Log{
		transferLog(usdt, from, to, big.NewInt(100), 4),
	})
	if len(transfers) != 0 {
		t.Fatalf("len(transfers) = %d, want 0 without token whitelist: %#v", len(transfers), transfers)
	}
}

func TestTransferUniqueHashUsesEntryTypeAndIndex(t *testing.T) {
	got := transferUniqueHash("0xabc", transferEntryTokenLog, 7)
	want := "0xabc:token_log:7"
	if got != want {
		t.Fatalf("transferUniqueHash() = %q, want %q", got, want)
	}
}

func transferLog(token, from, to common.Address, amount *big.Int, index uint) *types.Log {
	return &types.Log{
		Address: token,
		Topics: []common.Hash{
			transferEventTopic,
			common.BytesToHash(from.Bytes()),
			common.BytesToHash(to.Bytes()),
		},
		Data:  common.LeftPadBytes(amount.Bytes(), 32),
		Index: index,
	}
}
