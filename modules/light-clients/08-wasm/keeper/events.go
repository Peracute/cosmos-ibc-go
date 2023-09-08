package keeper

import (
	"encoding/hex"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/ibc-go/modules/light-clients/08-wasm/types"
)

// emitCreateClientEvent emits a create client event
func emitStoreWasmCodeEvent(ctx sdk.Context, codeHash []byte) {
	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			types.EventTypeStoreWasmCode,
			sdk.NewAttribute(types.AttributeKeyWasmCodeHash, hex.EncodeToString(codeHash)),
		),
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
		),
	})
}
