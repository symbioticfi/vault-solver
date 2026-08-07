package redstoneoev

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/chain"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

type unexpectedGasMulticaller struct {
	t *testing.T
}

func (m unexpectedGasMulticaller) Multicall(context.Context, []chain.Call) ([]chain.CallResult, error) {
	m.t.Fatal("gas multicall reached before token coverage validation")
	return nil, nil
}

func TestReadGasPricesValidatesConfiguredMode(t *testing.T) {
	loan := common.HexToAddress("0x1111111111111111111111111111111111111111")
	adapter := types.AdapterSnapshot{Loan: loan, LoanDecimals: 6}

	t.Run("gas omitted", func(t *testing.T) {
		prices, err := (&reader{}).ReadGasPrices(t.Context(), adapter, time.Unix(1, 0))
		if err != nil || prices != nil {
			t.Fatalf("prices = %v, error = %v; want nil, nil", prices, err)
		}
	})

	t.Run("configured feed does not cover adapter loan", func(t *testing.T) {
		feed := common.HexToAddress("0x2222222222222222222222222222222222222222")
		otherToken := common.HexToAddress("0x3333333333333333333333333333333333333333")
		oracle, err := liquidlanegas.NewOracleReader(unexpectedGasMulticaller{t}, liquidlanegas.OracleConfig{
			NativeUSDFeed: liquidlanegas.USDFeed{Address: feed, MaxAge: time.Hour},
			TokenUSDFeeds: map[common.Address]liquidlanegas.USDFeed{
				otherToken: {Address: feed, MaxAge: time.Hour},
			},
		})
		if err != nil {
			t.Fatalf("new gas reader: %v", err)
		}
		_, err = (&reader{gas: oracle}).ReadGasPrices(t.Context(), adapter, time.Unix(1, 0))
		if err == nil || !strings.Contains(err.Error(), "missing USD feed for token "+loan.Hex()) {
			t.Fatalf("error = %v, want missing adapter-loan feed", err)
		}
	})
}
