package relayer

import (
	"fmt"
	"time"

	"github.com/binance-chain/bsc-relayer/admin"
	"github.com/binance-chain/bsc-relayer/common"
	config "github.com/binance-chain/bsc-relayer/config"
	"github.com/binance-chain/bsc-relayer/executor"
	"github.com/binance-chain/go-sdk/common/types"
	"github.com/jinzhu/gorm"
	cmn "github.com/tendermint/tendermint/libs/common"
)

type Relayer struct {
	db          *gorm.DB
	cfg         *config.Config
	bbcExecutor *executor.BBCExecutor
	bscExecutor *executor.BSCExecutor
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

	//todo
	common.InitLogger(&cfg.LogConfig)

	//todo
	adm := admin.NewAdmin(nil, cfg)
	go adm.Serve()

	bbcExecutor, err := executor.NewBBCExecutor(cfg, bbcNetworkType)
	if err != nil {
		return nil, fmt.Errorf("NewBBCExecutor err: %s", err.Error())
	}

	//bscExecutor, err := executor.NewBSCExecutor(nil, bbcExecutor, cfg)
	//if err != nil {
	//	return nil, fmt.Errorf("NewBBCExecutor err: %s", err.Error())
	//}

	return &Relayer{
		cfg:         cfg,
		bbcExecutor: bbcExecutor,
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
	//height := uint64(36498451)

	common.Logger.Infof("Start monitor all packages in channel 8 from height %d", height)
	advance := false
	bbcChanged, bscChanged := false, false
	var err error
	// 1st header for bbc validator set change, at height
	// 2nd header for ibc package(bsc validator set change), at height+1
	var firstHeader, SecondHeader *common.Header
	for ; ; height = r.waitForNextBlock(height, advance) {
		// check bbc validator set
		bbcChanged, bbcHash, err = r.bbcExecutor.CheckValidatorSetChange(int64(height), bbcHash)
		if err != nil {
			advance = false
			continue
		}
		// get first bbc header
		if bbcChanged {
			firstHeader, err = r.bbcExecutor.QueryTendermintHeader(int64(height))
			if err != nil {
				advance = false
				continue
			}
		}

		common.Logger.Debugf("Finding packages in channel 8 in height %d", height)
		packageSet, err := r.bbcExecutor.FindAllStakingModulePackages(int64(height))
		if err != nil {
			advance = false
			continue
		}
		var pkg *executor.CrossChainPackage
		bscChanged, bscHash, pkg = executor.FindBscValidatorSetChangePackage(bscHash, packageSet)

		// get second bbc header
		if bscChanged {
			SecondHeader, err = r.bbcExecutor.QueryTendermintHeader(int64(height) + 1)
			if err != nil {
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
	sleepTime := time.Duration(r.bbcExecutor.Config.BBCConfig.SleepMillisecondForWaitBlock * int64(time.Millisecond))
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

func (r *Relayer) GetBBCStatus() (uint64, cmn.HexBytes, error) {
	abciInfo, err := r.bbcExecutor.GetClient().ABCIInfo()
	if err != nil {
		return 0, nil, err
	}
	latestHeight := abciInfo.Response.LastBlockHeight
	block, err := r.bbcExecutor.GetClient().Block(&(latestHeight))
	if err != nil {
		return 0, nil, err
	}
	return uint64(latestHeight), block.BlockMeta.Header.ValidatorsHash, nil
}

func (r *Relayer) Start(startHeight uint64, curValidatorsHash cmn.HexBytes) {

	// this relayer will not act as a real BC-BSC relayer
	//r.registerRelayerHub()

	if r.cfg.CrossChainConfig.CompetitionMode {
		_, err := r.cleanPreviousPackages(startHeight)
		if err != nil {
			common.Logger.Errorf("failure in cleanPreviousPackages: %s", err.Error())
		}
		go r.relayerCompetitionDaemon(startHeight, curValidatorsHash)
	} else {
		go r.relayerDaemon(curValidatorsHash)
	}

	go r.bbcExecutor.UpdateClients()
	//go r.bscExecutor.UpdateClients()

	go r.txTracker()

	//go r.autoClaimRewardDaemon()

	if len(r.cfg.BSCConfig.MonitorDataSeedList) >= 2 {
		go r.doubleSignMonitorDaemon()
	}
	go r.alert()
}
