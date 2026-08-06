package tron

import (
	"fmt"
)

type triggerSmartContractResponse struct {
	Result struct {
		Result  bool   `json:"result"`
		Message string `json:"message"`
	} `json:"result"`
	Transaction Transaction `json:"transaction"`
}

func (client *TronClient) triggerSmartContract(requestBody map[string]interface{}) (*Transaction, error) {
	var response triggerSmartContractResponse
	resp, err := client.rpc.R().
		SetBody(requestBody).
		SetResult(&response).
		Post("/wallet/triggersmartcontract")
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.IsError() {
		return nil, fmt.Errorf("API request failed with status code: %d, body: %s", resp.StatusCode(), string(resp.Body()))
	}
	if response.Result.Message != "" && !response.Result.Result {
		return nil, fmt.Errorf("triggersmartcontract rejected: %s", response.Result.Message)
	}
	tx := response.Transaction
	if err := validateUnsignedTransaction(&tx); err != nil {
		return nil, err
	}
	return &tx, nil
}

func validateUnsignedTransaction(tx *Transaction) error {
	if tx == nil {
		return fmt.Errorf("empty transaction returned from node")
	}
	if tx.RawDataHex == "" {
		return fmt.Errorf("unsigned transaction missing raw_data_hex")
	}
	if len(tx.RawData.Contract) == 0 {
		return fmt.Errorf("unsigned transaction missing contract")
	}
	return nil
}
