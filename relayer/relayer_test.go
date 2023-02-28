package relayer

import (
	"testing"

	"github.com/celer-network/bsc-relayer/common"
	config "github.com/celer-network/bsc-relayer/config"
	"github.com/celer-network/bsc-relayer/executor"
	"github.com/celer-network/goutils/log"
)

func TestRelayer(t *testing.T) {
	log.SetLevelByName("debug")
	r, err := NewRelayer(0, &config.Config{
		CrossChainConfig: config.CrossChainConfig{1, 97},
		BBCConfig:        config.BBCConfig{[]string{"tcp://data-seed-pre-0-s1.binance.org:80"}, 500},
	})
	if err != nil {
		t.Fatalf(err.Error())
	}
	t.Logf("latest block number %d", r.getLatestHeight())
	//start := 36647109
	//end := 36647110
	//ss, _ := r.BBCExecutor.GetNextSequence(8, int64(start))
	//se, _ := r.BBCExecutor.GetNextSequence(8, int64(end))
	//t.Logf("next sequence %d %d", ss, se)
	//right := end
	//left := start
	//for left+1 != right {
	//	temp := (left + right) / 2
	//	sc, _ := r.BBCExecutor.GetNextSequence(8, int64(temp))
	//	if sc == ss {
	//		left = temp
	//	} else {
	//		right = temp
	//	}
	//	t.Logf("temp %d", temp)
	//}
	//t.Logf("stopped at %d", left)
	height := uint64(36647109)
	r.MonitorValidatorSetChange(height, []byte{}, []byte{},
		func(header *common.Header) {
			t.Logf("callback1 at height %d", header.Height)
			bs, err := header.EncodeHeader()
			if err != nil {
				t.Errorf("EncodeHeader err:%s", err.Error())
			}
			t.Logf("header bytes %x", bs)
			t.Logf("current val set hash %x", header.ValidatorSet.Hash())
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
