package main

import (
	"flag"
	"fmt"

	"github.com/binance-chain/go-sdk/common/types"
	config "github.com/celer-network/bsc-relayer/config"
	"github.com/celer-network/bsc-relayer/executor"
	"github.com/celer-network/bsc-relayer/tendermint/light"
	"github.com/celer-network/goutils/log"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/celer-network/bsc-relayer/relayer"
)

const (
	flagConfigPath     = "config-path"
	flagBBCNetworkType = "bbc-network-type"
	flagLogLevel       = "log-level"
)

func initFlags() {
	flag.String(flagConfigPath, "", "config file path")
	flag.Int(flagBBCNetworkType, int(types.TmpTestNetwork), "Binance chain network type")
	flag.String(flagLogLevel, "info", "log level")

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	err := viper.BindPFlags(pflag.CommandLine)
	if err != nil {
		panic(err)
	}
}

func printUsage() {
	fmt.Print("usage: ./bsc-relayer --bbc-network-type 0 --config-path configFile\n --log-level debug")
}

func main() {
	initFlags()

	bbcNetworkType := viper.GetInt(flagBBCNetworkType)
	configFilePath := viper.GetString(flagConfigPath)
	logLevel := viper.GetString(flagLogLevel)
	log.Infof("log level %s", logLevel)
	log.SetLevelByName(logLevel)
	var cfg *config.Config
	if configFilePath == "" {
		log.Panic("empty config file path provided")
	}
	cfg = config.ParseConfigFromFile(configFilePath)
	relayerInstance, err := relayer.NewRelayer(types.ChainNetwork(bbcNetworkType), cfg)
	if err != nil {
		panic(err)
	}
	log.Info("Starting relayer")
	relayerInstance.MonitorValidatorSetChange(
		0, []byte{}, []byte{},
		func(header *light.TmHeader) {},
		func(pkg executor.CrossChainPackage) {},
	)
}
