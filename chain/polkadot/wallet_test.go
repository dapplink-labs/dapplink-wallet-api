package polkadot

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
	"github.com/cosmos/go-bip39"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/dapplink-labs/dapplink-wallet-api/chain/evmbase"
	"github.com/dapplink-labs/dapplink-wallet-api/chain/substratebase"
	"github.com/dapplink-labs/dapplink-wallet-api/config"
	wallet_api "github.com/dapplink-labs/dapplink-wallet-api/protobuf/wallet-api"
)

// TestGenerateMnemonicAndAddresses 测试生成助记词和地址
func TestGenerateMnemonicAndAddresses(t *testing.T) {
	entropy, err := bip39.NewEntropy(128)
	if err != nil {
		t.Fatalf("generate entropy failed: %v", err)
	}

	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		t.Fatalf("generate mnemonic failed: %v", err)
	}

	t.Logf("Mnemonic: %s", mnemonic)
	printAddressesFromMnemonic(t, mnemonic)
}

// TestAddressesFromExistingMnemonic 测试从现有助记词生成地址
func TestAddressesFromExistingMnemonic(t *testing.T) {
	mnemonic := ""
	t.Logf("Mnemonic: %s", mnemonic)
	printAddressesFromMnemonic(t, mnemonic)

	// EVM Address:        0xa65E2a906C3911C85B77BB1023903c4bacd95B4E
	// Polkadot Address:   1Keye2NMnuNqCfncACTLEu7LkDkrv633aRPd7XEo8axmnts
	// Kusama Address:     CtyVd7B8Neq9KUiRDxW63RxdiWLyHM5RTXerUoqiqmwLUvd
	// 测试地址
	// Mnemonic: welcome long strategy cable siege mirror warrior nothing anxiety initial urban music
	// EVM Address:   0xa65E2a906C3911C85B77BB1023903c4bacd95B4E
	// EVM PublicKey: 04bcdef4a02c6235a0c04f4e9b0810003e91d490cb48c2bef9365226b69d19547b1eaac8bd5a052b2e28b28b8c3b33067e595373815e101c89945c895c381f7d79
	// Polkadot SS58 Address:  1Keye2NMnuNqCfncACTLEu7LkDkrv633aRPd7XEo8axmnts
	// Polkadot PublicKey:     0x0e3a41f569d07bc820b7f89f8f25dd7fb0a9bb0bd2eaa3b3c98e675ddb6e7c5b
	// Kusama SS58 Address:    CtyVd7B8Neq9KUiRDxW63RxdiWLyHM5RTXerUoqiqmwLUvd
	// Kusama PublicKey:       0x0e3a41f569d07bc820b7f89f8f25dd7fb0a9bb0bd2eaa3b3c98e675ddb6e7c5b
}

func printAddressesFromMnemonic(t *testing.T, mnemonic string) {
	seed := bip39.NewSeed(mnemonic, "")

	evmAddr, evmPubKey := deriveEvmAddress(t, seed)
	t.Logf("EVM Address:   %s", evmAddr)
	t.Logf("EVM PublicKey: %s", evmPubKey)

	polkadotAddr, polkadotPubKey := deriveSubstrateAddress(t, mnemonic, uint16(substratebase.PolkadotSS58Prefix))
	t.Logf("Polkadot SS58 Address:  %s", polkadotAddr)
	t.Logf("Polkadot PublicKey:     %s", polkadotPubKey)

	kusamaAddr, kusamaPubKey := deriveSubstrateAddress(t, mnemonic, uint16(substratebase.KusamaSS58Prefix))
	t.Logf("Kusama SS58 Address:    %s", kusamaAddr)
	t.Logf("Kusama PublicKey:       %s", kusamaPubKey)

	fmt.Println()
	fmt.Printf("Mnemonic:           %s\n", mnemonic)
	fmt.Printf("EVM Address:        %s\n", evmAddr)
	fmt.Printf("Polkadot Address:   %s\n", polkadotAddr)
	fmt.Printf("Kusama Address:     %s\n", kusamaAddr)
}

func deriveEvmAddress(t *testing.T, seed []byte) (string, string) {
	privKey := deriveEvmPrivateKey(t, seed)
	publicKey := privKey.PublicKey
	publicKeyBytes := crypto.FromECDSAPub(&publicKey)
	address := crypto.PubkeyToAddress(publicKey).Hex()

	return address, "0x" + hex.EncodeToString(publicKeyBytes)
}

func deriveEvmPrivateKey(t *testing.T, seed []byte) *ecdsa.PrivateKey {
	masterKey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("create BIP32 master key failed: %v", err)
	}

	purpose, err := masterKey.Derive(hdkeychain.HardenedKeyStart + 44)
	if err != nil {
		t.Fatalf("derive purpose failed: %v", err)
	}
	coinType, err := purpose.Derive(hdkeychain.HardenedKeyStart + 60)
	if err != nil {
		t.Fatalf("derive coin type failed: %v", err)
	}
	account, err := coinType.Derive(hdkeychain.HardenedKeyStart + 0)
	if err != nil {
		t.Fatalf("derive account failed: %v", err)
	}
	change, err := account.Derive(0)
	if err != nil {
		t.Fatalf("derive change failed: %v", err)
	}
	addrIndex, err := change.Derive(0)
	if err != nil {
		t.Fatalf("derive address index failed: %v", err)
	}

	privKey, err := addrIndex.ECPrivKey()
	if err != nil {
		t.Fatalf("get ECDSA private key failed: %v", err)
	}

	ecdsaPrivKey, err := crypto.ToECDSA(privKey.Serialize())
	if err != nil {
		t.Fatalf("convert ECDSA private key failed: %v", err)
	}

	return ecdsaPrivKey
}

func deriveSubstrateAddress(t *testing.T, mnemonic string, ss58Prefix uint16) (string, string) {
	keyringPair := deriveSubstrateKeyringPair(t, mnemonic, ss58Prefix)
	pubKeyHex := "0x" + hex.EncodeToString(keyringPair.PublicKey)
	return keyringPair.Address, pubKeyHex
}

func deriveSubstrateKeyringPair(t *testing.T, mnemonic string, ss58Prefix uint16) signature.KeyringPair {
	keyringPair, err := signature.KeyringPairFromSecret(mnemonic, ss58Prefix)
	if err != nil {
		t.Fatalf("create keyring pair from mnemonic failed: %v", err)
	}

	return keyringPair
}

// TestConvertAddresses 测试ConvertAddresses方法
func TestConvertAddresses(t *testing.T) {
	// 测试数据
	evmPublicKey := "0x04bcdef4a02c6235a0c04f4e9b0810003e91d490cb48c2bef9365226b69d19547b1eaac8bd5a052b2e28b28b8c3b33067e595373815e101c89945c895c381f7d79"
	expectedEvmAddress := "0xa65E2a906C3911C85B77BB1023903c4bacd95B4E"
	substratePublicKey := "0x0e3a41f569d07bc820b7f89f8f25dd7fb0a9bb0bd2eaa3b3c98e675ddb6e7c5b"
	kusamaPublicKey := "0x0e3a41f569d07bc820b7f89f8f25dd7fb0a9bb0bd2eaa3b3c98e675ddb6e7c5b"
	expectedPolkadotAddress := "1Keye2NMnuNqCfncACTLEu7LkDkrv633aRPd7XEo8axmnts"
	expectedKusamaAddress := "CtyVd7B8Neq9KUiRDxW63RxdiWLyHM5RTXerUoqiqmwLUvd"

	// 从公钥字节生成预期的Polkadot和Kusama地址
	ctx := context.Background()

	// 测试EVM地址转换
	t.Run("EVM", func(t *testing.T) {
		evmAdaptor := &EvmChainAdaptor{}

		// 构建请求
		req := &wallet_api.ConvertAddressesRequest{
			PublicKey: []*wallet_api.PublicKey{
				{
					PublicKey: evmPublicKey,
				},
			},
		}

		// 调用方法
		resp, err := evmAdaptor.ConvertAddresses(ctx, req)
		if err != nil {
			t.Fatalf("EVM ConvertAddresses failed: %v", err)
		}

		// 验证结果
		if len(resp.Address) != 1 {
			t.Fatalf("EVM ConvertAddresses returned %d addresses, expected 1", len(resp.Address))
		}

		if resp.Address[0].Address != expectedEvmAddress {
			t.Errorf("EVM address mismatch: got %s, expected %s", resp.Address[0].Address, expectedEvmAddress)
		} else {
			t.Logf("EVM ConvertAddresses test passed: %s", resp.Address[0].Address)
		}
	})

	// 测试Polkadot地址转换
	t.Run("Polkadot", func(t *testing.T) {
		// 创建Substrate适配器实例
		polkadotAdaptor := &SubstrateChainAdaptor{
			ss58Prefix: uint8(substratebase.PolkadotSS58Prefix),
		}

		// 构建请求
		req := &wallet_api.ConvertAddressesRequest{
			PublicKey: []*wallet_api.PublicKey{
				{
					PublicKey: substratePublicKey,
				},
			},
		}

		// 调用方法
		resp, err := polkadotAdaptor.ConvertAddresses(ctx, req)
		if err != nil {
			t.Fatalf("Polkadot ConvertAddresses failed: %v", err)
		}

		// 验证结果
		if len(resp.Address) != 1 {
			t.Fatalf("Polkadot ConvertAddresses returned %d addresses, expected 1", len(resp.Address))
		}

		if resp.Address[0].Address != expectedPolkadotAddress {
			t.Errorf("Polkadot address mismatch: got %s, expected %s", resp.Address[0].Address, expectedPolkadotAddress)
		} else {
			t.Logf("Polkadot ConvertAddresses test passed: %s", resp.Address[0].Address)
		}
	})

	// 测试Kusama地址转换
	t.Run("Kusama", func(t *testing.T) {
		// 创建Substrate适配器实例
		kusamaAdaptor := &SubstrateChainAdaptor{
			ss58Prefix: uint8(substratebase.KusamaSS58Prefix),
		}

		// 构建请求
		req := &wallet_api.ConvertAddressesRequest{
			PublicKey: []*wallet_api.PublicKey{
				{
					PublicKey: kusamaPublicKey,
				},
			},
		}

		// 调用方法
		resp, err := kusamaAdaptor.ConvertAddresses(ctx, req)
		if err != nil {
			t.Fatalf("Kusama ConvertAddresses failed: %v", err)
		}

		// 验证结果
		if len(resp.Address) != 1 {
			t.Fatalf("Kusama ConvertAddresses returned %d addresses, expected 1", len(resp.Address))
		}

		if resp.Address[0].Address != expectedKusamaAddress {
			t.Errorf("Kusama address mismatch: got %s, expected %s", resp.Address[0].Address, expectedKusamaAddress)
		} else {
			t.Logf("Kusama ConvertAddresses test passed: %s", resp.Address[0].Address)
		}
	})
}

// TestValidAddresses 测试ValidAddresses方法
func TestValidAddresses(t *testing.T) {
	// 测试数据
	validEvmAddress := "0xa65E2a906C3911C85B77BB1023903c4bacd95B4E"
	validSubstrateAddress := "1Keye2NMnuNqCfncACTLEu7LkDkrv633aRPd7XEo8axmnts"
	validKusamaAddress := "CtyVd7B8Neq9KUiRDxW63RxdiWLyHM5RTXerUoqiqmwLUvd"

	ctx := context.Background()
	// 验证evm地址是否有效
	t.Run("EVM", func(t *testing.T) {
		// 创建EVM适配器实例
		evmAdaptor := &EvmChainAdaptor{}

		// 构建请求
		req := &wallet_api.ValidAddressesRequest{
			Addresses: []*wallet_api.Addresses{
				{Address: validEvmAddress},
			},
		}

		// 调用方法
		resp, err := evmAdaptor.ValidAddresses(ctx, req)
		if err != nil {
			t.Fatalf("EVM 有效地址失败: %v", err)
		}

		// 验证结果
		if len(resp.AddressValid) != 1 {
			t.Fatalf("EVM ValidAddresses 返回 %d 个地址，预期为 1", len(resp.AddressValid))
		}

		if !resp.AddressValid[0].Valid {
			t.Errorf("EVM 地址 %s 无效", resp.AddressValid[0].Address)
		} else {
			t.Logf("EVM 地址 %s 有效", resp.AddressValid[0].Address)
		}
	})

	// 验证Polkadot地址是否有效
	t.Run("Polkadot", func(t *testing.T) {
		// 创建Substrate适配器实例
		polkadotAdaptor := &SubstrateChainAdaptor{
			ss58Prefix: uint8(substratebase.PolkadotSS58Prefix),
		}

		// 构建请求
		req := &wallet_api.ValidAddressesRequest{
			Addresses: []*wallet_api.Addresses{
				{Address: validSubstrateAddress},
			},
		}

		// 调用方法
		resp, err := polkadotAdaptor.ValidAddresses(ctx, req)
		if err != nil {
			t.Fatalf("Polkadot 有效地址失败: %v", err)
		}

		// 验证结果
		if len(resp.AddressValid) != 1 {
			t.Fatalf("Polkadot ValidAddresses 返回 %d 个地址，预期为 1", len(resp.AddressValid))
		}

		if !resp.AddressValid[0].Valid {
			t.Errorf("Polkadot 地址 %s 无效", resp.AddressValid[0].Address)
		} else {
			t.Logf("Polkadot 地址 %s 有效", resp.AddressValid[0].Address)
		}
	})

	// 验证Kusama地址是否有效
	t.Run("Kusama", func(t *testing.T) {
		// 创建Substrate适配器实例
		kusamaAdaptor := &SubstrateChainAdaptor{
			ss58Prefix: uint8(substratebase.KusamaSS58Prefix),
		}

		// 构建请求
		req := &wallet_api.ValidAddressesRequest{
			Addresses: []*wallet_api.Addresses{
				{Address: validKusamaAddress},
			},
		}

		// 调用方法
		resp, err := kusamaAdaptor.ValidAddresses(ctx, req)
		if err != nil {
			t.Fatalf("Kusama 有效地址失败: %v", err)
		}

		// 验证结果
		if len(resp.AddressValid) != 1 {
			t.Fatalf("Kusama ValidAddresses 返回 %d 个地址，预期为 1", len(resp.AddressValid))
		}

		if !resp.AddressValid[0].Valid {
			t.Errorf("Kusama 地址 %s 无效", resp.AddressValid[0].Address)
		} else {
			t.Logf("Kusama 地址 %s 有效", resp.AddressValid[0].Address)
		}
	})

}

// TestGetLastestBlock 测试获取最新区块高度和哈希
func TestGetLastestBlock(t *testing.T) {

	// 构建请求
	req := &wallet_api.LastestBlockRequest{}

	ctx := context.Background()
	conf, err := config.NewConfig("../../config.yml")
	if err != nil {
		t.Fatalf("加载配置文件失败: %v", err)
	}

	// EVM方式
	t.Run("EVM", func(t *testing.T) {

		ethClient, err := evmbase.DialEthClient(context.Background(), conf.WalletNode.Dot.RpcUrl)
		if err != nil {
			t.Fatalf("连接EVM客户端失败: %v", err)
		}

		ethDataClient, err := evmbase.NewEthDataClient(conf.WalletNode.Dot.DataApiUrl, conf.WalletNode.Dot.DataApiKey, time.Second*15)
		if err != nil {
			t.Fatalf("创建EVM数据客户端失败: %v", err)
		}

		evmAdaptor := &EvmChainAdaptor{
			ethClient:     ethClient,
			ethDataClient: ethDataClient,
		}

		// 调用方法
		resp, err := evmAdaptor.GetLastestBlock(ctx, req)
		if err != nil {
			t.Fatalf("获取最新区块高度和哈希失败: %v", err)
		}

		// 验证结果
		if resp.Height == 0 {
			t.Errorf("最新区块高度为 0")
		} else {
			fmt.Printf("最新区块高度: %d\n", resp.Height)
		}

		if resp.Hash == "" {
			t.Errorf("最新区块哈希为空")
		} else {
			fmt.Printf("最新区块哈希: %s\n", resp.Hash)
		}
	})

	// Polkadot方法
	t.Run("Polkadot", func(t *testing.T) {
		cli, err := substratebase.NewSubstrateClient(conf.WalletNode.Dot.SubstrateRpcUrl)
		if err != nil {
			t.Fatalf("连接Substrate客户端失败: %v", err)
		}
		polkadotAdaptor := &SubstrateChainAdaptor{
			substrateClient: cli,
			ss58Prefix:      uint8(substratebase.PolkadotSS58Prefix),
		}
		// 调用方法
		resp, err := polkadotAdaptor.GetLastestBlock(ctx, req)
		if err != nil {
			t.Fatalf("获取最新区块高度和哈希失败: %v", err)
		}

		// 验证结果
		if resp.Height == 0 {
			t.Errorf("最新区块高度为 0")
		} else {
			fmt.Printf("最新区块高度: %d\n", resp.Height)
		}

		if resp.Hash == "" {
			t.Errorf("最新区块哈希为空")
		} else {
			fmt.Printf("最新区块哈希: %s\n", resp.Hash)
		}
	})

}

// TestGetBlock 测试获取指定区块的交易信息
func TestGetBlock(t *testing.T) {
	var height string = "14635363"

	ctx := context.Background()
	conf, err := config.NewConfig("../../config.yml")
	if err != nil {
		t.Fatalf("加载配置文件失败: %v", err)
	}

	// EVM方式
	t.Run("EVM", func(t *testing.T) {

		ethClient, err := evmbase.DialEthClient(context.Background(), conf.WalletNode.Dot.RpcUrl)
		if err != nil {
			t.Fatalf("连接EVM客户端失败: %v", err)
		}

		ethDataClient, err := evmbase.NewEthDataClient(conf.WalletNode.Dot.DataApiUrl, conf.WalletNode.Dot.DataApiKey, time.Second*15)
		if err != nil {
			t.Fatalf("创建EVM数据客户端失败: %v", err)
		}

		evmAdaptor := &EvmChainAdaptor{
			ethClient:     ethClient,
			ethDataClient: ethDataClient,
		}

		req := &wallet_api.BlockRequest{
			HashHeight:  height,
			IsBlockHash: false,
		}

		resp, err := evmAdaptor.GetBlock(ctx, req)
		if err != nil {
			t.Fatalf("获取指定区块高度和哈希失败: %v", err)
		}

		fmt.Printf("区块高度: %s\n", resp.Height)
		fmt.Printf("区块哈希: %s\n", resp.Hash)
		fmt.Printf("交易数量: %d\n", len(resp.Transactions))
		for i, tx := range resp.Transactions {
			fmt.Printf("  交易[%d]: TxHash=%s, Fee=%s, Status=%d, TxType=%d, ContractAddress=%s\n", i, tx.TxHash, tx.Fee, tx.Status, tx.TxType, tx.ContractAddress)
			for _, from := range tx.From {
				fmt.Printf("    From: %s, Amount: %s\n", from.Address, from.Amount)
				if from.MetaData != "" {
					fmt.Printf("      MetaData: %s\n", from.MetaData)
				}
			}
			for _, to := range tx.To {
				fmt.Printf("    To: %s, Amount: %s\n", to.Address, to.Amount)
				if to.MetaData != "" {
					fmt.Printf("      MetaData: %s\n", to.MetaData)
				}
			}
		}
	})

	// Polkadot方式
	t.Run("Polkadot", func(t *testing.T) {

		cli, err := substratebase.NewSubstrateClient(conf.WalletNode.Dot.SubstrateRpcUrl)
		if err != nil {
			t.Fatalf("连接Substrate客户端失败: %v", err)
		}
		polkadotAdaptor := &SubstrateChainAdaptor{
			substrateClient: cli,
			ss58Prefix:      uint8(substratebase.PolkadotSS58Prefix),
		}

		req := &wallet_api.BlockRequest{
			HashHeight:  height,
			IsBlockHash: false,
		}

		resp, err := polkadotAdaptor.GetBlock(ctx, req)
		if err != nil {
			t.Fatalf("获取指定区块高度和哈希失败: %v", err)
		}

		fmt.Printf("区块高度: %s\n", resp.Height)
		fmt.Printf("区块哈希: %s\n", resp.Hash)
		fmt.Printf("交易数量: %d\n", len(resp.Transactions))
		for i, tx := range resp.Transactions {
			fmt.Printf("  交易[%d]: TxHash=%s, Fee=%s, Status=%d, TxType=%d, ContractAddress=%s\n", i, tx.TxHash, tx.Fee, tx.Status, tx.TxType, tx.ContractAddress)
			for _, from := range tx.From {
				fmt.Printf("    From: %s, Amount: %s\n", from.Address, from.Amount)
				if from.MetaData != "" {
					fmt.Printf("      MetaData: %s\n", from.MetaData)
				}
			}
			for _, to := range tx.To {
				fmt.Printf("    To: %s\n", to.Address)
				if to.MetaData != "" {
					fmt.Printf("      MetaData: %s\n", to.MetaData)
				}
			}
		}
	})
}

// TestGetTransactionByHash 测试根据交易哈希获取交易详情
func TestGetTransactionByHash(t *testing.T) {

	ctx := context.Background()
	conf, err := config.NewConfig("../../config.yml")
	if err != nil {
		t.Fatalf("加载配置文件失败: %v", err)
	}

	// EVM方式
	t.Run("EVM", func(t *testing.T) {
		var txHash string = "0xd580cf0ba1ecaa6cb81b43ac43b0cb140677c4ba306c55053784a89410012486"
		ethClient, err := evmbase.DialEthClient(context.Background(), conf.WalletNode.Dot.RpcUrl)
		if err != nil {
			t.Fatalf("连接EVM客户端失败: %v", err)
		}

		ethDataClient, err := evmbase.NewEthDataClient(conf.WalletNode.Dot.DataApiUrl, conf.WalletNode.Dot.DataApiKey, time.Second*15)
		if err != nil {
			t.Fatalf("创建EVM数据客户端失败: %v", err)
		}

		evmAdaptor := &EvmChainAdaptor{
			ethClient:     ethClient,
			ethDataClient: ethDataClient,
		}

		req := &wallet_api.TransactionByHashRequest{
			Hash: txHash,
		}

		resp, err := evmAdaptor.GetTransactionByHash(ctx, req)
		if err != nil {
			t.Fatalf("获取指定区块高度和哈希失败: %v", err)
		}
		if resp.Transaction == nil {
			t.Fatalf("交易详情为空: code=%d, msg=%s", resp.Code, resp.Msg)
		}

		fmt.Printf("交易哈希: %s\n", resp.Transaction.TxHash)
		fmt.Printf("交易状态: %d\n", resp.Transaction.Status)
		fmt.Printf("交易类型: %d\n", resp.Transaction.TxType)
		fmt.Printf("合约地址：%s\n", resp.Transaction.ContractAddress)
		fmt.Printf("fee: %s\n", resp.Transaction.Fee)
		for _, from := range resp.Transaction.From {
			fmt.Printf("    From: %s\n", from.Address)
		}
		for _, to := range resp.Transaction.To {
			fmt.Printf("    To: %s\n", to.Address)
		}
	})

	// Polkadot方式
	t.Run("Polkadot", func(t *testing.T) {

		cli, err := substratebase.NewSubstrateClient(conf.WalletNode.Dot.SubstrateRpcUrl)
		if err != nil {
			t.Fatalf("连接Substrate客户端失败: %v", err)
		}
		polkadotAdaptor := &SubstrateChainAdaptor{
			substrateClient: cli,
			ss58Prefix:      uint8(substratebase.PolkadotSS58Prefix),
		}

		var txHash string = "0xb49c0811685fc635500eb38f7321d95809cad4ba92ac968bfbdc34dac8e7adfd"
		req := &wallet_api.TransactionByHashRequest{
			Hash: txHash,
		}

		resp, err := polkadotAdaptor.GetTransactionByHash(ctx, req)
		if err != nil {
			t.Fatalf("获取指定区块高度和哈希失败: %v", err)
		}
		if resp.Transaction == nil {
			t.Fatalf("交易详情为空: code=%d, msg=%s", resp.Code, resp.Msg)
		}

		fmt.Printf("交易哈希: %s\n", resp.Transaction.TxHash)
		fmt.Printf("交易状态: %d\n", resp.Transaction.Status)
		fmt.Printf("交易类型: %d\n", resp.Transaction.TxType)
		fmt.Printf("合约地址：%s\n", resp.Transaction.ContractAddress)
		fmt.Printf("fee: %s\n", resp.Transaction.Fee)
		for _, from := range resp.Transaction.From {
			fmt.Printf("    From: %s, Amount: %s\n", from.Address, from.Amount)
			if from.MetaData != "" {
				fmt.Printf("      MetaData: %s\n", from.MetaData)
			}
		}
		for _, to := range resp.Transaction.To {
			fmt.Printf("    To: %s\n", to.Address)
			if to.MetaData != "" {
				fmt.Printf("      MetaData: %s\n", to.MetaData)
			}
		}

	})

}

// GetTransactionByAddress 测试根据地址获取交易详情
func TestGetTransactionByAddress(t *testing.T) {

	ctx := context.Background()
	conf, err := config.NewConfig("../../config.yml")
	if err != nil {
		t.Fatalf("加载配置文件失败: %v", err)
	}

	// EVM方式
	t.Run("EVM", func(t *testing.T) {
		var address string = "0xDA3792fF5D99fced3EA1e383339F0E4006E379ed"

		ethClient, err := evmbase.DialEthClient(context.Background(), conf.WalletNode.Dot.RpcUrl)
		if err != nil {
			t.Fatalf("连接EVM客户端失败: %v", err)
		}

		ethDataClient, err := evmbase.NewEthDataClient(conf.WalletNode.Dot.DataApiUrl, conf.WalletNode.Dot.DataApiKey, time.Second*15)
		if err != nil {
			t.Fatalf("创建EVM数据客户端失败: %v", err)
		}
		evmAdaptor := &EvmChainAdaptor{
			ethClient:     ethClient,
			ethDataClient: ethDataClient,
		}

		req := &wallet_api.TransactionByAddressRequest{
			Address: address,
		}

		resp, err := evmAdaptor.GetTransactionByAddress(ctx, req)
		if err != nil {
			t.Fatalf("根据地址获取交易详情失败: %v", err)
		}
		if resp.Transaction == nil {
			t.Fatalf("交易详情为空: code=%d, msg=%s", resp.Code, resp.Msg)
		}

		fmt.Printf("交易数量: %d\n", len(resp.Transaction))
		for i, tx := range resp.Transaction {
			fmt.Printf("  交易[%d]: TxHash=%s, Fee=%s, Status=%d, TxType=%d, ContractAddress=%s\n", i, tx.TxHash, tx.Fee, tx.Status, tx.TxType, tx.ContractAddress)
			for _, from := range tx.From {
				fmt.Printf("    From: %s, Amount: %s\n", from.Address, from.Amount)
				if from.MetaData != "" {
					fmt.Printf("      MetaData: %s\n", from.MetaData)
				}
			}
			for _, to := range tx.To {
				fmt.Printf("    To: %s, Amount: %s\n", to.Address, to.Amount)
				if to.MetaData != "" {
					fmt.Printf("      MetaData: %s\n", to.MetaData)
				}
			}
		}

	})

	// Polkadot方式
	t.Run("Polkadot", func(t *testing.T) {
		var address string = "13UVJyLgBASGhE2ok3TvxUfaQBGUt88JCcdYjHvUhvQkFTTx"

		scanCtx, cancel := context.WithTimeout(ctx, time.Minute*5)
		defer cancel()

		cli, err := substratebase.NewSubstrateClient(conf.WalletNode.Dot.SubstrateRpcUrl)
		if err != nil {
			t.Fatalf("连接Substrate客户端失败: %v", err)
		}
		polkadotAdaptor := &SubstrateChainAdaptor{
			substrateClient: cli,
			ss58Prefix:      uint8(substratebase.PolkadotSS58Prefix),
		}

		req := &wallet_api.TransactionByAddressRequest{
			Address: address,
		}

		resp, err := polkadotAdaptor.GetTransactionByAddress(scanCtx, req)
		if err != nil {
			t.Fatalf("根据地址获取交易详情失败: %v", err)
		}
		if resp.Transaction == nil {
			t.Fatalf("交易详情为空: code=%d, msg=%s", resp.Code, resp.Msg)
		}

		fmt.Printf("交易数量: %d\n", len(resp.Transaction))
		for i, tx := range resp.Transaction {
			fmt.Printf("  交易[%d]: TxHash=%s, Fee=%s, Status=%d, TxType=%d, ContractAddress=%s\n", i, tx.TxHash, tx.Fee, tx.Status, tx.TxType, tx.ContractAddress)
			for _, from := range tx.From {
				fmt.Printf("    From: %s, Amount: %s\n", from.Address, from.Amount)
				if from.MetaData != "" {
					fmt.Printf("      MetaData: %s\n", from.MetaData)
				}
			}
			for _, to := range tx.To {
				fmt.Printf("    To: %s, Amount: %s\n", to.Address, to.Amount)
				if to.MetaData != "" {
					fmt.Printf("      MetaData: %s\n", to.MetaData)
				}
			}
		}
	})

}

// TestGetAccountBalance 测试获取账户余额
func TestGetAccountBalance(t *testing.T) {

	ctx := context.Background()
	conf, err := config.NewConfig("../../config.yml")
	if err != nil {
		t.Fatalf("加载配置文件失败: %v", err)
	}

	// EVM方式
	t.Run("EVM", func(t *testing.T) {
		var address string = "0xf3CF0a843a63fBf8b05d500cB86c602279925d43"

		ethClient, err := evmbase.DialEthClient(context.Background(), conf.WalletNode.Dot.RpcUrl)
		if err != nil {
			t.Fatalf("连接EVM客户端失败: %v", err)
		}

		ethDataClient, err := evmbase.NewEthDataClient(conf.WalletNode.Dot.DataApiUrl, conf.WalletNode.Dot.DataApiKey, time.Second*15)
		if err != nil {
			t.Fatalf("创建EVM数据客户端失败: %v", err)
		}
		evmAdaptor := &EvmChainAdaptor{
			ethClient:     ethClient,
			ethDataClient: ethDataClient,
		}

		req := &wallet_api.AccountBalanceRequest{
			Address: address,
		}

		resp, err := evmAdaptor.GetAccountBalance(ctx, req)
		if err != nil {
			t.Fatalf("获取EVM账户余额失败: %v", err)
		}

		if resp.Code != wallet_api.ApiReturnCode_APISUCCESS {
			t.Errorf("获取EVM账户余额返回错误: code=%d, msg=%s", resp.Code, resp.Msg)
		} else {
			t.Logf("EVM账户余额: %s", resp.Balance)
		}
	})

	// Polkadot方式
	t.Run("Polkadot", func(t *testing.T) {
		var address string = "13Z7KjGnzdAdMre9cqRwTZHR6F2p36gqBsaNmQwwosiPz8JT"

		cli, err := substratebase.NewSubstrateClient(conf.WalletNode.Dot.SubstrateRpcUrl)
		if err != nil {
			t.Fatalf("连接Substrate客户端失败: %v", err)
		}
		polkadotAdaptor := &SubstrateChainAdaptor{
			substrateClient: cli,
			ss58Prefix:      uint8(substratebase.PolkadotSS58Prefix),
		}

		req := &wallet_api.AccountBalanceRequest{
			Address: address,
		}

		resp, err := polkadotAdaptor.GetAccountBalance(ctx, req)
		if err != nil {
			t.Fatalf("获取Polkadot账户余额失败: %v", err)
		}

		if resp.Code != wallet_api.ApiReturnCode_APISUCCESS {
			t.Errorf("获取Polkadot账户余额返回错误: code=%d, msg=%s", resp.Code, resp.Msg)
		} else {
			for _, tokenBalance := range resp.TokenBalances {
				fmt.Printf(" Symbol: %s, 余额: %s\n", tokenBalance.Symbol, tokenBalance.Balance)
			}
		}

	})

}
