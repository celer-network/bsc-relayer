package relayer

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	_ "github.com/lib/pq"

	config "github.com/celer-network/bsc-relayer/config"
	"github.com/celer-network/bsc-relayer/executor"
	"github.com/celer-network/bsc-relayer/tendermint/light"
	"github.com/celer-network/goutils/log"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestRelayer(t *testing.T) {
	log.SetLevelByName("debug")
	dbUrl := fmt.Sprintf("postgresql://root@%s/bbc_relayer?sslmode=disable", "localhost:26257")
	db, err := sql.Open("postgres", dbUrl)
	if err != nil {
		t.Fatalf("open db, err:%s", err.Error())
	}
	r, err := NewRelayer(&config.Config{
		NetworkType:      0,
		CrossChainConfig: config.CrossChainConfig{1, 97},
		BBCConfig:        config.BBCConfig{[]string{"tcp://data-seed-pre-0-s1.binance.org:80"}, 500},
	}, db)
	if err != nil {
		t.Fatalf(err.Error())
	}
	t.Logf("latest block number %d", r.getLatestHeight())
	right := r.getLatestHeight()
	sr, _ := r.BBCExecutor.GetNextSequence(8, int64(right))
	left := right - 10000
	sl, _ := r.BBCExecutor.GetNextSequence(8, int64(left))
	for sl == sr {
		left -= 10000
		sl, _ = r.BBCExecutor.GetNextSequence(8, int64(left))
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
	height := left
	r.MonitorValidatorSetChange(height, []byte{1}, []byte{1},
		func(header *light.TmHeader) {
			t.Logf("callback1 at height %d", header.SignedHeader.Header.Height)
			js, _ := json.Marshal(header)
			t.Logf("header content %s", string(js))
			bs, err := proto.Marshal(header)
			if err != nil {
				t.Errorf("proto marshal err:%s", err.Error())
			}
			a := &anypb.Any{
				TypeUrl: "/tendermint.types.TmHeader",
				Value:   bs,
			}
			abs, err := proto.Marshal(a)
			if err != nil {
				t.Errorf("proto marshal err:%s", err.Error())
			}
			t.Logf("any bytes %x", abs)
			err = r.UpdateAfterSync(uint64(header.SignedHeader.Header.Height))
			if err != nil {
				t.Errorf("UpdateAfterSync err:%s", err.Error())
			}
		},
		func(pkg executor.CrossChainPackage) {
			t.Logf("callback 2 at height %d", pkg.Height)
			t.Logf("sequence %d", pkg.Sequence)
			t.Logf("msg %x", pkg.Msg)
			t.Logf("proof %x", pkg.Proof)
			ok, set := pkg.ParseBSCValidatorSet()
			if ok {
				t.Logf("bsc validator set %v", set)
			}
		})
}
