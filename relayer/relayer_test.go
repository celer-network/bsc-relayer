package relayer

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/celer-network/bsc-relayer/common"
	"github.com/celer-network/bsc-relayer/tendermint/light"
	_ "github.com/lib/pq"
	"github.com/tendermint/tendermint/types"

	config "github.com/celer-network/bsc-relayer/config"
	"github.com/celer-network/bsc-relayer/executor"
	"github.com/celer-network/goutils/log"
)

func genCallback1(r Relayer, t *testing.T) SyncBBCHeaderCallbackFunc {
	return func(header *common.Header) {
		t.Logf("callback1 at height %d", header.Height)
		js, _ := json.Marshal(header)
		t.Logf("header content %s", string(js))
		bz, _ := header.EncodeHeader()
		t.Logf("header bytes %x", bz)
		abs, err := light.EncodeHeader(header)
		if err != nil {
			t.Errorf("EncodeHeaderProto err:%s", err.Error())
		}
		t.Logf("any bytes %x", abs)
		err = r.UpdateAfterSync(uint64(header.Height))
		if err != nil {
			t.Errorf("UpdateAfterSync err:%s", err.Error())
		}
		vote := header.Commit.GetVote(1)
		cVote := types.CanonicalizeVote(header.ChainID, vote)
		bz = vote.SignBytes(header.ChainID)
		t.Logf("sign data len %d", len(bz))
		t.Logf("canonical vote %+v", cVote)
		bz, _ = common.Cdc.MarshalBinaryLengthPrefixed(cVote.Timestamp)
		t.Logf("timestamp data %x, len %d", bz, len(bz))
		pubkeys, sigs, signdatas := header.GetSigs()
		var a, b, c string
		for i := range pubkeys {
			a += fmt.Sprintf("%x,", pubkeys[i])
			b += fmt.Sprintf("%x,", sigs[i])
			c += fmt.Sprintf("%x,", signdatas[i])
		}
		t.Logf("pubkeys: %s", a)
		t.Logf("sigs: %s", b)
		t.Logf("signdatas: %s", c)
	}
}

func genCallback2(r Relayer, t *testing.T) RelayCrossChainPackageCallbackFunc {
	return func(pkg *executor.CrossChainPackage) {
		t.Logf("callback 2 at height %d", pkg.Height)
		t.Logf("sequence %d", pkg.Sequence)
		t.Logf("msg %x", pkg.Msg)
		t.Logf("proof %x", pkg.Proof)
		ok, set := pkg.ParseBSCValidatorSet()
		if ok {
			t.Logf("bsc validator set %v", set)
		}
	}
}

func TestMockRelayer(t *testing.T) {
	r := newMockRelayer()
	r.SetupInitialState("", []byte{1}, []byte{1})
	r.MonitorStakingModule(genCallback1(r, t), r.NewCallback2WithBSCHashCheck(genCallback2(r, t)))
}

func TestBaseRelayer(t *testing.T) {
	log.SetLevelByName("debug")
	createDatabaseIfNotExists("localhost:26257")
	dbUrl := fmt.Sprintf("postgresql://root@%s/bbc_relayer?sslmode=disable", "localhost:26257")
	db, err := sql.Open("postgres", dbUrl)
	if err != nil {
		t.Fatalf("open db, err:%s", err.Error())
	}
	r, err := newBaseRelayer(&config.Config{
		NetworkType:                  0,
		CrossChainConfig:             config.CrossChainConfig{SourceChainID: 1, DestChainID: 97},
		RpcAddrs:                     []string{"tcp://data-seed-pre-0-s1.binance.org:80"},
		SleepMillisecondForWaitBlock: 500,
	}, db)
	if err != nil {
		t.Fatalf(err.Error())
	}
	t.Logf("latest block number %d", r.getLatestHeight())
	right := r.getLatestHeight()
	sr, _ := r.BBCExecutor.GetNextSequence(8, int64(right))
	left := right
	sl := sr
	for sl == sr {
		left -= 10000
		sl, _ = r.BBCExecutor.GetNextSequence(8, int64(left))
		if sl == 0 {
			t.Fatalf("too old block height with staking module sequence 0")
		}
	}
	t.Logf("next sequence %d", sr)
	t.Logf("finding previous pkg from %d to %d", left, right)
	for left+1 != right {
		temp := (left + right) / 2
		sc, _ := r.BBCExecutor.GetNextSequence(8, int64(temp))
		if sc == sl {
			left = temp
		} else {
			right = temp
		}
	}
	t.Logf("stopped at %d", left)
	r.SetupInitialState(fmt.Sprintf("%d", left+1), []byte{1}, []byte{1})
	r.MonitorStakingModule(
		genCallback1(r, t),
		r.NewCallback2WithBSCHashCheck(
			genCallback2(r, t),
		),
	)
}

func createDatabaseIfNotExists(url string) {
	log.Infoln("sync database")
	dbUrl := fmt.Sprintf("postgresql://root@%s?sslmode=disable", url)
	log.Infof("dialing db %s", dbUrl)
	db, err := sql.Open("postgres", dbUrl)
	if err != nil {
		log.Panicf("sql open err %s", err.Error())
	}
	_, err = db.Exec("create database if not exists bbc_relayer;")
	if err != nil {
		log.Panicf("create database err %s", err.Error())
	}
}
