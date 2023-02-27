package relayer

import (
	"testing"

	"github.com/celer-network/bsc-relayer/common"
	config "github.com/celer-network/bsc-relayer/config"
	"github.com/celer-network/bsc-relayer/executor"
)

func TestRelayer(t *testing.T) {
	r, err := NewRelayer(0, &config.Config{
		CrossChainConfig: config.CrossChainConfig{1, 97},
		BBCConfig:        config.BBCConfig{[]string{"tcp://data-seed-pre-0-s1.binance.org:80"}, 500},
	})
	if err != nil {
		t.Fatalf(err.Error())
	}
	//height := uint64(36498451)
	r.MonitorValidatorSetChange(0, []byte{}, []byte{}, func(header *common.Header) { t.Logf("callback1 at height %d", header.Height) },
		func(pkg executor.CrossChainPackage) { t.Logf("callback 2 at height %d", pkg.Height) })
}
