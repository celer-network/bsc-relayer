package relayer

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/binance-chain/go-sdk/common/types"
	"github.com/celer-network/bsc-relayer/common"
	config "github.com/celer-network/bsc-relayer/config"
	"github.com/celer-network/bsc-relayer/executor"
	"github.com/celer-network/bsc-relayer/model"
	"github.com/celer-network/goutils/log"
	ethcmm "github.com/ethereum/go-ethereum/common"
)

type Relayer interface {
	NewCallback2WithBSCHashCheck(callback RelayCrossChainPackageCallbackFunc) RelayCrossChainPackageCallbackFunc
	SetupInitialState(height string, bbcHash []byte, bscHash []byte)
	MonitorStakingModule(callback1 SyncBBCHeaderCallbackFunc, callback2 RelayCrossChainPackageCallbackFunc)
	GetBBCStatus() (model.BbcStatus, error)
	UpdateAfterSync(height uint64) error
}

type baseRelayer struct {
	queries          *model.Queries
	cfg              *config.Config
	height           uint64
	bbcHash, bscHash []byte
	BBCExecutor      *executor.BBCExecutor
}

var _ Relayer = &baseRelayer{}

func NewRelayer(cfg *config.Config, db model.DBTX) (Relayer, error) {
	return newBaseRelayer(cfg, db)
}

func newBaseRelayer(cfg *config.Config, db model.DBTX) (*baseRelayer, error) {
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

	relayer := &baseRelayer{
		queries:     model.New(db),
		cfg:         cfg,
		BBCExecutor: bbcExecutor,
	}

	err = relayer.initFromDB()
	if err != nil {
		return nil, err
	}

	return relayer, nil
}

type SyncBBCHeaderCallbackFunc func(header *common.Header)

type RelayCrossChainPackageCallbackFunc func(pkg *executor.CrossChainPackage)

func (r *baseRelayer) NewCallback2WithBSCHashCheck(callback RelayCrossChainPackageCallbackFunc) RelayCrossChainPackageCallbackFunc {
	return func(pkg *executor.CrossChainPackage) {
		bscChanged, newBscHash, _ := executor.FindBscValidatorSetChangePackage(r.bscHash, []*executor.CrossChainPackage{pkg})
		if bscChanged {
			r.bscHash = newBscHash
			callback(pkg)
		}
	}
}

func (r *baseRelayer) SetupInitialState(height string, bbcHash, bscHash []byte) {
	if height == "latest" {
		r.height = r.getLatestHeight()
	}
	if number, err := strconv.Atoi(height); err == nil {
		r.height = uint64(number)
	}
	if len(bbcHash) != 0 {
		r.bbcHash = bbcHash
	}
	if len(bscHash) != 0 {
		r.bscHash = bscHash
	}
}

func (r *baseRelayer) MonitorStakingModule(callback1 SyncBBCHeaderCallbackFunc, callback2 RelayCrossChainPackageCallbackFunc) {
	height := r.height
	bbcHash := r.bbcHash
	if height == 0 {
		log.Errorf("MonitorStakingModule starts at height 0, which is not expected.")
		return
	}

	log.Infof("Start monitor all packages in channel 8 from height %d, current bbc vals hash %x, bsc vals hash %x",
		height, bbcHash, r.bscHash)
	advance := false
	bbcChanged := false
	var err error
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
		pkgs, err := r.BBCExecutor.FindAllStakingModulePackages(int64(height))
		if err != nil {
			log.Errorf("FindAllStakingModulePackages err:%s", err.Error())
			continue
		}

		// get second bbc header
		SecondHeader, err = r.BBCExecutor.QueryTendermintHeader(int64(height) + 1)
		if err != nil {
			log.Errorf("QueryTendermintHeader err:%s", err.Error())
			continue
		}

		// after gotten all data, trigger callback function
		if bbcChanged {
			callback1(firstHeader)
			err = r.updateBBCValsHash(bbcHash)
			if err != nil {
				log.Errorf("UpdateBBCValsHash into db, err:%s", err.Error())
			}
		}
		if len(pkgs) != 0 {
			callback1(SecondHeader)
			for _, pkg := range pkgs {
				callback2(pkg)
				err = r.updateBSCValsHash(pkg.Sequence, r.bscHash)
				if err != nil {
					log.Errorf("UpdateBSCValsHash into db, err:%s", err.Error())
				}
			}
		}
		advance = true
	}
}

func (r *baseRelayer) waitForNextBlock(height uint64, advance bool) (uint64, bool) {
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

func (r *baseRelayer) getLatestHeight() uint64 {
	abciInfo, err := r.BBCExecutor.GetClient().ABCIInfo()
	if err != nil {
		log.Errorf("Query latest height error: %s", err.Error())
		return 0
	}
	return uint64(abciInfo.Response.LastBlockHeight)
}

func (r *baseRelayer) GetBBCStatus() (model.BbcStatus, error) {
	if r.queries.IsNil() {
		return model.BbcStatus{}, nil
	}
	return r.queries.GetBBCStatus(context.Background(), uint64(r.cfg.NetworkType))
}

func (r *baseRelayer) UpdateAfterSync(height uint64) error {
	if r.queries.IsNil() {
		return nil
	}
	return r.queries.UpdateAfterSync(context.Background(), model.UpdateAfterSyncParams{
		NetworkID: uint64(r.cfg.NetworkType),
		SyncedAt:  height,
	})
}

// initBBCStatus insert a row into bbc_status table when relayer starts for the first time
func (r *baseRelayer) initBBCStatus(height uint64, bbcHash, bscHash []byte) error {
	if r.queries.IsNil() {
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
func (r *baseRelayer) updateHeight(height uint64) error {
	if r.queries.IsNil() {
		return nil
	}
	return r.queries.UpdateHeight(context.Background(), model.UpdateHeightParams{
		NetworkID: uint64(r.cfg.NetworkType),
		Height:    height,
	})
}

// updateBBCValsHash update bbc_val_hash of bbc_status table
func (r *baseRelayer) updateBBCValsHash(bbcHash []byte) error {
	if r.queries.IsNil() {
		return nil
	}
	return r.queries.UpdateBBCValsHash(context.Background(), model.UpdateBBCValsHashParams{
		NetworkID:   uint64(r.cfg.NetworkType),
		BbcValsHash: ethcmm.Bytes2Hex(bbcHash),
	})
}

// updateBSCValsHash update sequence and bsc_val_hash of bbc_status table
func (r *baseRelayer) updateBSCValsHash(sequence uint64, bbcHash []byte) error {
	if r.queries.IsNil() {
		return nil
	}
	return r.queries.UpdateBSCValsHash(context.Background(), model.UpdateBSCValsHashParams{
		NetworkID:   uint64(r.cfg.NetworkType),
		BscValsHash: ethcmm.Bytes2Hex(bbcHash),
		StakeModSeq: sequence,
	})
}

func (r *baseRelayer) initFromDB() error {
	if r.queries.IsNil() {
		r.bbcHash = make([]byte, 0)
		r.bscHash = make([]byte, 0)
		return nil
	}
	err := r.queries.ApplySchema()
	if err != nil {
		return err
	}
	latest := r.getLatestHeight()
	err = r.initBBCStatus(latest, []byte{}, []byte{})
	if err != nil {
		return err
	}
	status, err := r.GetBBCStatus()
	if err != nil {
		return err
	}
	r.height = status.Height + 1
	r.bbcHash = ethcmm.Hex2Bytes(status.BbcValsHash)
	r.bscHash = ethcmm.Hex2Bytes(status.BscValsHash)
	return nil
}
