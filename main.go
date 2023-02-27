package main

import (
	"flag"
	"fmt"

	"github.com/binance-chain/go-sdk/common/types"
	config "github.com/celer-network/bsc-relayer/config"
	"github.com/celer-network/bsc-relayer/executor"
	"github.com/celer-network/goutils/log"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/celer-network/bsc-relayer/common"
	"github.com/celer-network/bsc-relayer/relayer"
)

const (
	flagConfigPath     = "config-path"
	flagConfigType     = "config-type"
	flagBBCNetworkType = "bbc-network-type"
)

func initFlags() {
	flag.String(flagConfigPath, "", "config file path")
	flag.String(flagConfigType, "local_private_key", "config type, local_private_key or aws_private_key")
	flag.Int(flagBBCNetworkType, int(types.TmpTestNetwork), "Binance chain network type")

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	err := viper.BindPFlags(pflag.CommandLine)
	if err != nil {
		panic(err)
	}
}

func printUsage() {
	fmt.Print("usage: ./bsc-relayer --config-type local --config-path configFile\n")
	fmt.Print("usage: ./bsc-relayer --config-type aws --aws-region awsRegin --aws-secret-key awsSecretKey\n")
}

func main() {
	initFlags()

	bbcNetworkType := viper.GetInt(flagBBCNetworkType)
	configFilePath := viper.GetString(flagConfigPath)
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
		func(header *common.Header) {},
		func(pkg executor.CrossChainPackage) {},
	)
}
