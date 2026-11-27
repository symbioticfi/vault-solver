package threef

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/vault-solver/api/bindings/3f/positionmanager"
	"github.com/symbioticfi/vault-solver/api/bindings/3f/underlyingasset"
	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/internal/parse"
)

var positionManagerABI = positionmanager.NewIThreeFPositionManager()
var underlyingAssetABI = underlyingasset.NewIThreeFUnderlyingAsset()

func verifyPolicyAccount(ctx context.Context, chain provider.Reader, policy, account common.Address, block *big.Int) error {
	raw, err := chain.CallContract(ctx, ethereum.CallMsg{To: &policy, Data: policyABI.PackAccount()}, block)
	if err != nil {
		return errors.Errorf("3f policy account read: %w", err)
	}
	owner, err := policyABI.UnpackAccount(raw)
	if err != nil {
		return errors.Errorf("3f invalid policy account: %w", err)
	}
	if owner != account || owner == (common.Address{}) {
		return errors.Errorf("3f policy %s account %s differs from executor account %s", policy.Hex(), owner.Hex(), account.Hex())
	}
	return nil
}

// API metadata supplies candidates, never authority. Native policy.consume must
// attest each request and its exact funding/underlying budget before an offer.
func discoverMarkets(ctx context.Context, chain provider.Reader, pin provider.Snapshot, policy common.Address, auctions []threef.AuctionDto, offers []threef.OfferDto) ([]mintMarket, error) {
	live := make(map[common.Address]bool)
	for _, offer := range offers {
		if !strings.EqualFold(offer.Maker, pin.Account.Hex()) {
			continue
		}
		active, err := liveStatus(offer.Status)
		if err != nil {
			return nil, err
		}
		if active {
			live[common.HexToAddress(offer.RequestId)] = true
		}
	}
	underlyings := make(map[common.Address]common.Address)
	seen := make(map[[2]common.Address]bool)
	var markets []mintMarket
	for _, auction := range auctions {
		if !strings.EqualFold(auction.Status, "OPEN") && !strings.EqualFold(auction.Status, "SOLVABLE") && !live[common.HexToAddress(auction.RequestId)] {
			continue
		}
		asset, err := depositAsset(auction)
		if err != nil {
			return nil, err
		}
		metadata := auction.Asset.Get()
		if metadata == nil {
			return nil, errors.Errorf("3f auction %v has no asset metadata to name its position manager", auction.Id)
		}
		manager, err := parse.NonZeroAddress(metadata.PositionManager, fmt.Sprintf("3f auction %v position manager", auction.Id))
		if err != nil {
			return nil, err
		}
		underlying, ok := underlyings[manager]
		if !ok {
			raw, err := chain.CallContract(ctx, ethereum.CallMsg{To: &manager, Data: positionManagerABI.PackAssets()}, pin.BlockNumber)
			if err != nil {
				return nil, errors.Errorf("3f position manager %s assets read: %w", manager.Hex(), err)
			}
			assets, err := positionManagerABI.UnpackAssets(raw)
			if err != nil {
				return nil, errors.Errorf("3f position manager %s assets: %w", manager.Hex(), err)
			}
			if assets.CollateralAsset == (common.Address{}) {
				return nil, errors.Errorf("3f position manager %s has a zero collateral asset", manager.Hex())
			}
			raw, err = chain.CallContract(ctx, ethereum.CallMsg{To: &assets.CollateralAsset, Data: underlyingAssetABI.PackUnderlying()}, pin.BlockNumber)
			if err != nil {
				return nil, errors.Errorf("3f collateral %s underlying read: %w", assets.CollateralAsset.Hex(), err)
			}
			underlying, err = underlyingAssetABI.UnpackUnderlying(raw)
			if err != nil {
				return nil, errors.Errorf("3f collateral %s underlying: %w", assets.CollateralAsset.Hex(), err)
			}
			if underlying == (common.Address{}) {
				return nil, errors.Errorf("3f collateral %s has a zero underlying", assets.CollateralAsset.Hex())
			}
			underlyings[manager] = underlying
		}
		pair := [2]common.Address{asset, underlying}
		if !seen[pair] {
			seen[pair] = true
			markets = append(markets, mintMarket{Asset: asset, UnderlyingAsset: underlying, Policy: policy})
		}
	}
	return markets, nil
}
