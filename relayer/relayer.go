package relayer

import (
	"fmt"
	"time"

	"github.com/binance-chain/go-sdk/common/types"
	config "github.com/celer-network/bsc-relayer/config"
	"github.com/celer-network/bsc-relayer/executor"
	"github.com/celer-network/bsc-relayer/tendermint/light"
	"github.com/celer-network/goutils/log"
)

type Relayer struct {
	cfg         *config.Config
	BBCExecutor *executor.BBCExecutor
}

func NewRelayer(bbcNetworkType types.ChainNetwork, cfg *config.Config) (*Relayer, error) {
	if bbcNetworkType != types.TestNetwork && bbcNetworkType != types.TmpTestNetwork && bbcNetworkType != types.ProdNetwork {
		return nil, fmt.Errorf("unknown bbc network type %d", int(bbcNetworkType))
	}
	if cfg == nil || !cfg.Validate() {
		return nil, fmt.Errorf("nil or invalid config")
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

type SyncBBCHeaderCallbackFunc func(tmHeader *light.TmHeader)

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
	var firstHeader, SecondHeader *light.TmHeader
	for ; ; height, advance = r.waitForNextBlock(height, advance) {
		// check bbc validator set
		bbcChanged, bbcHash, err = r.BBCExecutor.CheckValidatorSetChange(int64(height), bbcHash)
		if err != nil {
			log.Errorf("CheckValidatorSetChange err:%s", err.Error())
			continue
		}
		// get first bbc header
		if bbcChanged {
			header, err := r.BBCExecutor.QueryTendermintHeader(int64(height))
			if err != nil {
				log.Errorf("QueryTendermintHeader err:%s", err.Error())
				continue
			}
			firstHeader, err = new(light.TmHeader).FromType(header)
			if err != nil {
				log.Errorf("Header conversion err:%s", err.Error())
				continue
			}
		}

		log.Debugf("Finding packages in channel 8 in height %d", height)
		packageSet, err := r.BBCExecutor.FindAllStakingModulePackages(int64(height))
		if err != nil {
			log.Errorf("FindAllStakingModulePackages err:%s", err.Error())
			continue
		}
		var pkg *executor.CrossChainPackage
		bscChanged, bscHash, pkg = executor.FindBscValidatorSetChangePackage(bscHash, packageSet)

		// get second bbc header
		if bscChanged {
			header, err := r.BBCExecutor.QueryTendermintHeader(int64(height) + 1)
			if err != nil {
				log.Errorf("QueryTendermintHeader err:%s", err.Error())
				continue
			}
			SecondHeader, err = new(light.TmHeader).FromType(header)
			if err != nil {
				log.Errorf("FindAllStakingModulePackages err:%s", err.Error())
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

func (r *Relayer) waitForNextBlock(height uint64, advance bool) (uint64, bool) {
	sleepTime := time.Duration(r.BBCExecutor.Config.BBCConfig.SleepMillisecondForWaitBlock * int64(time.Millisecond))
	if !advance {
		time.Sleep(sleepTime)
		return height, false
	}
	for {
		curHeight := r.getLatestHeight()
		if curHeight > height {
			return height + 1, false
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
