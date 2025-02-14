package keeper

import (
	"cosmossdk.io/errors"
	"github.com/cosmos/cosmos-sdk/store/prefix"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/empowerchain/empowerchain/x/plasticcredit"
)

func (k Keeper) GetCreditCollection(ctx sdk.Context, denom string) (plasticcredit.CreditCollection, bool) {
	store := k.getCreditCollectionStore(ctx)

	key := plasticcredit.CreateKeyFromString(denom)
	bz := store.Get(key)
	if bz == nil {
		return plasticcredit.CreditCollection{}, false
	}
	var creditCollection plasticcredit.CreditCollection
	k.cdc.MustUnmarshal(bz, &creditCollection)

	return creditCollection, true
}

func (k Keeper) GetCreditBalance(ctx sdk.Context, owner sdk.AccAddress, denom string) (plasticcredit.CreditBalance, bool) {
	store := k.getCreditBalanceStore(ctx)

	key := plasticcredit.CreateCreditBalanceKey(owner, denom)
	bz := store.Get(key)
	if bz == nil {
		return plasticcredit.CreditBalance{}, false
	}
	var creditBalance plasticcredit.CreditBalance
	k.cdc.MustUnmarshal(bz, &creditBalance)

	return creditBalance, true
}

func (k Keeper) retireCreditsForAddress(ctx sdk.Context, owner sdk.AccAddress, denom string, amount uint64) (*plasticcredit.CreditBalance, error) {
	creditBalance, found := k.GetCreditBalance(ctx, owner, denom)
	if !found {
		return &plasticcredit.CreditBalance{}, errors.Wrapf(plasticcredit.ErrNotEnoughCredits, "user %s doesn't have credits of denom %s", owner.String(), denom)
	}
	if creditBalance.Balance.Active < amount {
		return &plasticcredit.CreditBalance{}, errors.Wrapf(plasticcredit.ErrNotEnoughActiveCredits, "user %s has only %d credits", owner.String(), amount)
	}
	creditBalance.Balance.Active -= amount
	creditBalance.Balance.Retired += amount

	err := k.setCreditBalance(ctx, creditBalance)
	if err != nil {
		return &plasticcredit.CreditBalance{}, err
	}

	err = k.retireCreditsInCollection(ctx, denom, amount)
	if err != nil {
		return &plasticcredit.CreditBalance{}, err
	}
	return &creditBalance, nil
}

func (k Keeper) retireCreditsInCollection(ctx sdk.Context, denom string, amount uint64) error {
	creditCollection, found := k.GetCreditCollection(ctx, denom)
	if !found {
		return errors.Wrapf(sdkerrors.ErrNotFound, "credit collection with denom %s not found", denom)
	}
	if creditCollection.TotalAmount.Active < amount {
		return errors.Wrapf(plasticcredit.ErrNotEnoughActiveCredits, "only %d credits active for denom %s", amount, denom)
	}
	creditCollection.TotalAmount.Active -= amount
	creditCollection.TotalAmount.Retired += amount

	err := k.setCreditCollection(ctx, creditCollection)
	if err != nil {
		return err
	}
	// TODO event retire
	return nil
}

func (k Keeper) transferCredits(ctx sdk.Context, denom string, from sdk.AccAddress, to sdk.AccAddress, amount uint64, retire bool) error {
	balanceSender, found := k.GetCreditBalance(ctx, from, denom)
	if !found {
		return errors.Wrapf(sdkerrors.ErrNotFound, "no balance for denom %s and address %s found", denom, from)
	}
	// Check if sender has enough credits
	if balanceSender.Balance.Active < amount {
		return errors.Wrapf(plasticcredit.ErrNotEnoughCredits, "sender only has %d credits of denom %s", balanceSender.Balance.Active, denom)
	}

	balanceRecipient, found := k.GetCreditBalance(ctx, to, denom)
	if !found {
		balanceRecipient = plasticcredit.CreditBalance{
			Owner: to.String(),
			Denom: denom,
			Balance: &plasticcredit.CreditAmount{
				Active:  0,
				Retired: 0,
			},
		}
	}
	balanceSender.Balance.Active -= amount
	// If retiring credits, retire and update retired balance, if not, update active balance
	if retire {
		err := k.retireCreditsInCollection(ctx, denom, amount)
		if err != nil {
			return err
		}
		balanceRecipient.Balance.Retired += amount
	} else {
		balanceRecipient.Balance.Active += amount
	}
	err := k.setCreditBalance(ctx, balanceSender)
	if err != nil {
		return err
	}
	err = k.setCreditBalance(ctx, balanceRecipient)
	if err != nil {
		return err
	}
	// TODO event transfer
	return nil
}

func (k Keeper) issueCredits(ctx sdk.Context, creator string, projectID uint64, serialNumber string, amount uint64, data []*plasticcredit.ProvenData) (collection plasticcredit.CreditCollection, err error) {
	project, found := k.GetProject(ctx, projectID)
	if !found {
		return plasticcredit.CreditCollection{}, errors.Wrapf(sdkerrors.ErrNotFound, "project with id %d not found", projectID)
	}

	creditClass, found := k.GetCreditClass(ctx, project.CreditClassId)
	if !found {
		return plasticcredit.CreditCollection{}, errors.Wrapf(sdkerrors.ErrNotFound, "credit class with id %d not found", project.CreditClassId)
	}

	issuer, found := k.GetIssuer(ctx, creditClass.IssuerId)
	if !found {
		return plasticcredit.CreditCollection{}, errors.Wrapf(sdkerrors.ErrNotFound, "issuer with id %d not found", creditClass.IssuerId)
	}
	// Check if creator is issuer admin
	if issuer.Admin != creator {
		return plasticcredit.CreditCollection{}, errors.Wrapf(plasticcredit.ErrNotIssuer, "%s is not allowed to issue credits for class with id %s", creator, creditClass.CreditClassId)
	}

	applicant, found := k.GetApplicant(ctx, project.ApplicantId)
	if !found {
		return plasticcredit.CreditCollection{}, errors.Wrapf(sdkerrors.ErrNotFound, "applicant with id %d not found", project.ApplicantId)
	}

	denom := CreateCreditDenom(creditClass.CreditClassId, serialNumber)
	creditCollection, found := k.GetCreditCollection(ctx, denom)
	// If collection doesn't exist, create a new one
	if !found {
		creditCollection = plasticcredit.CreditCollection{
			ProjectId: projectID,
			TotalAmount: &plasticcredit.CreditAmount{
				Active:  amount,
				Retired: 0,
			},
			CreditData: data,
		}
	} else {
		// If collection already exists, add new credits to the total and append new data if present
		creditCollection.TotalAmount.Active += amount
		if data != nil {
			creditCollection.CreditData = append(creditCollection.CreditData, data...)
		}
	}

	err = k.setCreditCollection(ctx, creditCollection)
	if err != nil {
		return plasticcredit.CreditCollection{}, err
	}

	var recipient sdk.AccAddress
	recipient, err = sdk.AccAddressFromBech32(applicant.Admin)
	if err != nil {
		return plasticcredit.CreditCollection{}, err
	}

	recipientBalance, found := k.GetCreditBalance(ctx, recipient, denom)
	// Recipient doesn't have any credits from collection
	if !found {
		recipientBalance = plasticcredit.CreditBalance{
			Owner: recipient.String(),
			Denom: denom,
			Balance: &plasticcredit.CreditAmount{
				Active:  amount,
				Retired: 0,
			},
		}
	} else {
		// Recipient already have credits from collection
		recipientBalance.Balance.Active += amount
	}

	err = k.setCreditBalance(ctx, recipientBalance)
	if err != nil {
		return plasticcredit.CreditCollection{}, err
	}
	// TODO event issued credits
	return creditCollection, nil
}

func (k Keeper) setCreditCollection(ctx sdk.Context, creditCollection plasticcredit.CreditCollection) error {
	store := k.getCreditCollectionStore(ctx)

	b, err := k.cdc.Marshal(&creditCollection)
	if err != nil {
		return err
	}

	key := plasticcredit.CreateKeyFromString(creditCollection.Denom)
	store.Set(key, b)

	return nil
}

func (k Keeper) setCreditBalance(ctx sdk.Context, balance plasticcredit.CreditBalance) error {
	store := k.getCreditBalanceStore(ctx)

	b, err := k.cdc.Marshal(&balance)
	if err != nil {
		return err
	}
	owner, err := sdk.AccAddressFromBech32(balance.Owner)
	if err != nil {
		return errors.Wrapf(sdkerrors.ErrInvalidAddress, "invalid owner address (%s)", err)
	}
	key := plasticcredit.CreateCreditBalanceKey(owner, balance.Denom)
	store.Set(key, b)

	return nil
}

func (k Keeper) getAllCreditCollections(ctx sdk.Context) []*plasticcredit.CreditCollection {
	store := k.getCreditCollectionStore(ctx)

	iterator := store.Iterator(nil, nil)
	defer iterator.Close()

	var creditCollections []*plasticcredit.CreditCollection
	for ; iterator.Valid(); iterator.Next() {
		var creditCollection plasticcredit.CreditCollection
		k.cdc.MustUnmarshal(iterator.Value(), &creditCollection)
		creditCollections = append(creditCollections, &creditCollection)
	}
	return creditCollections
}

func (k Keeper) getAllCreditBalances(ctx sdk.Context) []*plasticcredit.CreditBalance {
	store := k.getCreditBalanceStore(ctx)

	iterator := store.Iterator(nil, nil)
	defer iterator.Close()

	var creditBalances []*plasticcredit.CreditBalance
	for ; iterator.Valid(); iterator.Next() {
		var creditBalance plasticcredit.CreditBalance
		k.cdc.MustUnmarshal(iterator.Value(), &creditBalance)
		creditBalances = append(creditBalances, &creditBalance)
	}
	return creditBalances
}

func (k Keeper) getCreditCollectionStore(ctx sdk.Context) storetypes.KVStore {
	store := ctx.KVStore(k.storeKey)
	creditCollectionStore := prefix.NewStore(store, plasticcredit.CreditCollectionKey)

	return creditCollectionStore
}

func (k Keeper) getCreditBalanceStore(ctx sdk.Context) storetypes.KVStore {
	store := ctx.KVStore(k.storeKey)
	creditBalanceStore := prefix.NewStore(store, plasticcredit.CreditBalanceKey)

	return creditBalanceStore
}

func CreateCreditDenom(creditClassDenom string, creditCollectionSuffix string) string {
	return creditClassDenom + "/" + creditCollectionSuffix
}
