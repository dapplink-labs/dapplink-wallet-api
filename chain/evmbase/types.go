package evmbase

func IsEthTransfer(tx *Eip1559DynamicFeeTx) bool {
	if tx.ContractAddress == "" || tx.ContractAddress == NativeToken {
		return true
	}
	return false
}

type Eip1559DynamicFeeTx struct {
	ChainId         string `json:"chain_id"`
	FromAddress     string `json:"from_address"`
	ToAddress       string `json:"to_address"`
	Amount          string `json:"amount"`
	ContractAddress string `json:"contract_address"`
	Signature       string `json:"signature,omitempty"`
}
