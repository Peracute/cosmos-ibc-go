package types_test

import (
<<<<<<< HEAD:modules/light-clients/06-solomachine/types/header_test.go
	"github.com/cosmos/ibc-go/v5/modules/core/exported"
	"github.com/cosmos/ibc-go/v5/modules/light-clients/06-solomachine/types"
	ibctesting "github.com/cosmos/ibc-go/v5/testing"
=======
	"github.com/cosmos/ibc-go/v6/modules/core/exported"
	solomachine "github.com/cosmos/ibc-go/v6/modules/light-clients/06-solomachine"
	ibctesting "github.com/cosmos/ibc-go/v6/testing"
>>>>>>> c86d27f (chore: increment go mod to v6 (#2318)):modules/light-clients/06-solomachine/header_test.go
)

func (suite *SoloMachineTestSuite) TestHeaderValidateBasic() {
	// test singlesig and multisig public keys
	for _, solomachine := range []*ibctesting.Solomachine{suite.solomachine, suite.solomachineMulti} {

		header := solomachine.CreateHeader()

		cases := []struct {
			name    string
			header  *types.Header
			expPass bool
		}{
			{
				"valid header",
				header,
				true,
			},
			{
				"sequence is zero",
				&types.Header{
					Sequence:       0,
					Timestamp:      header.Timestamp,
					Signature:      header.Signature,
					NewPublicKey:   header.NewPublicKey,
					NewDiversifier: header.NewDiversifier,
				},
				false,
			},
			{
				"timestamp is zero",
				&types.Header{
					Sequence:       header.Sequence,
					Timestamp:      0,
					Signature:      header.Signature,
					NewPublicKey:   header.NewPublicKey,
					NewDiversifier: header.NewDiversifier,
				},
				false,
			},
			{
				"signature is empty",
				&types.Header{
					Sequence:       header.Sequence,
					Timestamp:      header.Timestamp,
					Signature:      []byte{},
					NewPublicKey:   header.NewPublicKey,
					NewDiversifier: header.NewDiversifier,
				},
				false,
			},
			{
				"diversifier contains only spaces",
				&types.Header{
					Sequence:       header.Sequence,
					Timestamp:      header.Timestamp,
					Signature:      header.Signature,
					NewPublicKey:   header.NewPublicKey,
					NewDiversifier: " ",
				},
				false,
			},
			{
				"public key is nil",
				&types.Header{
					Sequence:       header.Sequence,
					Timestamp:      header.Timestamp,
					Signature:      header.Signature,
					NewPublicKey:   nil,
					NewDiversifier: header.NewDiversifier,
				},
				false,
			},
		}

		suite.Require().Equal(exported.Solomachine, header.ClientType())

		for _, tc := range cases {
			tc := tc

			suite.Run(tc.name, func() {
				err := tc.header.ValidateBasic()

				if tc.expPass {
					suite.Require().NoError(err)
				} else {
					suite.Require().Error(err)
				}
			})
		}
	}
}
