package keeper

import (
	"math/big"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
)

// CalculateBaseFee calculates the base fee for the current block. If the NoBaseFee parameter is
// enabled or below activation height, this function returns nil.
//
// NOTE: This code is inspired from the go-ethereum EIP1559 implementation and adapted to
// Cosmos SDK-based chains.
func (k Keeper) CalculateBaseFee(ctx sdk.Context) *big.Int {
	params := k.GetParams(ctx)

	// Ignore the calculation if not enabled.
	if !params.IsBaseFeeEnabled(ctx.BlockHeight()) {
		return nil
	}

	consParams := ctx.ConsensusParams()

	// If the current block is the first EIP-1559 block, return the base fee
	// defined in the parameters.
	if ctx.BlockHeight() == params.EnableHeight {
		return params.BaseFee.BigInt()
	}

	parentBaseFee := params.BaseFee.BigInt()
	if parentBaseFee == nil {
		return nil
	}

	parentGasUsed := k.GetBlockGasWanted(ctx)

	// gasLimit is initialized to the MaxUint64 and updated only if MaxGas is > -1. If MaxGas is
	// equal to -1 means that block gas is unlimited.
	blockGasLimit := new(big.Int).SetUint64(math.MaxUint64)
	if consParams != nil && consParams.Block != nil && consParams.Block.MaxGas > -1 {
		blockGasLimit = big.NewInt(consParams.Block.MaxGas)
	}

	// CONTRACT: ElasticityMultiplier cannot be 0 as it's checked in the params
	// validation
	parentGasTargetBig := new(big.Int).Div(blockGasLimit, new(big.Int).SetUint64(uint64(params.ElasticityMultiplier)))
	if !parentGasTargetBig.IsUint64() {
		return nil
	}

	parentGasTarget := parentGasTargetBig.Uint64()

	// If the parent gasUsed is the same as the target, the baseFee remains
	// unchanged.
	if parentGasUsed == parentGasTarget {
		return new(big.Int).Set(parentBaseFee)
	}

    baseFeeChangeDenominator := new(big.Int).SetUint64(uint64(params.BaseFeeChangeDenominator))

    // If the parent block used more gas than its target, the baseFee should
    // increase.
	if parentGasUsed > parentGasTarget {
		gasUsedDelta := new(big.Int).SetUint64(parentGasUsed - parentGasTarget)
		x := new(big.Int).Mul(parentBaseFee, gasUsedDelta)
		y := x.Div(x, parentGasTargetBig)
		baseFeeDelta := math.BigMax(
			x.Div(y, baseFeeChangeDenominator),
			common.Big1,
		)

		return x.Add(parentBaseFee, baseFeeDelta)
	}

	// Otherwise if the parent block used less gas than its target, the baseFee
	// should decrease.
	gasUsedDelta := new(big.Int).SetUint64(parentGasTarget - parentGasUsed)
	x := new(big.Int).Mul(parentBaseFee, gasUsedDelta)
	y := x.Div(x, parentGasTargetBig)
	baseFeeDelta := x.Div(y, baseFeeChangeDenominator)

	// Set global min gas price as lower bound of the base fee, transactions below
	// the min gas price don't even reach the mempool.
	minGasPrice := params.MinGasPrice.TruncateInt().BigInt()
	return math.BigMax(x.Sub(parentBaseFee, baseFeeDelta), minGasPrice)
}
