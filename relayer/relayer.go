package relayer

import (
	"fmt"
	"time"

	"github.com/binance-chain/bsc-relayer/admin"
	"github.com/binance-chain/bsc-relayer/common"
	config "github.com/binance-chain/bsc-relayer/config"
	"github.com/binance-chain/bsc-relayer/executor"
	"github.com/binance-chain/go-sdk/common/types"
	common2 "github.com/ethereum/go-ethereum/common"
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

type MonitorBBCvscConfig struct {
	InitValidatorsHash []byte
}

func (r *Relayer) MonitorBBCValidatorSetChange(config MonitorBBCvscConfig, callback SyncBBCHeaderCallbackFunc) {
	curValidatorsHash := config.InitValidatorsHash
	height := r.getLatestHeight()
	for {
		changed, curHash, err := r.bbcExecutor.CheckValidatorSetChange(int64(height), curValidatorsHash)
		if err != nil {
			common.Logger.Errorf("CheckValidatorSetChange err:%s", err.Error())
			continue
		}
		curValidatorsHash = curHash
		if changed {
			header, err := r.bbcExecutor.QueryTendermintHeader(int64(height))
			if err != nil {
				common.Logger.Errorf("QueryTendermintHeader err:%s", err.Error())
				continue
			}
			callback(header)
			common.Logger.Infof("tendermint header %s, validator set %s, next validator set %s", header.Header.StringIndented(""), header.ValidatorSet, header.NextValidatorSet)
		}
		height = r.waitForNextBlock(height, true)
	}
}

func (r *Relayer) MonitorStakingChannel(callback1 SyncBBCHeaderCallbackFunc, callback2 RelayCrossChainPackageCallbackFunc) {
	height := r.getLatestHeight() - 1
	common.Logger.Infof("Start monitor all packages in channel 8 from height %d", height)
	for {
		common.Logger.Infof("Finding packages in channel 8 in height %d", height)
		packageSet, err := r.bbcExecutor.FindAllStakingModulePackages(int64(height))
		if err != nil {
			common.Logger.Errorf("FindAllStakingModulePackages err:%s.", err.Error())
			continue
		}

		if len(packageSet) == 0 {
			height++
			continue // skip this height
		}

		header, err := r.bbcExecutor.QueryTendermintHeader(int64(height))
		if err != nil {
			common.Logger.Errorf("QueryTendermintHeader err:%s", err.Error())
			continue
		}
		callback1(header)
		common.Logger.Infof("tendermint header %s, validator set %s, next validator set %s", header.Header.StringIndented(""), header.ValidatorSet, header.NextValidatorSet)

		for _, pkg := range packageSet {
			callback2(pkg.ChannelID, pkg.Height, pkg.Sequence, pkg.Msg, pkg.Proof)
			common.Logger.Infof("cross chain package, channel %d, sequence %d, height %d, msg %s", pkg.ChannelID, pkg.Sequence, pkg.Height)
			// try to decode package
			if ok, ibcValidatorSetPkg := pkg.ToIbcValidateSetPackage(); ok {
				common.Logger.Infof("succeeded to decode this msg to IbcValidatorSetPackage. %v", ibcValidatorSetPkg)
			}
		}
		height = r.waitForNextBlock(height, true)
	}
}

type RelayCrossChainPackageCallbackFunc func(channelID common.CrossChainChannelID, height, sequence uint64, msgBytes, proofBytes []byte)

type MonitorBSCVSCConfig struct {
	InitBBCValidatorsHash common2.Hash
}

func (r *Relayer) MonitorBSCValidatorSetChange(callback1 SyncBBCHeaderCallbackFunc, callback2 RelayCrossChainPackageCallbackFunc) {
	present := time.Now()
	midNight := time.Date(present.Year(), present.Month(), present.Day()+1, 0, 0, 0, 0, time.UTC)
	time.AfterFunc(midNight.Sub(present), func() {
		ticker := time.NewTicker(24 * time.Hour)
		for {
			curHeight := r.getLatestHeight()
			advance := false
			for height := curHeight - 1; height < curHeight+9; height = r.waitForNextBlock(height, advance) {
				packageSet, err := r.bbcExecutor.FindAllStakingModulePackages(int64(height))
				if err != nil {
					common.Logger.Errorf("FindAllStakingModulePackages err:%s", err.Error())
					continue
				}
				advance = true
				for _, pkg := range packageSet {
					if ok, ibcValidatorSetPkg := pkg.ToIbcValidateSetPackage(); ok {
						common.Logger.Infof("succeeded to decode this msg to IbcValidatorSetPackage. %v", ibcValidatorSetPkg)
						header, err := r.bbcExecutor.QueryTendermintHeader(int64(height))
						if err != nil {
							advance = false
							common.Logger.Errorf("QueryTendermintHeader err:%s", err.Error())
							break
						}
						callback1(header)
						common.Logger.Infof("tendermint header %s, validator set %s, next validator set %s", header.Header.StringIndented(""), header.ValidatorSet, header.NextValidatorSet)
						callback2(pkg.ChannelID, pkg.Height, pkg.Sequence, pkg.Msg, pkg.Proof)
						common.Logger.Infof("cross chain package, channel %d, sequence %d, height %d, msg %s", pkg.ChannelID, pkg.Sequence, pkg.Height)
					}
				}
			}
			<-ticker.C
		}
	})
}

func (r *Relayer) waitForNextBlock(height uint64, advance bool) uint64 {
	if !advance {
		return height
	}
	sleepTime := time.Duration(r.bbcExecutor.Config.BBCConfig.SleepMillisecondForWaitBlock * int64(time.Millisecond))
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
	startHeight := abciInfo.Response.LastBlockHeight - 1
	block, err := r.bbcExecutor.GetClient().Block(&(startHeight))
	if err != nil {
		return 0, nil, err
	}
	return uint64(startHeight), block.BlockMeta.Header.ValidatorsHash, nil
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
