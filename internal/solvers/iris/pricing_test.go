package iris

import (
	"math/big"
	"testing"
	"time"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum/common"
)

func TestRatePicksTheMostSpecificRule(t *testing.T) {
	weth, usdc, usdt := common.HexToAddress("0x1"), common.HexToAddress("0x2"), common.HexToAddress("0x3")
	venue := uint64(1)
	pricing := Pricing{
		Default: big.NewInt(8e16),
		Rules: []RateRule{
			{DebtToken: &usdc, FixedRate: big.NewInt(7e16)},
			{CollateralToken: &weth, DebtToken: &usdc, FixedRate: big.NewInt(6e16)},
			{CollateralToken: &weth, DebtToken: &usdc, VenueID: &venue, FixedRate: big.NewInt(5e16)},
		},
	}
	tests := []struct {
		name   string
		market Market
		want   int64
	}{
		{"default", Market{CollateralToken: weth, DebtToken: usdt}, 8e16},
		{"debt only", Market{CollateralToken: usdt, DebtToken: usdc}, 7e16},
		{"pair", Market{CollateralToken: weth, DebtToken: usdc, VenueID: 2}, 6e16},
		{"pair and venue", Market{CollateralToken: weth, DebtToken: usdc, VenueID: 1}, 5e16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			equal(t, big.NewInt(tt.want), pricing.rate(tt.market))
		})
	}
	ok(t, Pricing{Rules: pricing.Rules[1:2]}.rate(Market{DebtToken: usdt}) == nil)
}

func TestBondBufferRoundsUp(t *testing.T) {
	equal(t, big.NewInt(1000), Pricing{}.bond(big.NewInt(1000)))
	equal(t, big.NewInt(1050), Pricing{BondBufferBps: 500}.bond(big.NewInt(1000)))
	equal(t, big.NewInt(2), Pricing{BondBufferBps: 1}.bond(big.NewInt(1)))
}

func TestScheduleTargetsTheFirstReachableSlot(t *testing.T) {
	timing := Timing{BlockTime: 12 * time.Second, SubmitLead: 2 * time.Second, QuoteTTL: 120 * time.Second}
	head := uint64(1_800_000_000)
	tests := []struct {
		name          string
		now           uint64
		wantInclusion uint64
	}{
		{"next slot", head + 1, head + 12},
		{"next slot is too close", head + 11, head + 24},
		{"head is stale", head + 40, head + 48},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inclusion, deadline := timing.schedule(time.Unix(int64(tt.now), 0), head)
			equal(t, tt.wantInclusion, inclusion)
			equal(t, tt.wantInclusion+120, deadline)
		})
	}
	timing.ResponseSlack = 2 * time.Second
	_, deadline := timing.schedule(time.Unix(int64(head+1), 0), head)
	equal(t, head+12+122, deadline)
}

func TestFreshAcceptsOnlyTheFinalWindow(t *testing.T) {
	timing := Timing{QuoteTTL: 120 * time.Second, DeadlineWindow: 5 * time.Second}
	now := time.Unix(1_800_000_000, 0)
	tests := []struct {
		deadline int64
		want     bool
	}{
		{now.Unix() + 120, true},
		{now.Unix() + 115, true},
		{now.Unix() + 114, false},
		{now.Unix() + 121, false},
	}
	for _, tt := range tests {
		equal(t, tt.want, timing.fresh(now, uint64(tt.deadline)), "deadline now+%d", tt.deadline-now.Unix())
	}
}

func TestSelectMarketTakesTheLowestRateUnderTheCeiling(t *testing.T) {
	a := Market{Executor: common.HexToAddress("0xA"), VenueID: 0}
	b := Market{Executor: common.HexToAddress("0xB"), VenueID: 1}
	venueA, venueB := uint64(0), uint64(1)
	s := &Solver{cfg: Config{Pricing: Pricing{Rules: []RateRule{
		{VenueID: &venueA, FixedRate: big.NewInt(7e16)},
		{VenueID: &venueB, FixedRate: big.NewInt(6e16)},
	}}}}

	m, rate, err := s.selectMarket([]Market{a, b}, big.NewInt(1e17))
	must(t, err)
	equal(t, b.Executor, m.Executor)
	equal(t, big.NewInt(6e16), rate)

	m, _, err = s.selectMarket([]Market{a, b}, big.NewInt(65e15))
	must(t, err)
	equal(t, b.Executor, m.Executor)

	_, _, err = s.selectMarket([]Market{a, b}, big.NewInt(5e16))
	var refused *declined
	ok(t, errors.As(err, &refused), err)
	equal(t, "rate_above_max_fixed_rate", refused.reason)
	equal(t, big.NewInt(6e16), refused.rate)

	_, _, err = s.selectMarket(nil, big.NewInt(1e17))
	ok(t, errors.As(err, &refused), err)
	equal(t, "no_market", refused.reason)
}
