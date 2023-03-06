package util

import (
	"encoding/json"
	"io/ioutil"

	"github.com/binance-chain/go-sdk/common/types"
	"github.com/ethereum/go-ethereum/common"
)

type ChannelConfig struct {
	ChannelID      int8           `json:"channel_id"`
	Method         string         `json:"method"`
	ABIName        string         `json:"abi_name"`
	ContractAddr   common.Address `json:"contract_addr"`
	SequenceMethod string         `json:"sequence_method"`
}

type Config struct {
	NetworkType      types.ChainNetwork `json:"network_type"`
	CrossChainConfig CrossChainConfig   `json:"cross_chain_config"`
	BBCConfig        BBCConfig          `json:"bbc_config"`
}

type CrossChainConfig struct {
	SourceChainID uint16 `json:"source_chain_id"`
	DestChainID   uint16 `json:"dest_chain_id"`
}

func (cfg *CrossChainConfig) Validate() {
}

type BBCConfig struct {
	RpcAddrs                     []string `json:"rpc_addrs"`
	SleepMillisecondForWaitBlock int64    `json:"sleep_millisecond_for_wait_block"`
}

func (cfg *BBCConfig) Validate() bool {
	if len(cfg.RpcAddrs) == 0 {
		return false
	}
	if cfg.SleepMillisecondForWaitBlock < 0 {
		return false
	}
	return true
}

func (cfg *Config) Validate() bool {
	return cfg.BBCConfig.Validate()
}

func ParseConfigFromJson(content string) *Config {
	var config Config
	if err := json.Unmarshal([]byte(content), &config); err != nil {
		panic(err)
	}
	return &config
}

func ParseConfigFromFile(filePath string) *Config {
	bz, err := ioutil.ReadFile(filePath)
	if err != nil {
		panic(err)
	}

	var config Config
	if err := json.Unmarshal(bz, &config); err != nil {
		panic(err)
	}

	if ok := config.Validate(); !ok {
		return nil
	}

	return &config
}
