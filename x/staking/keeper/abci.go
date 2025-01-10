package keeper

import (
	"context"
	"cosmossdk.io/math"

	abci "github.com/cometbft/cometbft/abci/types"

	"github.com/cosmos/cosmos-sdk/telemetry"
	"github.com/cosmos/cosmos-sdk/x/staking/types"
)

// BeginBlocker will persist the current header and validator set as a historical entry
// and prune the oldest entry based on the HistoricalEntries parameter
func (k *Keeper) BeginBlocker(ctx context.Context) error {
	defer telemetry.ModuleMeasureSince(types.ModuleName, telemetry.Now(), telemetry.MetricKeyBeginBlocker)
	return k.TrackHistoricalInfo(ctx)
}

// EndBlocker called at every block, update validator set
func (k *Keeper) EndBlocker(ctx context.Context) ([]abci.ValidatorUpdate, error) {
	defer telemetry.ModuleMeasureSince(types.ModuleName, telemetry.Now(), telemetry.MetricKeyEndBlocker)
	allValidators, err := k.GetAllValidators(ctx)
	if err != nil {
		return nil, err
	}
	updates := make([]abci.ValidatorUpdate, 0, len(allValidators))
	for _, validator := range allValidators {
		update := validator.ABCIValidatorUpdate(math.NewInt(1))
		if update.Power == 0 {
			k.Logger(ctx).Info("Validator has no power, skipping update", "validator", update)
			continue
		}
		updates = append(updates, update)
		k.Logger(ctx).Info("UpdateValidator:", "pubKey", update.PubKey, "power", update.Power)
	}
	return updates, nil
}
