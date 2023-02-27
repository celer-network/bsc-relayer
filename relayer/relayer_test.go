package relayer

import (
	"testing"

	"github.com/binance-chain/bsc-relayer/common"
	"github.com/binance-chain/bsc-relayer/executor"
)

func TestRelayer(t *testing.T) {
	r, err := NewRelayer(0, "../.config")
	if err != nil {
		t.Fatalf(err.Error())
	}
	t.Logf("%v", r.cfg)
	//height := uint64(36498451)
	height := uint64(36498451)
	r.MonitorValidatorSetChange(height, []byte{}, []byte{}, func(header *common.Header) { t.Logf("callback1 at height %d", header.Height) },
		func(pkg executor.CrossChainPackage) { t.Logf("callback 2 at height %d", pkg.Height) })
}
