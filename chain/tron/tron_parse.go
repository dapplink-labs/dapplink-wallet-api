package tron

import (
	"math/big"
	"strconv"
	"strings"

	"github.com/dapplink-labs/dapplink-wallet-api/protobuf/walletapi"
)

const (
	transferKindTRX         = "outer_trx"
	transferKindTRC20       = "receipt_trc20"
	txKindActivate          = "activate"
	txKindDelegate          = "delegate_resource"
	txKindJustLendRent      = "justlend_rent_energy"
	txKindJustLendReturn    = "justlend_return_energy"
	txKindTRC20             = "trc20_transfer"
)

func parseBlockTransactions(block *BlockResponse) []*walletapi.TransactionList {
	if block == nil || len(block.Transactions) == 0 {
		return nil
	}
	blockHeight := uint64(block.BlockHeader.RawData.Number)
	txList := make([]*walletapi.TransactionList, 0, len(block.Transactions))
	for idx, tx := range block.Transactions {
		parsed := parseSingleTransaction(&tx, block.BlockID, blockHeight, uint32(idx))
		if parsed != nil {
			txList = append(txList, parsed)
		}
	}
	return txList
}

func parseSingleTransaction(tx *Transaction, blockHash string, blockHeight uint64, logIndex uint32) *walletapi.TransactionList {
	if tx == nil || len(tx.RawData.Contract) == 0 {
		return nil
	}
	contract := tx.RawData.Contract[0]
	var (
		fromAddrs       []*walletapi.FromAddress
		toAddrs         []*walletapi.ToAddress
		contractAddress string
		txType          uint32
		transferKind    string
	)
	switch contract.Type {
	case "TransferContract":
		txType = 1
		transferKind = transferKindTRX
		if contract.Parameter.Value.OwnerAddress != "" {
			fromAddr := HexToTronAddress(contract.Parameter.Value.OwnerAddress)
			fromAddrs = append(fromAddrs, &walletapi.FromAddress{
				Address: fromAddr,
				Amount:  strconv.FormatInt(contract.Parameter.Value.Amount, 10),
			})
		}
		if contract.Parameter.Value.ToAddress != "" {
			toAddr := HexToTronAddress(contract.Parameter.Value.ToAddress)
			toAddrs = append(toAddrs, &walletapi.ToAddress{
				Address: toAddr,
				Amount:  strconv.FormatInt(contract.Parameter.Value.Amount, 10),
			})
		}
	case "TriggerSmartContract":
		txType = 2
		transferKind = transferKindTRC20
		if contract.Parameter.Value.ContractAddress != "" {
			contractAddress = HexToTronAddress(contract.Parameter.Value.ContractAddress)
		}
		if contract.Parameter.Value.OwnerAddress != "" {
			fromAddr := HexToTronAddress(contract.Parameter.Value.OwnerAddress)
			fromAddrs = append(fromAddrs, &walletapi.FromAddress{Address: fromAddr})
		}
		data := contract.Parameter.Value.Data
		if len(data) >= 136 && strings.HasPrefix(data, "a9059cbb") {
			toAddrHex := "41" + data[32:72]
			toAddr := HexToTronAddress(toAddrHex)
			amountHex := data[72:136]
			amount := "0"
			if amountBig, ok := new(big.Int).SetString(amountHex, 16); ok {
				amount = amountBig.String()
			}
			if len(fromAddrs) > 0 {
				fromAddrs[0].Amount = amount
			}
			toAddrs = append(toAddrs, &walletapi.ToAddress{
				Address: toAddr,
				Amount:  amount,
			})
		} else {
			return nil
		}
	default:
		return nil
	}
	if len(fromAddrs) == 0 && len(toAddrs) == 0 {
		return nil
	}
	return &walletapi.TransactionList{
		TxHash:          tx.TxID,
		From:            fromAddrs,
		To:              toAddrs,
		ContractAddress: contractAddress,
		TxType:          txType,
		BlockHash:       blockHash,
		BlockHeight:     blockHeight,
		LogIndex:        logIndex,
		TransferKind:    transferKind,
		Status:          contractRetStatus(tx),
	}
}

func contractRetStatus(tx *Transaction) uint32 {
	if tx == nil || len(tx.Ret) == 0 {
		return uint32(walletapi.TxStatus_Pending)
	}
	if strings.EqualFold(tx.Ret[0].ContractRet, "SUCCESS") {
		return uint32(walletapi.TxStatus_Success)
	}
	return uint32(walletapi.TxStatus_Failed)
}

func buildUnsignedTransaction(client *TronClient, data TxStructure) (*Transaction, error) {
	switch strings.ToLower(strings.TrimSpace(data.TxKind)) {
	case txKindActivate:
		amount := data.Value
		if amount <= 0 {
			amount = defaultActivationTransferSun
		}
		return client.CreateTRXTransaction(data.FromAddress, data.ToAddress, amount)
	case txKindDelegate:
		balance := data.DelegateBalance
		if balance <= 0 {
			balance = defaultDelegateBalanceSun
		}
		resource := data.Resource
		if resource == "" {
			resource = "ENERGY"
		}
		return client.CreateDelegateResourceTransaction(data.FromAddress, data.ToAddress, balance, resource)
	case txKindJustLendRent:
		contractAddress := data.ContractAddress
		if contractAddress == "" {
			contractAddress = DefaultJustLendContractMainnet
		}
		return client.CreateJustLendRentTransaction(data.FromAddress, data.ToAddress, contractAddress, data.EnergyAmount, data.RentCallValue)
	case txKindJustLendReturn:
		contractAddress := data.ContractAddress
		if contractAddress == "" {
			contractAddress = DefaultJustLendContractMainnet
		}
		return client.CreateJustLendReturnTransaction(data.FromAddress, data.ToAddress, contractAddress, data.EnergyAmount)
	default:
		if data.ContractAddress == "" {
			return client.CreateTRXTransaction(data.FromAddress, data.ToAddress, data.Value)
		}
		return client.CreateTRC20Transaction(data.FromAddress, data.ToAddress, data.ContractAddress, data.Value)
	}
}
