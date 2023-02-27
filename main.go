package main

import (
	"flag"
	"fmt"

	"github.com/binance-chain/bsc-relayer/executor"
	"github.com/binance-chain/go-sdk/common/types"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/binance-chain/bsc-relayer/common"
	"github.com/binance-chain/bsc-relayer/relayer"
)

const (
	flagConfigPath         = "config-path"
	flagConfigType         = "config-type"
	flagBBCNetworkType     = "bbc-network-type"
	flagConfigAwsRegion    = "aws-region"
	flagConfigAwsSecretKey = "aws-secret-key"
)

func initFlags() {
	flag.String(flagConfigPath, "", "config file path")
	flag.String(flagConfigType, "local_private_key", "config type, local_private_key or aws_private_key")
	flag.Int(flagBBCNetworkType, int(types.TmpTestNetwork), "Binance chain network type")
	flag.String(flagConfigAwsRegion, "", "aws region")
	flag.String(flagConfigAwsSecretKey, "", "aws secret key")

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
	relayerInstance, err := relayer.NewRelayer(types.ChainNetwork(bbcNetworkType), configFilePath)
	if err != nil {
		panic(err)
	}
	common.Logger.Info("Starting relayer")
	relayerInstance.MonitorValidatorSetChange(
		0, []byte{}, []byte{},
		func(header *common.Header) {},
		func(pkg executor.CrossChainPackage) {},
	)
}
