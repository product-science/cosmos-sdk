package keeper

import (
	"context"

	"cosmossdk.io/math"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/staking/types"
)

type ComputeResult struct {
	Power           int64
	ValidatorPubKey cryptotypes.PubKey
	OperatorAddress string
}

func (k Keeper) SetComputeValidators(ctx context.Context, computeResults []ComputeResult) ([]types.Validator, error) {
	server := NewMsgServerImpl(&k)
	logger := k.Logger(ctx)
	resultsMap := make(map[string]ComputeResult)
	totalBonded := 0
	for _, result := range computeResults {
		resultsMap[result.ValidatorPubKey.String()] = result
	}

	currentValidators, err := k.GetAllValidators(ctx)
	if err != nil {
		logger.Error("error getting current validators", "error", err.Error())
		return nil, err
	}

	validatorsAlreadyExisting := make(map[string]bool)
	for _, validator := range currentValidators {
		conPubKey, err := validator.ConsPubKey()
		if err != nil {
			logger.Error("Error getting cons pubkey", "error", err.Error())
			return nil, err
		}
		validatorsAlreadyExisting[conPubKey.String()] = true
	}

	// Handle validators already in
	for _, validator := range currentValidators {
		conPubKey, err := validator.ConsPubKey()
		if err != nil {
			logger.Error("Error getting cons pubkey", "error", err.Error())
			return nil, err
		}
		computeResult, inNewResults := resultsMap[conPubKey.String()]
		if inNewResults {
			logger.Info("Updating validator", "operator", validator.GetOperator(), "power", computeResult.Power)
			err := k.setValidatorPower(ctx, validator, computeResult.Power)
			if err != nil {
				return nil, err
			}
			totalBonded += int(computeResult.Power)
		} else {
			logger.Info("Removing validator", "operator", validator.GetOperator(), "power", computeResult.Power)
			err := k.setValidatorPower(ctx, validator, 0)
			if err != nil {
				return nil, err
			}
		}
	}
	// Handle validators not in
	for _, computeResult := range computeResults {
		if computeResult.Power == 0 {
			logger.Warn("Power is 0 for new validator, skipping validator", "address", computeResult.OperatorAddress, "key", computeResult.ValidatorPubKey.String())
			continue
		}
		if _, ok := validatorsAlreadyExisting[computeResult.ValidatorPubKey.String()]; !ok {
			logger.Info("Creating validator", "power", computeResult, "operator", computeResult.OperatorAddress)
			newVal, err := k.createValidator(ctx, computeResult, server)
			if err != nil {
				logger.Error("Error creating validator", "error", err.Error())
				return nil, err
			}
			err = k.delegateResult(ctx, computeResult, newVal.OperatorAddress)
			if err != nil {
				logger.Error("Error delegating result", "error", err.Error())
				return nil, err
			}
			totalBonded += int(computeResult.Power)
		}
	}
	return k.GetAllValidators(ctx)
}

func (k Keeper) setValidatorPower(ctx context.Context, validator types.Validator, power int64) error {
	logger := k.Logger(ctx)
	err := k.DeleteValidatorByPowerIndex(ctx, validator)
	if err != nil {
		logger.Error("Error deleting validator by power index", "error", err.Error())
		return err
	}

	validator.Tokens = math.NewInt(power)
	err = k.SetValidator(ctx, validator)
	if err != nil {
		logger.Error("Error setting validator", "error", err.Error())
		return err
	}
	err = k.SetValidatorByPowerIndex(ctx, validator)
	if err != nil {
		logger.Error("Error setting validator by power index", "error", err.Error())
		return err
	}
	return nil
}

func (k Keeper) delegateResult(ctx context.Context, computeResult ComputeResult, validatorAddress string) error {
	delegation := types.Delegation{
		DelegatorAddress: computeResult.OperatorAddress,
		ValidatorAddress: validatorAddress,
	}
	delegation.Shares = math.LegacyNewDec(computeResult.Power)
	err := k.SetDelegation(ctx, delegation)
	if err != nil {
		k.Logger(ctx).Error("Error setting delegation", "error", err.Error())
		return err
	}
	return err
}

func (k Keeper) createValidator(ctx context.Context, computeResult ComputeResult, server types.MsgServer) (*types.Validator, error) {
	logger := k.Logger(ctx)
	newValAddr, err := sdk.ValAddressFromBech32(computeResult.OperatorAddress)

	if err != nil {
		logger.Error("Error converting operator address to val address", "error", err.Error())
		return nil, err
	}
	denom, err := k.BondDenom(ctx)
	if err != nil {
		return nil, err
	}

	createValidatorMsg, err := types.NewMsgCreateValidator(
		newValAddr.String(),
		computeResult.ValidatorPubKey,
		sdk.NewCoin(denom, math.NewInt(computeResult.Power)),
		types.Description{
			Moniker: newValAddr.String(),
			Details: "Created after Proof of Compute",
		},

		types.CommissionRates{
			Rate:          math.LegacyMustNewDecFromStr("0.1"),
			MaxRate:       math.LegacyMustNewDecFromStr("0.2"),
			MaxChangeRate: math.LegacyMustNewDecFromStr("0.01"),
		},
		math.NewInt(1),
	)
	if err != nil {
		logger.Error("Error creating validator message", "error", err.Error())
		return nil, err
	}
	_, err = server.CreateValidator(ctx, createValidatorMsg)
	if err != nil {
		logger.Error("Error creating validator", "error", err.Error())
		return nil, err
	}
	newVal, err := k.GetValidator(ctx, newValAddr)
	if err != nil {
		logger.Error("Error getting created validator", "error", err.Error())
		return nil, err
	}
	logger.Info("Created validator", "validator", newVal.String())
	bondedVal, err := k.bondValidator(ctx, newVal)
	if err != nil {
		logger.Error("Error bonding validator", "error", err.Error())
		return nil, err
	}
	logger.Info("Bonded validator", "validator", bondedVal.String())
	return &bondedVal, nil
}
