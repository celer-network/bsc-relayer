package relayer

import (
	"fmt"
	"time"

	"github.com/binance-chain/bsc-relayer/common"
	config "github.com/binance-chain/bsc-relayer/config"
	"github.com/binance-chain/bsc-relayer/executor"
	"github.com/binance-chain/go-sdk/common/types"
	"github.com/celer-network/goutils/log"
)

type Relayer struct {
	cfg         *config.Config
	BBCExecutor *executor.BBCExecutor
}

func NewRelayer(bbcNetworkType types.ChainNetwork, configFilePath string) (*Relayer, error) {
	if bbcNetworkType != types.TestNetwork && bbcNetworkType != types.TmpTestNetwork && bbcNetworkType != types.ProdNetwork {
		return nil, fmt.Errorf("unknown bbc network type %d", int(bbcNetworkType))
	}

	var cfg *config.Config
	if configFilePath == "" {
		return nil, fmt.Errorf("empty config file path provided")
	}
	cfg = config.ParseConfigFromFile(configFilePath)

	if cfg == nil {
		return nil, fmt.Errorf("failed to parse configuration from file %s", configFilePath)
	}

	bbcExecutor, err := executor.NewBBCExecutor(cfg, bbcNetworkType)
	if err != nil {
		return nil, fmt.Errorf("NewBBCExecutor err: %s", err.Error())
	}

	return &Relayer{
		cfg:         cfg,
		BBCExecutor: bbcExecutor,
	}, nil
}

type SyncBBCHeaderCallbackFunc func(header *common.Header)

type RelayCrossChainPackageCallbackFunc func(pkg executor.CrossChainPackage)

func (r *Relayer) MonitorValidatorSetChange(height uint64, bbcHash, bscHash []byte, callback1 SyncBBCHeaderCallbackFunc, callback2 RelayCrossChainPackageCallbackFunc) {
	if height == 0 {
		height = r.getLatestHeight()
	}
	if height == 0 {
		return
	}

	log.Infof("Start monitor all packages in channel 8 from height %d", height)
	advance := false
	bbcChanged, bscChanged := false, false
	var err error
	// 1st header for bbc validator set change, at height
	// 2nd header for ibc package(bsc validator set change), at height+1
	var firstHeader, SecondHeader *common.Header
	for ; ; height = r.waitForNextBlock(height, advance) {
		// check bbc validator set
		bbcChanged, bbcHash, err = r.BBCExecutor.CheckValidatorSetChange(int64(height), bbcHash)
		if err != nil {
			log.Errorf("CheckValidatorSetChange err:%s", err.Error())
			advance = false
			continue
		}
		// get first bbc header
		if bbcChanged {
			firstHeader, err = r.BBCExecutor.QueryTendermintHeader(int64(height))
			if err != nil {
				log.Errorf("QueryTendermintHeader err:%s", err.Error())
				advance = false
				continue
			}
		}

		log.Debugf("Finding packages in channel 8 in height %d", height)
		packageSet, err := r.BBCExecutor.FindAllStakingModulePackages(int64(height))
		if err != nil {
			log.Errorf("FindAllStakingModulePackages err:%s", err.Error())
			advance = false
			continue
		}
		var pkg *executor.CrossChainPackage
		bscChanged, bscHash, pkg = executor.FindBscValidatorSetChangePackage(bscHash, packageSet)

		// get second bbc header
		if bscChanged {
			SecondHeader, err = r.BBCExecutor.QueryTendermintHeader(int64(height) + 1)
			if err != nil {
				log.Errorf("QueryTendermintHeader err:%s", err.Error())
				advance = false
				continue
			}
		}

		// after gotten all data, trigger callback function
		if bbcChanged {
			callback1(firstHeader)
		}
		if bscChanged {
			callback1(SecondHeader)
			callback2(*pkg)
		}
		advance = true
	}
}

func (r *Relayer) waitForNextBlock(height uint64, advance bool) uint64 {
	sleepTime := time.Duration(r.BBCExecutor.Config.BBCConfig.SleepMillisecondForWaitBlock * int64(time.Millisecond))
	if !advance {
		time.Sleep(sleepTime)
		return height
	}
	for {
		curHeight := r.getLatestHeight()
		if curHeight > height {
			return height + 1
		}
		time.Sleep(sleepTime)
	}
}

func (r *Relayer) getLatestHeight() uint64 {
	abciInfo, err := r.BBCExecutor.GetClient().ABCIInfo()
	if err != nil {
		log.Errorf("Query latest height error: %s", err.Error())
		return 0
	}
	return uint64(abciInfo.Response.LastBlockHeight)
}
