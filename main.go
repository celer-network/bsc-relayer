package main

import (
	"database/sql"
	"flag"
	"fmt"

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
	flagDB         = "db"
	flagConfigPath = "config-path"
	flagLogLevel   = "log-level"
)

func initFlags() {
	flag.String(flagDB, "localhost:26257", "db host")
	flag.String(flagConfigPath, "", "config file path")
	flag.String(flagLogLevel, "info", "log level")

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	err := viper.BindPFlags(pflag.CommandLine)
	if err != nil {
		panic(err)
	}
}

func printUsage() {
	fmt.Print("usage: ./bsc-relayer --config-path configFile\n --log-level debug")
}

func main() {
	initFlags()

	dbHost := viper.GetString(flagDB)
	configFilePath := viper.GetString(flagConfigPath)
	logLevel := viper.GetString(flagLogLevel)
	log.Infof("log level %s", logLevel)
	log.SetLevelByName(logLevel)
	var cfg *config.Config
	if configFilePath == "" {
		log.Panic("empty config file path provided")
	}
	cfg = config.ParseConfigFromFile(configFilePath)
	dbUrl := fmt.Sprintf("postgresql://root@%s/bbc-relayer?sslmode=disable", dbHost)
	db, err := sql.Open("postgres", dbUrl)
	if err != nil {
		log.Panic("open db, err:%s", err.Error())
	}
	relayerInstance, err := relayer.NewRelayer(cfg, db)
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
