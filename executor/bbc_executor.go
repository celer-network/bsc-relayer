package executor

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/binance-chain/bsc-double-sign-sdk/client"
	"github.com/binance-chain/bsc-double-sign-sdk/types/bsc"
	"github.com/binance-chain/bsc-relayer/common"
	config "github.com/binance-chain/bsc-relayer/config"
	"github.com/binance-chain/go-sdk/client/rpc"
	ctypes "github.com/binance-chain/go-sdk/common/types"
	"github.com/binance-chain/go-sdk/keys"
	ethcmm "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	cmn "github.com/tendermint/tendermint/libs/common"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
	tmtypes "github.com/tendermint/tendermint/types"
)

const StakingModuleChannelID = 8

type BBCClient struct {
	BBCClient     *rpc.HTTP
	Provider      string
	CurrentHeight int64
	UpdatedAt     time.Time
}

type BBCExecutor struct {
	mutex         sync.RWMutex
	clientIdx     int
	BBCClients    []*BBCClient
	Config        *config.Config
	keyManager    keys.KeyManager
	sourceChainID common.CrossChainID
	destChainID   common.CrossChainID
}

type CrossChainPackage struct {
	Height    uint64
	ChannelID common.CrossChainChannelID
	Sequence  uint64
	Msg       []byte
	Proof     []byte
}

type PackageType uint8

const (
	StakePackageType PackageType = 0x00
	JailPackageType  PackageType = 0x01
)

type IbcValidator struct {
	ConsAddr []byte
	FeeAddr  []byte
	DistAddr []byte
	Power    uint64
}

func (validator IbcValidator) String() string {
	return fmt.Sprintf("Validator{ConsAddr:%x,FeeAddr:%x,DistAddr:%x,Power:%d}", validator.ConsAddr, validator.FeeAddr, validator.DistAddr, validator.Power)
}

type IbcValidatorSetPackage struct {
	Type         PackageType
	ValidatorSet []IbcValidator
}

func (pkg IbcValidatorSetPackage) String() string {
	valSetStr := "["
	for _, val := range pkg.ValidatorSet {
		valSetStr += val.String() + ","
	}
	valSetStr = strings.TrimSuffix(valSetStr, ",")
	valSetStr += "]"
	return fmt.Sprintf("IbcValidatorSetPackage{PackageType:%d, ValidatorSet:%s}", pkg.Type, valSetStr)
}

func (ccp *CrossChainPackage) ParseBSCValidatorSet() (bool, []ethcmm.Address) {
	expectedPkg := new(IbcValidatorSetPackage)
	// prefix length is 33 bytes(1 byte for package type + 32 bytes for relayer fee)
	err := rlp.DecodeBytes(ccp.Msg[33:], expectedPkg)
	if err != nil {
		common.Logger.Errorf("failed to decode this msg to IbcValidatorSetPackage, err:%s", err.Error())
		return false, nil
	}
	bscValidatorSet := make([]ethcmm.Address, 0)
	for _, val := range expectedPkg.ValidatorSet {
		bscValidatorSet = append(bscValidatorSet, ethcmm.BytesToAddress(val.ConsAddr))
	}
	return true, bscValidatorSet
}

func FindBscValidatorSetChangePackage(curHash []byte, pkgs []*CrossChainPackage) (bool, []byte, *CrossChainPackage) {
	for _, pkg := range pkgs {
		if ok, set := pkg.ParseBSCValidatorSet(); ok {
			sort.Slice(set, func(i, j int) bool {
				return ethcmm.Bytes2Hex(set[i].Bytes()) < ethcmm.Bytes2Hex(set[j].Bytes())
			})
			sha := sha256.New()
			for _, addr := range set {
				sha.Write(addr.Bytes())
			}
			newHash := sha.Sum(nil)
			if !bytes.Equal(curHash, newHash) {
				return true, newHash, pkg
			}
		}
	}
	return false, curHash, nil
}

func getMnemonic(cfg *config.BBCConfig) (string, error) {
	var mnemonic string
	if cfg.MnemonicType == config.KeyTypeAWSMnemonic {
		result, err := config.GetSecret(cfg.AWSSecretName, cfg.AWSRegion)
		if err != nil {
			return "", err
		}
		type AwsMnemonic struct {
			Mnemonic string `json:"mnemonic"`
		}
		var awsMnemonic AwsMnemonic
		err = json.Unmarshal([]byte(result), &awsMnemonic)
		if err != nil {
			return "", err
		}
		mnemonic = awsMnemonic.Mnemonic
	} else {
		if cfg.Mnemonic == "" {
			return "", fmt.Errorf("missing local mnemonic")
		}
		mnemonic = cfg.Mnemonic
	}
	return mnemonic, nil
}

func initBBCClients(keyManager keys.KeyManager, providers []string, network ctypes.ChainNetwork) []*BBCClient {
	bcClients := make([]*BBCClient, 0)
	for _, provider := range providers {
		rpcClient := rpc.NewRPCClient(provider, network)
		rpcClient.SetKeyManager(keyManager)
		bcClients = append(bcClients, &BBCClient{
			BBCClient: rpcClient,
			Provider:  provider,
			UpdatedAt: time.Now(),
		})
	}
	return bcClients
}

func NewBBCExecutor(cfg *config.Config, networkType ctypes.ChainNetwork) (*BBCExecutor, error) {
	var keyManager keys.KeyManager
	if len(cfg.BSCConfig.MonitorDataSeedList) >= 2 {
		mnemonic, err := getMnemonic(&cfg.BBCConfig)
		if err != nil {
			return nil, err
		}
		keyManager, err = keys.NewMnemonicKeyManager(mnemonic)
		if err != nil {
			return nil, err
		}
	}

	return &BBCExecutor{
		clientIdx:     0,
		BBCClients:    initBBCClients(keyManager, cfg.BBCConfig.RpcAddrs, networkType),
		keyManager:    keyManager,
		Config:        cfg,
		sourceChainID: common.CrossChainID(cfg.CrossChainConfig.SourceChainID),
		destChainID:   common.CrossChainID(cfg.CrossChainConfig.DestChainID),
	}, nil
}

func (executor *BBCExecutor) GetClient() *rpc.HTTP {
	executor.mutex.RLock()
	defer executor.mutex.RUnlock()
	return executor.BBCClients[executor.clientIdx].BBCClient
}

func (executor *BBCExecutor) SwitchBCClient() {
	executor.mutex.Lock()
	defer executor.mutex.Unlock()
	executor.clientIdx++
	if executor.clientIdx >= len(executor.BBCClients) {
		executor.clientIdx = 0
	}
	common.Logger.Infof("Switch to RPC endpoint: %s", executor.Config.BBCConfig.RpcAddrs[executor.clientIdx])
}

func (executor *BBCExecutor) GetLatestBlockHeight(client rpc.Client) (int64, error) {
	status, err := client.Status()
	if err != nil {
		return 0, err
	}
	return status.SyncInfo.LatestBlockHeight, nil
}

func (executor *BBCExecutor) UpdateClients() {
	for {
		common.Logger.Infof("Start to monitor bc data-seeds healthy")
		for _, bbcClient := range executor.BBCClients {
			if time.Since(bbcClient.UpdatedAt).Seconds() > DataSeedDenyServiceThreshold {
				msg := fmt.Sprintf("data seed %s is not accessable", bbcClient.Provider)
				common.Logger.Error(msg)
				config.SendTelegramMessage(executor.Config.AlertConfig.Identity, executor.Config.AlertConfig.TelegramBotId, executor.Config.AlertConfig.TelegramChatId, msg)
			}
			height, err := executor.GetLatestBlockHeight(bbcClient.BBCClient)
			if err != nil {
				common.Logger.Errorf("get latest block height error, err=%s", err.Error())
				continue
			}
			bbcClient.CurrentHeight = height
			bbcClient.UpdatedAt = time.Now()
		}
		highestHeight := int64(0)
		highestIdx := 0
		for idx := 0; idx < len(executor.BBCClients); idx++ {
			if executor.BBCClients[idx].CurrentHeight > highestHeight {
				highestHeight = executor.BBCClients[idx].CurrentHeight
				highestIdx = idx
			}
		}
		// current bbcClient block sync is fall behind, switch to the bbcClient with highest block height
		if executor.BBCClients[executor.clientIdx].CurrentHeight+FallBehindThreshold < highestHeight {
			executor.mutex.Lock()
			executor.clientIdx = highestIdx
			executor.mutex.Unlock()
		}
		time.Sleep(SleepSecondForUpdateClient * time.Second)
	}
}

func (executor *BBCExecutor) SubmitEvidence(headers []*bsc.Header) (*coretypes.ResultBroadcastTx, error) {
	return client.BSCSubmitEvidence(executor.GetClient(), executor.keyManager.GetAddr(), headers, rpc.Sync)
}

func (executor *BBCExecutor) MonitorCrossChainPackage(height int64, preValidatorsHash cmn.HexBytes) (*common.TaskSet, cmn.HexBytes, error) {
	block, err := executor.GetClient().Block(&height)
	if err != nil {
		return nil, nil, err
	}

	blockResults, err := executor.GetClient().BlockResults(&height)
	if err != nil {
		return nil, nil, err
	}

	var taskSet common.TaskSet
	taskSet.Height = uint64(height)

	var curValidatorsHash cmn.HexBytes
	if preValidatorsHash != nil {
		if !bytes.Equal(block.Block.Header.ValidatorsHash, preValidatorsHash) ||
			!bytes.Equal(block.Block.Header.ValidatorsHash, block.Block.Header.NextValidatorsHash) {
			taskSet.TaskList = append(taskSet.TaskList, common.Task{
				ChannelID: PureHeaderSyncChannelID,
			})
			curValidatorsHash = block.Block.Header.ValidatorsHash
		} else {
			curValidatorsHash = preValidatorsHash
		}
	}

	for _, event := range blockResults.Results.EndBlock.Events {
		if event.Type == CrossChainPackageEventType {
			for _, tag := range event.Attributes {
				if string(tag.Key) != CorssChainPackageInfoAttributeKey {
					continue
				}
				items := strings.Split(string(tag.Value), separator)
				if len(items) != 3 {
					continue
				}

				destChainID, err := strconv.Atoi(items[0])
				if err != nil {
					continue
				}
				if uint16(destChainID) != executor.Config.CrossChainConfig.DestChainID {
					continue
				}

				channelID, err := strconv.Atoi(items[1])
				if err != nil {
					continue
				}
				if channelID > math.MaxInt8 || channelID < 0 {
					continue
				}

				sequence, err := strconv.Atoi(items[2])
				if err != nil {
					continue
				}
				if sequence < 0 {
					continue
				}

				taskSet.TaskList = append(taskSet.TaskList, common.Task{
					ChannelID: common.CrossChainChannelID(channelID),
					Sequence:  uint64(sequence),
				})
			}
		}
	}

	return &taskSet, curValidatorsHash, nil
}

func (executor *BBCExecutor) MonitorValidatorSetChange(height int64, preValidatorsHash cmn.HexBytes) (bool, cmn.HexBytes, error) {
	validatorSetChanged := false

	block, err := executor.GetClient().Block(&height)
	if err != nil {
		return false, nil, err
	}

	var curValidatorsHash cmn.HexBytes
	if preValidatorsHash != nil {
		if !bytes.Equal(block.Block.Header.ValidatorsHash, preValidatorsHash) ||
			!bytes.Equal(block.Block.Header.ValidatorsHash, block.Block.Header.NextValidatorsHash) {
			validatorSetChanged = true
			curValidatorsHash = block.Block.Header.ValidatorsHash
		} else {
			curValidatorsHash = preValidatorsHash
		}
	}

	return validatorSetChanged, curValidatorsHash, nil
}

func (executor *BBCExecutor) GetInitConsensusState(height int64) (*common.ConsensusState, error) {
	status, err := executor.GetClient().Status()
	if err != nil {
		return nil, err
	}

	nextValHeight := height + 1
	nextValidatorSet, err := executor.GetClient().Validators(&nextValHeight)
	if err != nil {
		return nil, err
	}

	header, err := executor.GetClient().Block(&height)
	if err != nil {
		return nil, err
	}

	appHash := header.Block.Header.AppHash
	curValidatorSetHash := header.Block.Header.ValidatorsHash

	cs := &common.ConsensusState{
		ChainID:             status.NodeInfo.Network,
		Height:              uint64(height),
		AppHash:             appHash,
		CurValidatorSetHash: curValidatorSetHash,
		NextValidatorSet: &tmtypes.ValidatorSet{
			Validators: nextValidatorSet.Validators,
		},
	}
	return cs, nil
}

func (executor *BBCExecutor) QueryTendermintHeader(height int64) (*common.Header, error) {
	nextHeight := height + 1

	commit, err := executor.GetClient().Commit(&height)
	if err != nil {
		return nil, err
	}

	validators, err := executor.GetClient().Validators(&height)
	if err != nil {
		return nil, err
	}

	nextvalidators, err := executor.GetClient().Validators(&nextHeight)
	if err != nil {
		return nil, err
	}

	header := &common.Header{
		SignedHeader:     commit.SignedHeader,
		ValidatorSet:     tmtypes.NewValidatorSet(validators.Validators),
		NextValidatorSet: tmtypes.NewValidatorSet(nextvalidators.Validators),
	}

	return header, nil
}

func (executor *BBCExecutor) QueryKeyWithProof(key []byte, height int64) (int64, []byte, []byte, []byte, error) {
	opts := rpcclient.ABCIQueryOptions{
		Height: height,
		Prove:  true,
	}

	path := fmt.Sprintf("/store/%s/%s", packageStoreName, "key")
	result, err := executor.GetClient().ABCIQueryWithOptions(path, key, opts)
	if err != nil {
		return 0, nil, nil, nil, err
	}
	proofBytes, err := result.Response.Proof.Marshal()
	if err != nil {
		return 0, nil, nil, nil, err
	}

	return result.Response.Height, key, result.Response.Value, proofBytes, nil
}

func (executor *BBCExecutor) GetNextSequence(channelID common.CrossChainChannelID, height int64) (uint64, error) {
	opts := rpcclient.ABCIQueryOptions{
		Height: height,
		Prove:  false,
	}

	path := fmt.Sprintf("/store/%s/%s", sequenceStoreName, "key")
	key := buildChannelSequenceKey(executor.destChainID, channelID)

	response, err := executor.GetClient().ABCIQueryWithOptions(path, key, opts)
	if err != nil {
		return 0, err
	}
	if response.Response.Value == nil {
		return 0, nil
	}
	return binary.BigEndian.Uint64(response.Response.Value), nil

}

func (executor *BBCExecutor) GetPackage(channelID common.CrossChainChannelID, sequence, height uint64) ([]byte, []byte, error) {
	key := buildCrossChainPackageKey(executor.sourceChainID, executor.destChainID, channelID, sequence)
	var value []byte
	var proofBytes []byte
	var err error
	for i := 0; i < maxTryTimes; i++ {
		_, _, value, proofBytes, err = executor.QueryKeyWithProof(key, int64(height))
		if err != nil {
			return nil, nil, err
		}
		if len(value) == 0 {
			common.Logger.Infof("Try again to get package, channelID %d, sequence %d", channelID, sequence)
			time.Sleep(1 * time.Second) // wait 1s
		} else {
			break
		}
	}
	if len(value) == 0 {
		return nil, nil, fmt.Errorf("channelID %d, package with sequence %d is not existing", channelID, sequence)
	}

	return value, proofBytes, nil
}

func (executor *BBCExecutor) FindAllStakingModulePackages(height int64) ([]*CrossChainPackage, error) {
	blockResults, err := executor.GetClient().BlockResults(&height)
	if err != nil {
		return nil, err
	}
	packageSet := make([]*CrossChainPackage, 0)

	for i, event := range blockResults.Results.EndBlock.Events {
		if event.Type == CrossChainPackageEventType {
			common.Logger.Infof("event %d from block %d is %s", i, height, event)
			for _, tag := range event.Attributes {
				if string(tag.Key) != CorssChainPackageInfoAttributeKey {
					continue
				}
				items := strings.Split(string(tag.Value), separator)
				if len(items) != 3 {
					continue
				}

				destChainID, err := strconv.Atoi(items[0])
				if err != nil {
					continue
				}
				if uint16(destChainID) != executor.Config.CrossChainConfig.DestChainID {
					continue
				}

				channelID, err := strconv.Atoi(items[1])
				if err != nil {
					continue
				}
				if channelID > math.MaxInt8 || channelID < 0 || channelID != StakingModuleChannelID {
					continue
				}

				sequence, err := strconv.Atoi(items[2])
				if err != nil {
					continue
				}
				if sequence < 0 {
					continue
				}

				msgBytes, proofBytes, err := executor.GetPackage(common.CrossChainChannelID(channelID), uint64(sequence), uint64(height))
				if err != nil {
					common.Logger.Errorf("GetPackage channelId %d sequence %d height %d, err:%s", channelID, sequence, height, err.Error())
					return nil, err
				}

				pkg := &CrossChainPackage{
					Height:    uint64(height),
					ChannelID: common.CrossChainChannelID(channelID),
					Sequence:  uint64(sequence),
					Msg:       msgBytes,
					Proof:     proofBytes,
				}
				packageSet = append(packageSet, pkg)
			}
		}
	}

	return packageSet, nil
}

func (executor *BBCExecutor) CheckValidatorSetChange(height int64, preValidatorsHash cmn.HexBytes) (bool, cmn.HexBytes, error) {
	validatorSetChanged := false

	block, err := executor.GetClient().Block(&height)
	if err != nil {
		return false, nil, err
	}

	var curValidatorsHash cmn.HexBytes
	if preValidatorsHash != nil {
		if !bytes.Equal(block.Block.Header.ValidatorsHash, preValidatorsHash) ||
			!bytes.Equal(block.Block.Header.ValidatorsHash, block.Block.Header.NextValidatorsHash) {
			validatorSetChanged = true
			curValidatorsHash = block.Block.Header.ValidatorsHash
		} else {
			curValidatorsHash = preValidatorsHash
		}
	}

	return validatorSetChanged, curValidatorsHash, nil
}
