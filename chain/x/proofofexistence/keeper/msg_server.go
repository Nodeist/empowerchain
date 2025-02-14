package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/empowerchain/empowerchain/x/proofofexistence"
)

var _ proofofexistence.MsgServer = msgServer{}

type msgServer struct {
	Keeper
}

// NewMsgServerImpl returns an implementation of the MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(keeper Keeper) proofofexistence.MsgServer {
	return &msgServer{Keeper: keeper}
}

func (k msgServer) CreateProof(goCtx context.Context, msg *proofofexistence.MsgCreateProof) (*proofofexistence.MsgCreateProofResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	acc, err := sdk.AccAddressFromBech32(msg.Creator)
	if err != nil {
		return nil, err
	}

	if err := k.CreateNewProof(ctx, msg.Hash, acc); err != nil {
		return nil, err
	}

	return &proofofexistence.MsgCreateProofResponse{}, nil
}
