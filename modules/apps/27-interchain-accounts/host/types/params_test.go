package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/ibc-go/v7/modules/apps/27-interchain-accounts/host/types"
)

func TestValidateParams(t *testing.T) {
	require.NoError(t, types.DefaultParams().Validate())
	require.NoError(t, types.NewParams(false, []string{}).Validate())
<<<<<<< HEAD
=======
	require.Error(t, types.NewParams(true, []string{""}).Validate())
	require.Error(t, types.NewParams(true, []string{" "}).Validate())
	require.Error(t, types.NewParams(true, []string{"*", "/cosmos.bank.v1beta1.MsgSend"}).Validate())
	require.Error(t, types.NewParams(true, make([]string, types.MaxAllowListLength+1)).Validate())
>>>>>>> 478f4c60 (imp: check length of slices of messages (#6256))
}
