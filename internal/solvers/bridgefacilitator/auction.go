package bridgefacilitator

import (
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/threef"
)

// auction is the solver's immutable projection of one API row. Optional quote
// inputs stay absent until the API resolves them; signing fields are checked later.
type auction struct {
	id                        int64
	request, asset            common.Address
	status                    string
	amount                    *big.Int
	maxRate                   *float64
	domainName, domainVersion string
	domainChain               *big.Int
	solveStart                time.Time
}

func parseAuction(row threef.AuctionDto) (auction, error) {
	if row.Id <= 0 || math.IsNaN(float64(row.Id)) || math.IsInf(float64(row.Id), 0) || float64(row.Id) >= math.MaxInt64 {
		return auction{}, errors.Errorf("%w: id", errRequiredFieldMissing)
	}
	if strings.TrimSpace(row.Status) == "" {
		return auction{}, errors.Errorf("%w: status", errRequiredFieldMissing)
	}
	if !common.IsHexAddress(row.RequestId) {
		return auction{}, errors.Errorf("%w: requestId", errRequiredFieldMissing)
	}
	a := auction{id: int64(row.Id), request: common.HexToAddress(row.RequestId), status: row.Status, domainVersion: OfferDomainVersion}
	if value, ok := row.GetAmountRequestedOk(); ok && value != nil {
		a.amount, _ = new(big.Int).SetString(*value, 10)
	}
	if value, ok := row.GetMaxRateOk(); ok && value != nil {
		rate := float64(*value)
		if !math.IsNaN(rate) && !math.IsInf(rate, 0) {
			a.maxRate = &rate
		}
	}
	if asset, ok := row.GetDepositAssetOk(); ok && asset != nil && common.IsHexAddress(asset.GetAddress()) {
		a.asset = common.HexToAddress(asset.GetAddress())
	}
	if domain, ok := row.GetEip712DomainOk(); ok && domain != nil {
		a.domainName = domain.GetName()
		if version := domain.GetVersion(); version != "" {
			a.domainVersion = version
		}
		chainID := float64(domain.GetChainId())
		if chainID > 0 && chainID < math.MaxInt64 && math.Trunc(chainID) == chainID {
			a.domainChain = big.NewInt(int64(chainID))
		}
	}
	a.solveStart, _ = time.Parse(time.RFC3339, row.GetSolveStartTime())
	return a, nil
}

func (s *Solver) validAuctions(rows []threef.AuctionDto) []auction {
	out := make([]auction, 0, len(rows))
	for _, row := range rows {
		a, err := parseAuction(row)
		if err != nil {
			s.log.Error(err, "discover: skipping auction", "auctionId", row.Id)
			continue
		}
		out = append(out, a)
	}
	return out
}

func (a auction) quotable() bool {
	return (strings.EqualFold(a.status, "open") || strings.EqualFold(a.status, "solvable")) &&
		a.request != (common.Address{}) && a.asset != (common.Address{}) && a.amount != nil && a.amount.Sign() > 0 && a.maxRate != nil
}
