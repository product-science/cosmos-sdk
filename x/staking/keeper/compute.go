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
			// update to new power
			validator.Tokens = math.NewInt(computeResult.Power)
			err := k.SetValidator(ctx, validator)
			if err != nil {
				return nil, err
			}
		} else {
			logger.Info("Removing validator", "operator", validator.GetOperator(), "power", computeResult.Power)
			// update to new power
			validator.Tokens = math.NewInt(0)
			err := k.SetValidator(ctx, validator)
			if err != nil {
				return nil, err
			}
		}
	}

	// Handle validators not in
	for _, computeResult := range computeResults {
		if _, ok := validatorsAlreadyExisting[computeResult.ValidatorPubKey.String()]; !ok {
			logger.Info("Creating validator", "power", computeResult, "operator", computeResult.OperatorAddress)
			_, err := k.createValidator(ctx, computeResult, server)
			if err != nil {
				logger.Error("Error creating validator", "error", err.Error())
				return nil, err
			}
		}
	}
	return k.GetAllValidators(ctx)
}

func (k Keeper) createValidator(ctx context.Context, computeResult ComputeResult, server types.MsgServer) (*types.Validator, error) {
	logger := k.Logger(ctx)
	s := computeResult.ValidatorPubKey.Address().String()
	newValAddr, err := sdk.ValAddressFromHex(s)

	if err != nil {
		logger.Error("Error converting pubkey to val address", "error", err.Error())
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
			Moniker: computeResult.OperatorAddress,
			Details: "Created after Proof of Compute",
		},

		types.CommissionRates{
			Rate:          math.LegacyMustNewDecFromStr("0.1"),
			MaxRate:       math.LegacyMustNewDecFromStr("0.2"),
			MaxChangeRate: math.LegacyMustNewDecFromStr("0.01"),
		},
		math.NewInt(1),
	)
	// I think the Go fanatics are off their rocker. This is a lot of boilerplate.
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
