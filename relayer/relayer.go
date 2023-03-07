package relayer

import (
	"context"
	"fmt"
	"time"

	"github.com/binance-chain/go-sdk/common/types"
	"github.com/celer-network/bsc-relayer/common"
	config "github.com/celer-network/bsc-relayer/config"
	"github.com/celer-network/bsc-relayer/executor"
	"github.com/celer-network/bsc-relayer/model"
	"github.com/celer-network/goutils/log"
	ethcmm "github.com/ethereum/go-ethereum/common"
)

type Relayer struct {
	queries     *model.Queries
	cfg         *config.Config
	BBCExecutor *executor.BBCExecutor
}

func NewRelayer(cfg *config.Config, db model.DBTX) (*Relayer, error) {
	bbcNetworkType := cfg.NetworkType
	if bbcNetworkType != types.TestNetwork && bbcNetworkType != types.TmpTestNetwork && bbcNetworkType != types.ProdNetwork {
		return nil, fmt.Errorf("unknown bbc network type %d", int(bbcNetworkType))
	}
	if cfg == nil || !cfg.Validate() {
		return nil, fmt.Errorf("nil or invalid config")
	}

	bbcExecutor, err := executor.NewBBCExecutor(cfg)
	if err != nil {
		return nil, fmt.Errorf("NewBBCExecutor err: %s", err.Error())
	}

	relayer := &Relayer{
		cfg:         cfg,
		BBCExecutor: bbcExecutor,
	}

	if db == nil {
		relayer.queries = nil
	} else {
		relayer.queries = model.New(db)
		err = relayer.initDB()
		if err != nil {
			return nil, err
		}
	}

	return relayer, nil
}

type SyncBBCHeaderCallbackFunc func(header *common.Header)

type RelayCrossChainPackageCallbackFunc func(pkg *executor.CrossChainPackage)

func (r *Relayer) MonitorValidatorSetChange(height uint64, bbcHash, bscHash []byte, callback1 SyncBBCHeaderCallbackFunc, callback2 RelayCrossChainPackageCallbackFunc) {
	statusFromDB, err := r.GetBBCStatus()
	if err != nil {
		log.Errorf("GetBBCStatus from db, err:%s", err.Error())
		return
	}
	if height == 0 {
		if statusFromDB.Height == 0 {
			height = r.getLatestHeight()
		} else {
			height = statusFromDB.Height
		}
	}
	if bbcHash == nil || len(bbcHash) == 0 {
		bbcHash = ethcmm.Hex2Bytes(statusFromDB.BbcValsHash)
	}
	if bscHash == nil || len(bscHash) == 0 {
		bscHash = ethcmm.Hex2Bytes(statusFromDB.BscValsHash)
	}

	if height == 0 {
		log.Errorf("MonitorValidatorSetChange starts at height 0, which is not expected.")
		return
	}

	log.Infof("Start monitor all packages in channel 8 from height %d", height)
	advance := false
	bbcChanged, bscChanged := false, false
	// 1st header for bbc validator set change, at height
	// 2nd header for ibc package(bsc validator set change), at height+1
	var firstHeader, SecondHeader *common.Header
	for ; ; height, advance = r.waitForNextBlock(height, advance) {
		// check bbc validator set
		bbcChanged, bbcHash, err = r.BBCExecutor.CheckValidatorSetChange(int64(height), bbcHash)
		if err != nil {
			log.Errorf("CheckValidatorSetChange err:%s", err.Error())
			continue
		}
		// get first bbc header
		if bbcChanged {
			firstHeader, err = r.BBCExecutor.QueryTendermintHeader(int64(height))
			if err != nil {
				log.Errorf("QueryTendermintHeader err:%s", err.Error())
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
			SecondHeader, err = r.BBCExecutor.QueryTendermintHeader(int64(height) + 1)
			if err != nil {
				log.Errorf("QueryTendermintHeader err:%s", err.Error())
				continue
			}
		}

		// after gotten all data, trigger callback function
		if bbcChanged {
			callback1(firstHeader)
			err = r.updateBBCValsHash(bbcHash)
			if err != nil {
				log.Errorf("UpdateBBCValsHash into db, err:%s", err.Error())
			}
		}
		if bscChanged {
			callback1(SecondHeader)
			callback2(pkg)
			err = r.updateBSCValsHash(pkg.Sequence, bscHash)
			if err != nil {
				log.Errorf("UpdateBSCValsHash into db, err:%s", err.Error())
			}
		}
		advance = true
	}
}

func (r *Relayer) waitForNextBlock(height uint64, advance bool) (uint64, bool) {
	sleepTime := time.Duration(r.BBCExecutor.Config.SleepMillisecondForWaitBlock * int64(time.Millisecond))
	if !advance {
		time.Sleep(sleepTime)
		return height, false
	}
	for {
		curHeight := r.getLatestHeight()
		if curHeight > height {
			err := r.updateHeight(height)
			if err != nil {
				log.Errorf("UpdateHeight into db, err:%s", err.Error())
				continue
			}
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

func (r *Relayer) GetBBCStatus() (model.BbcStatus, error) {
	if r.queries == nil {
		return model.BbcStatus{}, nil
	}
	return r.queries.GetBBCStatus(context.Background(), uint64(r.cfg.NetworkType))
}

func (r *Relayer) UpdateAfterSync(height uint64) error {
	if r.queries == nil {
		return nil
	}
	return r.queries.UpdateAfterSync(context.Background(), model.UpdateAfterSyncParams{
		NetworkID: uint64(r.cfg.NetworkType),
		SyncedAt:  height,
	})
}

// initBBCStatus insert a row into bbc_status table when relayer starts for the first time
func (r *Relayer) initBBCStatus(height uint64, bbcHash, bscHash []byte) error {
	if r.queries == nil {
		return nil
	}
	return r.queries.InitBBCStatus(context.Background(), model.InitBBCStatusParams{
		NetworkID:   uint64(r.cfg.NetworkType),
		Height:      height,
		BbcValsHash: ethcmm.Bytes2Hex(bbcHash),
		BscValsHash: ethcmm.Bytes2Hex(bscHash),
	})
}

// updateHeight update height of bbc_status table
func (r *Relayer) updateHeight(height uint64) error {
	if r.queries == nil {
		return nil
	}
	return r.queries.UpdateHeight(context.Background(), model.UpdateHeightParams{
		NetworkID: uint64(r.cfg.NetworkType),
		Height:    height,
	})
}

// updateBBCValsHash update bbc_val_hash of bbc_status table
func (r *Relayer) updateBBCValsHash(bbcHash []byte) error {
	if r.queries == nil {
		return nil
	}
	return r.queries.UpdateBBCValsHash(context.Background(), model.UpdateBBCValsHashParams{
		NetworkID:   uint64(r.cfg.NetworkType),
		BbcValsHash: ethcmm.Bytes2Hex(bbcHash),
	})
}

// updateBSCValsHash update sequence and bsc_val_hash of bbc_status table
func (r *Relayer) updateBSCValsHash(sequence uint64, bbcHash []byte) error {
	if r.queries == nil {
		return nil
	}
	return r.queries.UpdateBSCValsHash(context.Background(), model.UpdateBSCValsHashParams{
		NetworkID:   uint64(r.cfg.NetworkType),
		BscValsHash: ethcmm.Bytes2Hex(bbcHash),
		StakeModSeq: sequence,
	})
}

func (r *Relayer) initDB() error {
	err := r.queries.ApplySchema()
	if err != nil {
		return err
	}
	latest := r.getLatestHeight()
	err = r.initBBCStatus(latest, []byte{}, []byte{})
	if err != nil {
		return err
	}
	return nil
}
