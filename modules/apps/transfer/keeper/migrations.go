package keeper

import (
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
)

// Migrator is a struct for handling in-place store migrations.
type Migrator struct {
	keeper Keeper
}

// NewMigrator returns a new Migrator.
func NewMigrator(keeper Keeper) Migrator {
	return Migrator{keeper: keeper}
}

// MigrateTraces migrates the DenomTraces to the correct format, accounting for slashes in the BaseDenom.
func (m Migrator) MigrateTraces(ctx sdk.Context) error {
	// list of traces that must replace the old traces in store
	var newTraces []types.DenomTrace
	m.keeper.IterateDenomTraces(ctx,
		func(dt types.DenomTrace) (stop bool) {
			// check if the new way of splitting FullDenom
			// is the same as the current DenomTrace.
			// If it isn't then store the new DenomTrace in the list of new traces.
			newTrace := types.ParseDenomTrace(dt.GetFullDenomPath())
			err := newTrace.Validate()
			if err != nil {
				panic(err)
			}

			if dt.IBCDenom() != newTrace.IBCDenom() {
				// The new form of parsing will result in a token denomination change.
				// A bank migration is required. A panic should occur to prevent the
				// chain from using corrupted state.
				panic(fmt.Sprintf("migration will result in corrupted state. Previous IBC token (%s) requires a bank migration. Expected denom trace (%s)", dt, newTrace))
			}

			if !equalTraces(newTrace, dt) {
				newTraces = append(newTraces, newTrace)
			}
			return false
		})

	// replace the outdated traces with the new trace information
	for _, nt := range newTraces {
		m.keeper.SetDenomTrace(ctx, nt)
	}
	return nil
}

// MigrateTotalEscrowOut migrates the total escrow amount to calculate total IBC'd out.
func (m Migrator) MigrateTotalEscrowOut(ctx sdk.Context) error {
	var nativeTokens = make(map[string]int64)

	transferChannels := m.keeper.channelKeeper.GetAllChannelsWithPortPrefix(ctx, types.PortID)
	for _, channel := range transferChannels {
		escrowAddress := types.GetEscrowAddress(types.PortID, channel.ChannelId)
		getEscrowBalances := m.keeper.bankKeeper.GetAllBalances(ctx, escrowAddress)

		for _, escrowBalance := range getEscrowBalances {
			// Denom possibilities:
			// - "atom" = native denom
			// - "ibc/atom" = non native denom
			// - "atom/ibc/osmo" = native denom

			denomSplit := strings.SplitN(escrowBalance.Denom, "/", 2)
			if denomSplit[0] != "ibc" || len(denomSplit) == 1 {
				// native denom
				escrowAmount := sdk.NewInt(nativeTokens[escrowBalance.Denom]).Add(escrowBalance.Amount).Int64()
				nativeTokens[escrowBalance.Denom] = escrowAmount
			}
		}
	}

	if len(nativeTokens) != 0 {
		for denom, amount := range nativeTokens {
			m.keeper.SetIBCOutDenomAmount(ctx, denom, sdk.NewInt(amount))
		}
	}

	return nil
}

func equalTraces(dtA, dtB types.DenomTrace) bool {
	return dtA.BaseDenom == dtB.BaseDenom && dtA.Path == dtB.Path
}
