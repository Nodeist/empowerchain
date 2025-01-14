package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/empowerchain/empowerchain/x/plasticcredit"
)

func (k Keeper) GetIDCounters(ctx sdk.Context) (plasticcredit.IDCounters, error) {
	store := ctx.KVStore(k.storeKey)
	bz := store.Get(plasticcredit.IDCountersKey)
	if bz == nil {
		return plasticcredit.IDCounters{}, nil
	}

	var idc plasticcredit.IDCounters
	err := k.cdc.Unmarshal(bz, &idc)
	return idc, err
}

func (k Keeper) setIDCounters(ctx sdk.Context, idc plasticcredit.IDCounters) error {
	if err := idc.Validate(); err != nil {
		return err
	}

	store := ctx.KVStore(k.storeKey)
	bz, err := k.cdc.Marshal(&idc)
	if err != nil {
		return err
	}
	store.Set(plasticcredit.IDCountersKey, bz)

	return nil
}
