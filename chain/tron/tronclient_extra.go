package tron

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
)

const (
	defaultActivationTransferSun = int64(100_000)       // 0.1 TRX
	defaultDelegateBalanceSun    = int64(1_000_000_000) // 1000 TRX
	defaultEstimatedEnergy       = int64(150_000)
)

func (client *TronClient) GetAccount(address string) (*Account, error) {
	var response Account
	requestBody := map[string]interface{}{
		"address": address,
		"visible": true,
	}
	resp, err := client.rpc.R().
		SetBody(requestBody).
		SetResult(&response).
		Post("/wallet/getaccount")
	if err != nil {
		return nil, fmt.Errorf("get account failed: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("get account failed status %d: %s", resp.StatusCode(), string(resp.Body()))
	}
	return &response, nil
}

func (client *TronClient) IsAccountActivated(address string) (bool, error) {
	account, err := client.GetAccount(address)
	if err != nil {
		return false, err
	}
	return account != nil && account.CreateTime > 0, nil
}

func (client *TronClient) GetAccountResource(address string) (*AccountResourceResponse, error) {
	var response AccountResourceResponse
	requestBody := map[string]interface{}{
		"address": address,
		"visible": true,
	}
	resp, err := client.rpc.R().
		SetBody(requestBody).
		SetResult(&response).
		Post("/wallet/getaccountresource")
	if err != nil {
		return nil, fmt.Errorf("get account resource failed: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("get account resource failed status %d: %s", resp.StatusCode(), string(resp.Body()))
	}
	return &response, nil
}

func (client *TronClient) AvailableEnergy(address string) (int64, error) {
	resource, err := client.GetAccountResource(address)
	if err != nil {
		return 0, err
	}
	available := resource.EnergyLimit - resource.EnergyUsed
	if available < 0 {
		return 0, nil
	}
	return available, nil
}

type CanDelegatedMaxSizeResponse struct {
	MaxSize int64 `json:"max_size"`
}

func (client *TronClient) GetCanDelegatedMaxSize(ownerAddress string, resourceType int) (int64, error) {
	var response CanDelegatedMaxSizeResponse
	requestBody := map[string]interface{}{
		"owner_address": ownerAddress,
		"type":          resourceType,
		"visible":       true,
	}
	resp, err := client.rpc.R().
		SetBody(requestBody).
		SetResult(&response).
		Post("/wallet/getcandelegatedmaxsize")
	if err != nil {
		return 0, fmt.Errorf("get can delegated max size failed: %w", err)
	}
	if resp.IsError() {
		return 0, fmt.Errorf("get can delegated max size failed status %d: %s", resp.StatusCode(), string(resp.Body()))
	}
	if response.MaxSize < 0 {
		return 0, nil
	}
	return response.MaxSize, nil
}

func broadcastAccepted(result BroadcastReturns) bool {
	if result.Error != "" {
		return false
	}
	if strings.EqualFold(result.Code, "SUCCESS") {
		return true
	}
	return result.Result
}

func (client *TronClient) BroadcastTransaction(signedTx *Transaction) (string, error) {
	payload := broadcastTransactionPayload(signedTx)
	var result BroadcastReturns
	resp, err := client.rpc.R().
		SetBody(payload).
		SetResult(&result).
		Post("/wallet/broadcasttransaction")
	if err != nil {
		return "", fmt.Errorf("broadcast failed: %w", err)
	}
	body := string(resp.Body())
	if resp.IsError() {
		return "", fmt.Errorf("broadcast failed status %d: %s", resp.StatusCode(), body)
	}
	if result.Error != "" {
		return "", fmt.Errorf("broadcast rejected: %s", result.Error)
	}
	if result.Code != "" && !strings.EqualFold(result.Code, "SUCCESS") {
		return "", fmt.Errorf("broadcast rejected: code=%s message=%s", result.Code, decodeBroadcastMessage(result.Message))
	}
	if !broadcastAccepted(result) {
		return "", fmt.Errorf("broadcast not confirmed: code=%q result=%v txid=%q body=%s", result.Code, result.Result, result.Txid, body)
	}
	if result.Txid != "" {
		return result.Txid, nil
	}
	return "", fmt.Errorf("broadcast returned success without txid: body=%s", body)
}

func (client *TronClient) CreateDelegateResourceTransaction(ownerAddress, receiverAddress string, balanceSun int64, resource string) (*Transaction, error) {
	if balanceSun <= 0 {
		balanceSun = defaultDelegateBalanceSun
	}
	if resource == "" {
		resource = "ENERGY"
	}
	requestBody := map[string]interface{}{
		"owner_address":    ownerAddress,
		"receiver_address": receiverAddress,
		"balance":          balanceSun,
		"resource":         resource,
		"lock":             false,
		"visible":          true,
	}
	var response Transaction
	resp, err := client.rpc.R().
		SetBody(requestBody).
		SetResult(&response).
		Post("/wallet/delegateresource")
	if err != nil {
		return nil, fmt.Errorf("delegate resource failed: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("delegate resource failed status %d: %s", resp.StatusCode(), string(resp.Body()))
	}
	return &response, nil
}

func (client *TronClient) GetTRC20Balance(ownerAddress, contractAddress string) (string, error) {
	ownerHex := TronAddressToHex(ownerAddress)
	contractHex := TronAddressToHex(contractAddress)
	if ownerHex == "" || contractHex == "" {
		return "", fmt.Errorf("invalid owner or contract address")
	}
	ownerParam := strings.TrimPrefix(ownerHex, "0x")
	if len(ownerParam) < 64 {
		ownerParam = PadLeftZero(ownerParam[2:], 64)
	} else {
		ownerParam = ownerParam[len(ownerParam)-64:]
	}

	requestBody := map[string]interface{}{
		"owner_address":     ownerAddress,
		"contract_address":  contractAddress,
		"function_selector": "balanceOf(address)",
		"parameter":         ownerParam,
		"visible":           true,
	}
	var response struct {
		Result struct {
			Result  bool   `json:"result"`
			Message string `json:"message"`
		} `json:"result"`
		ConstantResult []string `json:"constant_result"`
	}
	resp, err := client.rpc.R().
		SetBody(requestBody).
		SetResult(&response).
		Post("/wallet/triggerconstantcontract")
	if err != nil {
		return "", fmt.Errorf("triggerconstantcontract failed: %w", err)
	}
	if resp.IsError() {
		return "", fmt.Errorf("triggerconstantcontract failed status %d: %s", resp.StatusCode(), string(resp.Body()))
	}
	if len(response.ConstantResult) == 0 {
		return "0", nil
	}
	raw := response.ConstantResult[0]
	raw = strings.TrimPrefix(raw, "0x")
	if raw == "" {
		return "0", nil
	}
	value := new(big.Int)
	value, ok := value.SetString(raw, 16)
	if !ok {
		return "", fmt.Errorf("decode balance result: invalid hex %s", raw)
	}
	return value.String(), nil
}

func decodeBroadcastMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return message
	}
	decoded, err := hex.DecodeString(message)
	if err != nil {
		return message
	}
	return string(decoded)
}
