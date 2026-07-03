package bnbchain

import "testing"

func TestCollectNativeTraceTransfersReturnsPositiveValueCalls(t *testing.T) {
	frame := TraceCallFrame{
		From:  "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		To:    "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Value: "0x56bc75e2d63100000",
		Calls: []TraceCallFrame{
			{
				From:  "0x1111111111111111111111111111111111111111",
				To:    "0x2222222222222222222222222222222222222222",
				Value: "0xde0b6b3a7640000",
			},
			{
				From:  "0x3333333333333333333333333333333333333333",
				To:    "0x4444444444444444444444444444444444444444",
				Value: "0x0",
			},
			{
				From:  "0x5555555555555555555555555555555555555555",
				To:    "0x6666666666666666666666666666666666666666",
				Value: "25",
			},
		},
	}

	transfers := collectNativeTraceTransfers(frame)
	if len(transfers) != 2 {
		t.Fatalf("len(transfers) = %d, want 2", len(transfers))
	}
	if transfers[0].Index != 0 || transfers[0].Amount != "1000000000000000000" {
		t.Fatalf("first native transfer mismatch: %#v", transfers[0])
	}
	if transfers[1].Index != 1 || transfers[1].Amount != "25" {
		t.Fatalf("second native transfer mismatch: %#v", transfers[1])
	}
}
