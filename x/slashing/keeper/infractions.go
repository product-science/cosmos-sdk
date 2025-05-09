package keeper

import (
	"context"
	"fmt"

	"github.com/cockroachdb/errors"

	"cosmossdk.io/core/comet"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/slashing/types"
)

// HandleValidatorSignature handles a validator signature, must be called once per validator per block.
func (k Keeper) HandleValidatorSignature(ctx context.Context, addr cryptotypes.Address, power int64, signed comet.BlockIDFlag) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	logger := k.Logger(ctx)
	height := sdkCtx.BlockHeight()

	// fetch the validator public key
	consAddr := sdk.ConsAddress(addr)

	// don't update missed blocks when validator's jailed
	isJailed, err := k.sk.IsValidatorJailed(ctx, consAddr)
	if err != nil {
		return err
	}

	if isJailed {
		return nil
	}

	// fetch signing info
	signInfo, err := k.GetValidatorSigningInfo(ctx, consAddr)
	if err != nil {
		return err
	}

	signedBlocksWindow, err := k.SignedBlocksWindow(ctx)
	if err != nil {
		return err
	}

	// Compute the relative index, so we count the blocks the validator *should*
	// have signed. We will use the 0-value default signing info if not present,
	// except for start height. The index is in the range [0, SignedBlocksWindow)
	// and is used to see if a validator signed a block at the given height, which
	// is represented by a bit in the bitmap.
	index := signInfo.IndexOffset % signedBlocksWindow
	signInfo.IndexOffset++

	// determine if the validator signed the previous block
	previous, err := k.GetMissedBlockBitmapValue(ctx, consAddr, index)
	if err != nil {
		return errors.Wrap(err, "failed to get the validator's bitmap value")
	}

	missed := signed == comet.BlockIDFlagAbsent
	switch {
	case !previous && missed:
		// Bitmap value has changed from not missed to missed, so we flip the bit
		// and increment the counter.
		if err := k.SetMissedBlockBitmapValue(ctx, consAddr, index, true); err != nil {
			return err
		}

		signInfo.MissedBlocksCounter++

	case previous && !missed:
		// Bitmap value has changed from missed to not missed, so we flip the bit
		// and decrement the counter.
		if err := k.SetMissedBlockBitmapValue(ctx, consAddr, index, false); err != nil {
			return err
		}

		signInfo.MissedBlocksCounter--

	default:
		// bitmap value at this index has not changed, no need to update counter
	}

	minSignedPerWindow, err := k.MinSignedPerWindow(ctx)
	if err != nil {
		return err
	}

	if missed {
		sdkCtx.EventManager().EmitEvent(
			sdk.NewEvent(
				types.EventTypeLiveness,
				sdk.NewAttribute(types.AttributeKeyAddress, consAddr.String()),
				sdk.NewAttribute(types.AttributeKeyMissedBlocks, fmt.Sprintf("%d", signInfo.MissedBlocksCounter)),
				sdk.NewAttribute(types.AttributeKeyHeight, fmt.Sprintf("%d", height)),
			),
		)

		logger.Debug(
			"absent validator",
			"height", height,
			"validator", consAddr.String(),
			"missed", signInfo.MissedBlocksCounter,
			"threshold", minSignedPerWindow,
		)
	}
	// Set the updated signing info
	return k.SetValidatorSigningInfo(ctx, consAddr, signInfo)
}
