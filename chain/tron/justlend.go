package tron

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/log"
	base582 "github.com/btcsuite/btcutil/base58"
)

const (
	DefaultJustLendContractMainnet = "TU2MJ5Veik1LRAgjeSzEdvmDYx7mefJZvd"
	justLendResourceTypeEnergy       = 1

	rentResourceMethodSelector   = "fd8527a1"
	returnResourceMethodSelector = "af6f4896"

	defaultPrepaySun65k  int64 = 25_000_000 // 20 TRX fee + rent buffer
	defaultPrepaySun131k int64 = 30_000_000 // 20 TRX fee + rent buffer
	defaultPrepaySun     int64 = 28_000_000
	defaultSmartFeeLimit int64 = 100_000_000
)

func encodeTronAddressABIParam(address string) (string, error) {
	decoded, version, err := base582.CheckDecode(address)
	if err != nil {
		return "", fmt.Errorf("invalid tron address %s: %w", address, err)
	}
	addrBytes := append([]byte{version}, decoded...)
	if len(addrBytes) != 21 {
		return "", fmt.Errorf("invalid tron address length for %s", address)
	}
	return PadLeftZero(hex.EncodeToString(addrBytes[1:]), 64), nil
}

func encodeUint256ABIParam(value int64) string {
	if value < 0 {
		value = 0
	}
	bi := big.NewInt(value)
	return fmt.Sprintf("%064x", bi)
}

func encodeRentResourceParameter(receiver string, trxAmountSun int64) (string, error) {
	receiverParam, err := encodeTronAddressABIParam(receiver)
	if err != nil {
		return "", err
	}
	return receiverParam + encodeUint256ABIParam(trxAmountSun) + encodeUint256ABIParam(justLendResourceTypeEnergy), nil
}

func encodeGetRentInfoParameter(renter, receiver string) (string, error) {
	renterParam, err := encodeTronAddressABIParam(renter)
	if err != nil {
		return "", err
	}
	receiverParam, err := encodeTronAddressABIParam(receiver)
	if err != nil {
		return "", err
	}
	return renterParam + receiverParam + encodeUint256ABIParam(justLendResourceTypeEnergy), nil
}

func defaultRentPrepaySun(energyAmount int64) int64 {
	switch {
	case energyAmount <= 65_000:
		return defaultPrepaySun65k
	case energyAmount <= 131_000:
		return defaultPrepaySun131k
	default:
		return defaultPrepaySun
	}
}

func normalizeJustLendPrepaySun(energyAmount, callValueSun int64) int64 {
	if callValueSun >= minJustLendLiquidationFeeSun {
		return callValueSun
	}
	prepay := defaultRentPrepaySun(energyAmount)
	if prepay < minJustLendLiquidationFeeSun {
		return minJustLendLiquidationFeeSun
	}
	return prepay
}

// EstimateRentPrepay estimates the TRX prepayment in sun for a JustLend energy rental.
func (client *TronClient) EstimateRentPrepay(renter, receiver, contractAddress string, energyAmount int64) (int64, error) {
	if contractAddress == "" {
		contractAddress = DefaultJustLendContractMainnet
	}
	parameter, err := encodeGetRentInfoParameter(renter, receiver)
	if err != nil {
		return defaultRentPrepaySun(energyAmount), nil
	}
	requestBody := map[string]interface{}{
		"owner_address":     renter,
		"contract_address":  contractAddress,
		"function_selector": "getRentInfo(address,address,uint256)",
		"parameter":         parameter,
		"visible":           true,
	}
	var response struct {
		ConstantResult []string `json:"constant_result"`
	}
	resp, err := client.rpc.R().
		SetBody(requestBody).
		SetResult(&response).
		Post("/wallet/triggerconstantcontract")
	if err != nil || resp.IsError() || len(response.ConstantResult) == 0 {
		return defaultRentPrepaySun(energyAmount), nil
	}
	raw := strings.TrimPrefix(response.ConstantResult[0], "0x")
	if len(raw) < 64 {
		return defaultRentPrepaySun(energyAmount), nil
	}
	deposit := new(big.Int)
	if _, ok := deposit.SetString(raw[:64], 16); !ok || deposit.Sign() <= 0 {
		return defaultRentPrepaySun(energyAmount), nil
	}
	if !deposit.IsInt64() {
		return defaultRentPrepaySun(energyAmount), nil
	}
	return deposit.Int64(), nil
}

// CreateJustLendRentTransaction builds an unsigned JustLend rentResource transaction.
func (client *TronClient) CreateJustLendRentTransaction(fromAddress, receiverAddress, contractAddress string, energyAmount, callValueSun int64) (*Transaction, error) {
	if contractAddress == "" {
		contractAddress = DefaultJustLendContractMainnet
	}
	if energyAmount <= 0 {
		return nil, fmt.Errorf("invalid energy amount: %d", energyAmount)
	}
	activated, err := client.IsAccountActivated(receiverAddress)
	if err != nil {
		return nil, fmt.Errorf("check receiver activation: %w", err)
	}
	if !activated {
		return nil, fmt.Errorf("receiver account is not activated: %s", receiverAddress)
	}

	trxAmountSun, err := EnergyToDelegatedSun(energyAmount)
	if err != nil {
		return nil, err
	}

	if callValueSun <= 0 {
		estimated, err := client.EstimateRentPrepay(fromAddress, receiverAddress, contractAddress, energyAmount)
		if err != nil || estimated <= 0 {
			callValueSun = defaultRentPrepaySun(energyAmount)
		} else {
			callValueSun = estimated
		}
	}
	callValueSun = normalizeJustLendPrepaySun(energyAmount, callValueSun)

	log.Info("justlend rentResource params",
		"from", fromAddress,
		"receiver", receiverAddress,
		"energy", energyAmount,
		"delegatedSun", trxAmountSun,
		"callValueSun", callValueSun,
	)

	parameter, err := encodeRentResourceParameter(receiverAddress, trxAmountSun)
	if err != nil {
		return nil, err
	}
	requestBody := map[string]interface{}{
		"owner_address":     fromAddress,
		"contract_address":  contractAddress,
		"function_selector": "rentResource(address,uint256,uint256)",
		"parameter":         parameter,
		"fee_limit":         defaultSmartFeeLimit,
		"call_value":        callValueSun,
		"visible":           true,
	}
	return client.triggerSmartContract(requestBody)
}

func buildJustLendRentCalldata(receiver string, trxAmountSun int64) (string, error) {
	parameter, err := encodeRentResourceParameter(receiver, trxAmountSun)
	if err != nil {
		return "", err
	}
	return rentResourceMethodSelector + parameter, nil
}

// QueryJustLendRentalDelegatedSun returns active delegated TRX (sun) for a rental order, or 0 if none.
func (client *TronClient) QueryJustLendRentalDelegatedSun(renter, receiver, contractAddress string) (int64, error) {
	if contractAddress == "" {
		contractAddress = DefaultJustLendContractMainnet
	}
	parameter, err := encodeGetRentInfoParameter(renter, receiver)
	if err != nil {
		return 0, err
	}
	requestBody := map[string]interface{}{
		"owner_address":     renter,
		"contract_address":  contractAddress,
		"function_selector": "rentals(address,address,uint256)",
		"parameter":         parameter,
		"visible":           true,
	}
	var response struct {
		ConstantResult []string `json:"constant_result"`
	}
	resp, err := client.rpc.R().
		SetBody(requestBody).
		SetResult(&response).
		Post("/wallet/triggerconstantcontract")
	if err != nil || resp.IsError() || len(response.ConstantResult) == 0 {
		return 0, err
	}
	raw := strings.TrimPrefix(response.ConstantResult[0], "0x")
	if len(raw) < 64 {
		return 0, nil
	}
	amount := new(big.Int)
	if _, ok := amount.SetString(raw[:64], 16); !ok || amount.Sign() <= 0 {
		return 0, nil
	}
	if !amount.IsInt64() {
		return 0, fmt.Errorf("delegated sun overflows int64")
	}
	return amount.Int64(), nil
}

// CreateJustLendReturnTransaction builds an unsigned JustLend returnResource transaction.
func (client *TronClient) CreateJustLendReturnTransaction(fromAddress, receiverAddress, contractAddress string, energyAmount int64) (*Transaction, error) {
	if contractAddress == "" {
		contractAddress = DefaultJustLendContractMainnet
	}
	if energyAmount <= 0 {
		return nil, fmt.Errorf("invalid energy amount: %d", energyAmount)
	}

	delegatedSun, err := client.QueryJustLendRentalDelegatedSun(fromAddress, receiverAddress, contractAddress)
	if err != nil {
		log.Warn("justlend rentals query failed, fallback to estimated delegated sun", "err", err, "receiver", receiverAddress)
		trxAmountSun, convErr := EnergyToDelegatedSun(energyAmount)
		if convErr != nil {
			return nil, convErr
		}
		delegatedSun = trxAmountSun
	} else if delegatedSun <= 0 {
		return nil, fmt.Errorf("no active justlend rental for receiver %s", receiverAddress)
	}

	log.Info("justlend returnResource params",
		"from", fromAddress,
		"receiver", receiverAddress,
		"energy", energyAmount,
		"delegatedSun", delegatedSun,
	)

	parameter, err := encodeRentResourceParameter(receiverAddress, delegatedSun)
	if err != nil {
		return nil, err
	}
	requestBody := map[string]interface{}{
		"owner_address":     fromAddress,
		"contract_address":  contractAddress,
		"function_selector": "returnResource(address,uint256,uint256)",
		"parameter":         parameter,
		"fee_limit":         defaultSmartFeeLimit,
		"call_value":        0,
		"visible":           true,
	}
	return client.triggerSmartContract(requestBody)
}

func buildJustLendReturnCalldata(receiver string, trxAmountSun int64) (string, error) {
	parameter, err := encodeRentResourceParameter(receiver, trxAmountSun)
	if err != nil {
		return "", err
	}
	return returnResourceMethodSelector + parameter, nil
}
