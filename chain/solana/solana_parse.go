package solana

import (
	"encoding/binary"
	"encoding/json"
	"math/big"
	"strings"

	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/programs/token"
	"github.com/mr-tron/base58"

	"github.com/dapplink-labs/dapplink-wallet-api/protobuf/walletapi"
)

const (
	txTypeNative uint32 = 1
	txTypeSPL    uint32 = 2

	tokenIxTransfer        byte = 3
	tokenIxTransferChecked byte = 12
	token2022ProgramID          = "TokenzQdBNbLqP5VEhdkAS6EPFLC1PHnBqCXEpPxuEb"
)

// ParsedTransfer is a normalized SOL/SPL transfer extracted from a Solana tx.
type ParsedTransfer struct {
	From            string
	To              string
	Amount          string
	ContractAddress string
	TxType          uint32
	Fee             string
}

type tokenBalanceEntry struct {
	AccountIndex  int    `json:"accountIndex"`
	Mint          string `json:"mint"`
	Owner         string `json:"owner"`
	UITokenAmount struct {
		Amount string `json:"amount"`
	} `json:"uiTokenAmount"`
}

func parseTransactionResult(txResult *TransactionResult) *ParsedTransfer {
	if txResult == nil {
		return nil
	}
	fee := amountString(txResult.Meta.Fee)

	if transfer := parseSPLFromTokenBalances(txResult); transfer != nil {
		transfer.Fee = fee
		return transfer
	}

	accounts := txResult.Transaction.Message.AccountKeys
	for _, instruction := range txResult.Transaction.Message.Instructions {
		if instruction.ProgramIdIndex >= len(accounts) {
			continue
		}
		programID := accounts[instruction.ProgramIdIndex]
		data, err := base58.Decode(instruction.Data)
		if err != nil || len(data) == 0 {
			continue
		}

		if programID == system.ProgramID.String() {
			if len(data) >= 12 && binary.LittleEndian.Uint32(data[0:4]) == 2 {
				lamports := binary.LittleEndian.Uint64(data[4:12])
				fromAddr, toAddr := "", ""
				if len(instruction.Accounts) >= 2 {
					fromAddr = accountAt(accounts, instruction.Accounts, 0)
					toAddr = accountAt(accounts, instruction.Accounts, 1)
				}
				return &ParsedTransfer{
					From:   fromAddr,
					To:     toAddr,
					Amount: amountString(lamports),
					TxType: txTypeNative,
					Fee:    fee,
				}
			}
			continue
		}

		if programID == token.ProgramID.String() || programID == token2022ProgramID {
			if transfer := parseTokenInstruction(accounts, instruction, data); transfer != nil {
				transfer.Fee = fee
				return transfer
			}
		}
	}
	return nil
}

func parseTokenInstruction(accounts []string, instruction Instruction, data []byte) *ParsedTransfer {
	switch data[0] {
	case tokenIxTransfer:
		if len(data) < 9 || len(instruction.Accounts) < 3 {
			return nil
		}
		amount := binary.LittleEndian.Uint64(data[1:9])
		return &ParsedTransfer{
			From:   accountAt(accounts, instruction.Accounts, 2),
			To:     accountAt(accounts, instruction.Accounts, 1),
			Amount: amountString(amount),
			TxType: txTypeSPL,
		}
	case tokenIxTransferChecked:
		if len(data) < 10 || len(instruction.Accounts) < 4 {
			return nil
		}
		amount := binary.LittleEndian.Uint64(data[1:9])
		return &ParsedTransfer{
			From:            accountAt(accounts, instruction.Accounts, 3),
			To:              accountAt(accounts, instruction.Accounts, 2),
			Amount:          amountString(amount),
			ContractAddress: accountAt(accounts, instruction.Accounts, 1),
			TxType:          txTypeSPL,
		}
	default:
		return nil
	}
}

func accountAt(accounts []string, idxs []int, i int) string {
	if i >= len(idxs) || idxs[i] >= len(accounts) {
		return ""
	}
	return accounts[idxs[i]]
}

func parseSPLFromTokenBalances(txResult *TransactionResult) *ParsedTransfer {
	pre := decodeTokenBalances(txResult.Meta.PreTokenBalances)
	post := decodeTokenBalances(txResult.Meta.PostTokenBalances)
	if len(pre) == 0 && len(post) == 0 {
		return nil
	}

	type balKey struct{ owner, mint string }
	preMap := make(map[balKey]*big.Int)
	postMap := make(map[balKey]*big.Int)
	for _, b := range pre {
		preMap[balKey{owner: b.Owner, mint: b.Mint}] = parseBigInt(b.UITokenAmount.Amount)
	}
	for _, b := range post {
		postMap[balKey{owner: b.Owner, mint: b.Mint}] = parseBigInt(b.UITokenAmount.Amount)
	}

	keys := make(map[balKey]struct{})
	for k := range preMap {
		keys[k] = struct{}{}
	}
	for k := range postMap {
		keys[k] = struct{}{}
	}

	var fromOwner, toOwner, mint, amount string
	for k := range keys {
		preAmt := preMap[k]
		if preAmt == nil {
			preAmt = big.NewInt(0)
		}
		postAmt := postMap[k]
		if postAmt == nil {
			postAmt = big.NewInt(0)
		}
		diff := new(big.Int).Sub(postAmt, preAmt)
		switch diff.Sign() {
		case 1:
			toOwner = k.owner
			mint = k.mint
			amount = diff.String()
		case -1:
			fromOwner = k.owner
			if mint == "" {
				mint = k.mint
			}
			neg := new(big.Int).Neg(diff)
			if amount == "" {
				amount = neg.String()
			}
		}
	}
	if fromOwner == "" || toOwner == "" || amount == "" || amount == "0" {
		return nil
	}
	return &ParsedTransfer{
		From:            fromOwner,
		To:              toOwner,
		Amount:          amount,
		ContractAddress: mint,
		TxType:          txTypeSPL,
	}
}

func parseBigInt(s string) *big.Int {
	s = strings.TrimSpace(s)
	if s == "" {
		return big.NewInt(0)
	}
	v := new(big.Int)
	if _, ok := v.SetString(s, 10); !ok {
		return big.NewInt(0)
	}
	return v
}

func decodeTokenBalances(raw []interface{}) []tokenBalanceEntry {
	out := make([]tokenBalanceEntry, 0, len(raw))
	for _, item := range raw {
		b, err := json.Marshal(item)
		if err != nil {
			continue
		}
		var entry tokenBalanceEntry
		if err := json.Unmarshal(b, &entry); err != nil {
			continue
		}
		if entry.Owner != "" && entry.Mint != "" {
			out = append(out, entry)
		}
	}
	return out
}

func solanaTxStatus(metaErr interface{}) uint32 {
	if metaErr != nil {
		return uint32(walletapi.TxStatus_Failed)
	}
	return uint32(walletapi.TxStatus_Success)
}

func toWalletTransaction(hash string, transfer *ParsedTransfer, metaErr interface{}) *walletapi.TransactionList {
	status := solanaTxStatus(metaErr)
	if transfer == nil {
		return &walletapi.TransactionList{TxHash: hash, Status: status}
	}
	kind := "sol_transfer"
	if transfer.TxType == txTypeSPL {
		kind = "spl_transfer"
	}
	return &walletapi.TransactionList{
		TxHash:          hash,
		Fee:             transfer.Fee,
		Status:          status,
		TxType:          transfer.TxType,
		ContractAddress: transfer.ContractAddress,
		TransferKind:    kind,
		From: []*walletapi.FromAddress{
			{Address: transfer.From, Amount: transfer.Amount},
		},
		To: []*walletapi.ToAddress{
			{Address: transfer.To, Amount: transfer.Amount},
		},
	}
}
