package util

import (
	"encoding/json"
	"io/ioutil"

	"github.com/binance-chain/go-sdk/common/types"
)

type Config struct {
	NetworkType                  types.ChainNetwork `json:"network_type"`
	CrossChainConfig             CrossChainConfig   `json:"cross_chain_config"`
	RpcAddrs                     []string           `json:"rpc_addrs"`
	SleepMillisecondForWaitBlock int64              `json:"sleep_millisecond_for_wait_block"`
}

type CrossChainConfig struct {
	SourceChainID uint16 `json:"source_chain_id"`
	DestChainID   uint16 `json:"dest_chain_id"`
}

func (cfg *Config) Validate() bool {
	if len(cfg.RpcAddrs) == 0 {
		return false
	}
	if cfg.SleepMillisecondForWaitBlock < 0 {
		return false
	}
	return true
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
