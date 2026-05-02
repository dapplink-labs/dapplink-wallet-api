package tron

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/go-resty/resty/v2"
)

const (
	defaultRequestTimeout = 10 * time.Second
	defaultRetryCount     = 3
)

type TronClient struct {
	rpc *resty.Client
}

func DialTronClient(rpcURL, rpcUser, rpcPass string) *TronClient {
	client := resty.New()
	if rpcUser != "" && rpcPass != "" {
		client.SetHeader("TRON-PRO-API-KEY", rpcPass)
	}
	client.SetBaseURL(rpcURL)
	client.SetTimeout(defaultRequestTimeout)
	client.SetRetryCount(defaultRetryCount)
	return &TronClient{
		rpc: client,
	}
}

func (client *TronClient) GetLatestBlock() (*BlockResponse, error) {
	var response BlockResponse
	err := client.JsonRpcGetLatestBlock(&response)
	if err != nil {
		return nil, fmt.Errorf("failed to get block by number: %v", err)
	}
	log.Info("get latest block fail", "number", response.BlockHeader)
	return &response, nil
}

func (client *TronClient) GetBlockByNumber(blockNumber interface{}) (*BlockResponse, error) {
	var response BlockResponse
	err := client.JsonRpcBlock(blockNumber, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to get block by number: %v", err)
	}
	log.Info("get block by number", "number", response.BlockHeader)
	return &response, nil
}

func (client *TronClient) GetBalance(address string) (*Account, error) {
	params := []interface{}{address}
	var response Account
	err := client.JsonRpcGetBalance(params, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to get block by hash: %v", err)
	}
	return &response, nil
}

func (client *TronClient) GetTransactionByHash(hush string) (*Transaction, error) {
	params := []interface{}{hush}
	var response Transaction
	err := client.JsonRpcGetTransactionByHash(params, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to get block by hash: %v", err)
	}
	return &response, nil
}

func (client *TronClient) JsonRpcGetLatestBlock(result interface{}) error {
	resp, err := client.rpc.R().SetResult(result).Get("/walletsolidity/getblock")
	if err != nil {
		return fmt.Errorf("request failed: %v", err)
	}
	if resp.IsError() {
		return fmt.Errorf("API request failed with status code: %d, body: %s", resp.StatusCode(), string(resp.Body()))
	}
	return nil
}

func (client *TronClient) JsonRpcBlock(params interface{}, result interface{}) error {
	var idOrNum string
	switch v := params.(type) {
	case int64:
		idOrNum = fmt.Sprintf("\"%d\"", v)
	case string:
		idOrNum = fmt.Sprintf("\"%s\"", v)
	default:
		return fmt.Errorf("unsupported params type: %T", params)
	}
	requestBody := map[string]interface{}{
		"id_or_num": json.RawMessage(idOrNum),
		"detail":    true,
	}
	resp, err := client.rpc.R().
		SetBody(requestBody).
		SetResult(result).
		Post("/walletsolidity/getblock")
	if err != nil {
		return fmt.Errorf("request failed: %v", err)
	}
	if resp.IsError() {
		return fmt.Errorf("API request failed with status code: %d, body: %s", resp.StatusCode(), string(resp.Body()))
	}
	return nil
}

func (client *TronClient) JsonRpcBlockHeader(params interface{}, result interface{}) error {
	var idOrNum string
	switch v := params.(type) {
	case int64:
		idOrNum = fmt.Sprintf("\"%d\"", v)
	case string:
		idOrNum = fmt.Sprintf("\"%s\"", v)
	default:
		return fmt.Errorf("unsupported params type: %T", params)
	}
	requestBody := map[string]interface{}{
		"id_or_num": json.RawMessage(idOrNum),
		"detail":    false,
	}
	resp, err := client.rpc.R().
		SetBody(requestBody).
		SetResult(result).
		Get("/wallet/getblock")
	if err != nil {
		return fmt.Errorf("request failed: %v", err)
	}
	if resp.IsError() {
		return fmt.Errorf("API request failed with status code: %d, body: %s", resp.StatusCode(), string(resp.Body()))
	}
	return nil
}

func (client *TronClient) JsonRpcGetBalance(params interface{}, result interface{}) error {
	requestBody := map[string]interface{}{
		"address": params,
		"visible": true,
	}
	resp, err := client.rpc.R().
		SetBody(requestBody).
		SetResult(result).
		Post("/wallet/getaccount")
	if err != nil {
		return fmt.Errorf("request failed: %v", err)
	}
	if resp.IsError() {
		return fmt.Errorf("API request failed with status code: %d, body: %s", resp.StatusCode(), string(resp.Body()))
	}
	return nil
}

func (client *TronClient) JsonRpcGetTransactionByHash(params interface{}, result interface{}) error {
	requestBody := map[string]interface{}{
		"value": params,
	}
	resp, err := client.rpc.R().
		SetBody(requestBody).
		SetResult(result).
		Post("/walletsolidity/gettransactionbyid")
	if err != nil {
		return fmt.Errorf("request failed: %v", err)
	}
	if resp.IsError() {
		return fmt.Errorf("API request failed with status code: %d, body: %s", resp.StatusCode(), string(resp.Body()))
	}
	return nil
}
