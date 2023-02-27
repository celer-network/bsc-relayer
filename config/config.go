package util

import (
	"encoding/json"
	"io/ioutil"

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
	CrossChainConfig CrossChainConfig `json:"cross_chain_config"`
	BBCConfig        BBCConfig        `json:"bbc_config"`
	LogConfig        LogConfig        `json:"log_config"`
}

type CrossChainConfig struct {
	SourceChainID      uint16  `json:"source_chain_id"`
	DestChainID        uint16  `json:"dest_chain_id"`
	MonitorChannelList []uint8 `json:"monitor_channel_list"`
	CompetitionMode    bool    `json:"competition_mode"`
}

func (cfg *CrossChainConfig) Validate() {
}

type BBCConfig struct {
	RpcAddrs                     []string `json:"rpc_addrs"`
	SleepMillisecondForWaitBlock int64    `json:"sleep_millisecond_for_wait_block"`
}

func (cfg *BBCConfig) Validate() {
	if len(cfg.RpcAddrs) == 0 {
		panic("rpc endpoint of Binance chain should not be empty")
	}
	if cfg.SleepMillisecondForWaitBlock < 0 {
		panic("SleepMillisecondForWaitBlock must not be negative")
	}
}

type LogConfig struct {
	Level                        string `json:"level"`
	Filename                     string `json:"filename"`
	MaxFileSizeInMB              int    `json:"max_file_size_in_mb"`
	MaxBackupsOfLogFiles         int    `json:"max_backups_of_log_files"`
	MaxAgeToRetainLogFilesInDays int    `json:"max_age_to_retain_log_files_in_days"`
	UseConsoleLogger             bool   `json:"use_console_logger"`
	UseFileLogger                bool   `json:"use_file_logger"`
	Compress                     bool   `json:"compress"`
}

func (cfg *LogConfig) Validate() {
	if cfg.UseFileLogger {
		if cfg.Filename == "" {
			panic("filename should not be empty if use file logger")
		}
		if cfg.MaxFileSizeInMB <= 0 {
			panic("max_file_size_in_mb should be larger than 0 if use file logger")
		}
		if cfg.MaxBackupsOfLogFiles <= 0 {
			panic("max_backups_off_log_files should be larger than 0 if use file logger")
		}
	}
}

func (cfg *Config) Validate() {
	cfg.CrossChainConfig.Validate()
	cfg.LogConfig.Validate()
	cfg.BBCConfig.Validate()
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

	config.Validate()

	return &config
}
