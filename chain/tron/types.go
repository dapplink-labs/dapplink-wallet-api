package tron

type T struct {
	BlockID     string `json:"blockID"`
	BlockHeader struct {
		RawData struct {
			Number         int    `json:"number"`
			TxTrieRoot     string `json:"txTrieRoot"`
			WitnessAddress string `json:"witness_address"`
			ParentHash     string `json:"parentHash"`
			Version        int    `json:"version"`
			Timestamp      int64  `json:"timestamp"`
		} `json:"raw_data"`
		WitnessSignature string `json:"witness_signature"`
	} `json:"block_header"`
}

type Transactiondata struct {
	BlockHash   string `json:"blockHash"`   // 区块哈希
	BlockNumber string `json:"blockNumber"` // 区块高度(十六进制)

	Hash  string `json:"hash"`  // 交易哈希
	From  string `json:"from"`  // 发送方地址
	To    string `json:"to"`    // 接收方地址
	Value string `json:"value"` // 转账金额(十六进制)

	// Gas 相关
	Gas      string `json:"gas"`      // Gas限制(十六进制)
	GasPrice string `json:"gasPrice"` // Gas价格(十六进制)

	// 交易详情
	Input            string `json:"input"`            // 输入数据
	Nonce            string `json:"nonce"`            // 交易序号
	TransactionIndex string `json:"transactionIndex"` // 交易在区块中的索引(十六进制)
	Type             string `json:"type"`             // 交易类型

	// 签名相关
	V string `json:"v"` // 签名 V 值
	R string `json:"r"` // 签名 R 值
	S string `json:"s"` // 签名 S 值
}
type Block struct {
	BaseFeePerGas    string            `json:"baseFeePerGas"`
	Difficulty       string            `json:"difficulty"`
	ExtraData        string            `json:"extraData"`
	GasLimit         string            `json:"gasLimit"`
	GasUsed          string            `json:"gasUsed"`
	Hash             string            `json:"hash"`
	LogsBloom        string            `json:"logsBloom"`
	Miner            string            `json:"miner"`
	MixHash          string            `json:"mixHash"`
	Nonce            string            `json:"nonce"`
	Number           string            `json:"number"`
	ParentHash       string            `json:"parentHash"`
	ReceiptsRoot     string            `json:"receiptsRoot"`
	Sha3Uncles       string            `json:"sha3Uncles"`
	Size             string            `json:"size"`
	StateRoot        string            `json:"stateRoot"`
	Timestamp        string            `json:"timestamp"`
	TotalDifficulty  string            `json:"totalDifficulty"`
	Transactions     []Transactiondata `json:"transactions"`
	TransactionsRoot string            `json:"transactionsRoot"`
	Uncles           []interface{}     `json:"uncles"`
}

type Account struct {
	Address             string          `json:"address"`
	Balance             int64           `json:"balance"`
	CreateTime          int64           `json:"create_time"`
	LatestConsumeTime   int64           `json:"latest_consume_time"`
	NetWindowSize       int64           `json:"net_window_size"`
	NetWindowOptimized  bool            `json:"net_window_optimized"`
	AccountResource     AccountResource `json:"account_resource"`
	OwnerPermission     Permission      `json:"owner_permission"`
	ActivePermission    []Permission    `json:"active_permission"`
	FrozenV2            []FrozenV2      `json:"frozenV2"`
	AssetV2             []Asset         `json:"assetV2"`
	FreeAssetNetUsageV2 []Asset         `json:"free_asset_net_usageV2"`
	AssetOptimized      bool            `json:"asset_optimized"`
}

type AccountResource struct {
	LatestConsumeTimeForEnergy                int64 `json:"latest_consume_time_for_energy"`
	EnergyWindowSize                          int64 `json:"energy_window_size"`
	AcquiredDelegatedFrozenV2BalanceForEnergy int64 `json:"acquired_delegated_frozenV2_balance_for_energy"`
	EnergyWindowOptimized                     bool  `json:"energy_window_optimized"`
}

type Permission struct {
	Type           string `json:"type,omitempty"`
	ID             int    `json:"id,omitempty"`
	PermissionName string `json:"permission_name"`
	Threshold      int    `json:"threshold"`
	Operations     string `json:"operations,omitempty"`
	Keys           []Key  `json:"keys"`
}

// Key 密钥信息
type Key struct {
	Address string `json:"address"`
	Weight  int    `json:"weight"`
}

// FrozenV2 冻结信息
type FrozenV2 struct {
	Type string `json:"type,omitempty"`
}

// Asset 资产信息
type Asset struct {
	Key   string `json:"key"`
	Value int64  `json:"value"`
}

type Keys struct {
	Address string `json:"address"`
	Weight  int    `json:"weight"`
}

type OwnerPermission struct {
	PermissionName string `json:"permission_name"`
	Threshold      int    `json:"threshold"`
	Keys           []Keys `json:"keys"`
}

type ActivePermission struct {
	Type           string `json:"type"`
	ID             int    `json:"id"`
	PermissionName string `json:"permission_name"`
	Threshold      int    `json:"threshold"`
	Operations     string `json:"operations"`
	Keys           []Keys `json:"keys"`
}

type Ret struct {
	ContractRet string `json:"contractRet"`
}

type Value struct {
	Amount          int    `json:"amount"`
	OwnerAddress    string `json:"owner_address"`
	ToAddress       string `json:"to_address"`
	CallValue       int    `json:"call_value"`
	ContractAddress string `json:"contract_address"`
	Data            string `json:"data"`
}

type TxStructure struct {
	ContractAddress string `json:"contract_address"`
	FromAddress     string `json:"from_address"`
	ToAddress       string `json:"to_address"`
	Value           int64  `json:"value"`
}

type BroadcastReturns struct {
	Code    string `json:"code"`
	Txid    string `json:"txid"`
	Message string `json:"message"`
}

type JSONRPCResponse struct {
	JsonRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Result  string `json:"result"`
	Error   struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}
type Response[T any] struct {
	Jsonrpc string `json:"jsonrpc"`
	Id      int    `json:"id"`
	Result  T      `json:"result"`
}

type ChainParameters struct {
	ChainParameter []struct {
		Key   string `json:"key"`
		Value int64  `json:"value,omitempty"`
	} `json:"chainParameter"`
}

type JSONRPCTrResponse struct {
	JsonRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Result  Transaction `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type BlockResponse struct {
	BlockID      string        `json:"blockID"`
	BlockHeader  BlockHeader   `json:"block_header"`
	Transactions []Transaction `json:"transactions"`
}

type BlockHeader struct {
	RawData          BlockHeaderRaw `json:"raw_data"`
	WitnessSignature string         `json:"witness_signature"`
}

type BlockHeaderRaw struct {
	Number         int64  `json:"number"`
	TxTrieRoot     string `json:"txTrieRoot"`
	WitnessAddress string `json:"witness_address"`
	ParentHash     string `json:"parentHash"`
	Version        int    `json:"version"`
	Timestamp      int64  `json:"timestamp"`
}

type Transaction struct {
	Ret        []TransactionRet `json:"ret"`
	Signature  []string         `json:"signature"`
	TxID       string           `json:"txID"`
	RawData    TxRawData        `json:"raw_data"`
	RawDataHex string           `json:"raw_data_hex"`
}

type TransactionRet struct {
	ContractRet string `json:"contractRet"`
}

type TxRawData struct {
	Contract      []Contract `json:"contract"`
	RefBlockBytes string     `json:"ref_block_bytes"`
	RefBlockHash  string     `json:"ref_block_hash"`
	Expiration    int64      `json:"expiration"`
	FeeLimit      int64      `json:"fee_limit,omitempty"`
	Timestamp     int64      `json:"timestamp"`
}

type Contract struct {
	Parameter    Parameter `json:"parameter"`
	Type         string    `json:"type"`
	PermissionID int       `json:"Permission_id,omitempty"`
}

type Parameter struct {
	Value   ContractValue `json:"value"`
	TypeURL string        `json:"type_url"`
}

type ContractValue struct {
	OwnerAddress    string `json:"owner_address"`
	ToAddress       string `json:"to_address"`
	Amount          int64  `json:"amount"`
	ContractAddress string `json:"contract_address"`
	Data            string `json:"data"`
	AssetName       string `json:"asset_name"`
}
