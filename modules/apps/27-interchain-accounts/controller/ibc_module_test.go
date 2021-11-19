package controller_test

import (
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	capabilitytypes "github.com/cosmos/cosmos-sdk/x/capability/types"
	"github.com/stretchr/testify/suite"
	"github.com/tendermint/tendermint/crypto"

	"github.com/cosmos/ibc-go/v2/modules/apps/27-interchain-accounts/types"
	clienttypes "github.com/cosmos/ibc-go/v2/modules/core/02-client/types"
	channeltypes "github.com/cosmos/ibc-go/v2/modules/core/04-channel/types"
	host "github.com/cosmos/ibc-go/v2/modules/core/24-host"
	ibctesting "github.com/cosmos/ibc-go/v2/testing"
)

var (
	// TestAccAddress defines a resuable bech32 address for testing purposes
	// TODO: update crypto.AddressHash() when sdk uses address.Module()
	TestAccAddress = types.GenerateAddress(sdk.AccAddress(crypto.AddressHash([]byte(types.ModuleName))), TestPortID)
	// TestOwnerAddress defines a reusable bech32 address for testing purposes
	TestOwnerAddress = "cosmos17dtl0mjt3t77kpuhg2edqzjpszulwhgzuj9ljs"
	// TestPortID defines a resuable port identifier for testing purposes
	TestPortID, _ = types.GeneratePortID(TestOwnerAddress, ibctesting.FirstConnectionID, ibctesting.FirstConnectionID)
	// TestVersion defines a resuable interchainaccounts version string for testing purposes
	TestVersion = types.NewAppVersion(types.VersionPrefix, TestAccAddress.String())
)

type InterchainAccountsTestSuite struct {
	suite.Suite

	coordinator *ibctesting.Coordinator

	// testing chains used for convenience and readability
	chainA *ibctesting.TestChain
	chainB *ibctesting.TestChain
}

func TestICATestSuite(t *testing.T) {
	suite.Run(t, new(InterchainAccountsTestSuite))
}

func (suite *InterchainAccountsTestSuite) SetupTest() {
	suite.coordinator = ibctesting.NewCoordinator(suite.T(), 2)
	suite.chainA = suite.coordinator.GetChain(ibctesting.GetChainID(0))
	suite.chainB = suite.coordinator.GetChain(ibctesting.GetChainID(1))
}

func NewICAPath(chainA, chainB *ibctesting.TestChain) *ibctesting.Path {
	path := ibctesting.NewPath(chainA, chainB)
	path.EndpointA.ChannelConfig.PortID = types.PortID
	path.EndpointB.ChannelConfig.PortID = types.PortID
	path.EndpointA.ChannelConfig.Order = channeltypes.ORDERED
	path.EndpointB.ChannelConfig.Order = channeltypes.ORDERED
	path.EndpointA.ChannelConfig.Version = types.VersionPrefix
	path.EndpointB.ChannelConfig.Version = TestVersion

	return path
}

func InitInterchainAccount(endpoint *ibctesting.Endpoint, owner string) error {
	portID, err := types.GeneratePortID(owner, endpoint.ConnectionID, endpoint.Counterparty.ConnectionID)
	if err != nil {
		return err
	}

	channelSequence := endpoint.Chain.App.GetIBCKeeper().ChannelKeeper.GetNextChannelSequence(endpoint.Chain.GetContext())

	if err := endpoint.Chain.GetSimApp().ICAControllerKeeper.InitInterchainAccount(endpoint.Chain.GetContext(), endpoint.ConnectionID, endpoint.Counterparty.ConnectionID, owner); err != nil {
		return err
	}

	// commit state changes for proof verification
	endpoint.Chain.App.Commit()
	endpoint.Chain.NextBlock()

	// update port/channel ids
	endpoint.ChannelID = channeltypes.FormatChannelIdentifier(channelSequence)
	endpoint.ChannelConfig.PortID = portID

	return nil
}

// SetupICAPath invokes the InterchainAccounts entrypoint and subsequent channel handshake handlers
func SetupICAPath(path *ibctesting.Path, owner string) error {
	if err := InitInterchainAccount(path.EndpointA, owner); err != nil {
		return err
	}

	if err := path.EndpointB.ChanOpenTry(); err != nil {
		return err
	}

	if err := path.EndpointA.ChanOpenAck(); err != nil {
		return err
	}

	if err := path.EndpointB.ChanOpenConfirm(); err != nil {
		return err
	}

	return nil
}

func (suite *InterchainAccountsTestSuite) TestOnChanOpenInit() {
	var (
		channel *channeltypes.Channel
	)

	testCases := []struct {
		name     string
		malleate func()
		expPass  bool
	}{
		{
			"success", func() {}, true,
		},
		{
			"ICA OnChanOpenInit fails - UNORDERED channel", func() {
				channel.Ordering = channeltypes.UNORDERED
			}, false,
		},
		{
			"ICA auth module callback fails", func() {
				suite.chainA.GetSimApp().ICAAuthModule.IBCApp.OnChanOpenInit = func(ctx sdk.Context, order channeltypes.Order, connectionHops []string,
					portID, channelID string, chanCap *capabilitytypes.Capability,
					counterparty channeltypes.Counterparty, version string,
				) error {
					return fmt.Errorf("mock ica auth fails")
				}
			}, false,
		},
	}

	for _, tc := range testCases {
		tc := tc

		suite.Run(tc.name, func() {
			suite.SetupTest() // reset

			path := NewICAPath(suite.chainA, suite.chainB)
			suite.coordinator.SetupConnections(path)

			// mock init interchain account
			portID, err := types.GeneratePortID(TestOwnerAddress, path.EndpointA.ConnectionID, path.EndpointB.ConnectionID)
			suite.Require().NoError(err)

			portCap := suite.chainA.GetSimApp().IBCKeeper.PortKeeper.BindPort(suite.chainA.GetContext(), portID)
			suite.chainA.GetSimApp().ICAControllerKeeper.ClaimCapability(suite.chainA.GetContext(), portCap, host.PortPath(portID))

			path.EndpointA.ChannelConfig.PortID = portID
			path.EndpointA.ChannelID = ibctesting.FirstChannelID

			// default values
			counterparty := channeltypes.NewCounterparty(path.EndpointB.ChannelConfig.PortID, path.EndpointB.ChannelID)
			channel = &channeltypes.Channel{
				State:          channeltypes.INIT,
				Ordering:       channeltypes.ORDERED,
				Counterparty:   counterparty,
				ConnectionHops: []string{path.EndpointA.ConnectionID},
				Version:        types.VersionPrefix,
			}

			tc.malleate() // malleate mutates test data

			// ensure channel on chainA is set in state
			suite.chainA.GetSimApp().IBCKeeper.ChannelKeeper.SetChannel(suite.chainA.GetContext(), path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID, *channel)

			module, _, err := suite.chainA.App.GetIBCKeeper().PortKeeper.LookupModuleByPort(suite.chainA.GetContext(), path.EndpointA.ChannelConfig.PortID)
			suite.Require().NoError(err)

			chanCap, err := suite.chainA.App.GetScopedIBCKeeper().NewCapability(suite.chainA.GetContext(), host.ChannelCapabilityPath(path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID))
			suite.Require().NoError(err)

			cbs, ok := suite.chainA.App.GetIBCKeeper().Router.GetRoute(module)
			suite.Require().True(ok)

			err = cbs.OnChanOpenInit(suite.chainA.GetContext(), channel.Ordering, channel.GetConnectionHops(),
				path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID, chanCap, channel.Counterparty, channel.GetVersion(),
			)

			if tc.expPass {
				suite.Require().NoError(err)
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

// Test initiating a ChanOpenTry using the controller chain instead of the host chain
// ChainA is the controller chain. ChainB creates a controller port as well,
// attempting to trick chainA.
// Sending a MsgChanOpenTry will never reach the application callback due to
// core IBC checks not passing, so a call to the application callback is also
// done directly.
func (suite *InterchainAccountsTestSuite) TestChanOpenTry() {
	suite.SetupTest() // reset
	path := NewICAPath(suite.chainA, suite.chainB)
	suite.coordinator.SetupConnections(path)

	err := InitInterchainAccount(path.EndpointA, TestOwnerAddress)
	suite.Require().NoError(err)

	// chainB also creates a controller port
	err = InitInterchainAccount(path.EndpointB, TestOwnerAddress)
	suite.Require().NoError(err)

	path.EndpointA.UpdateClient()
	channelKey := host.ChannelKey(path.EndpointB.ChannelConfig.PortID, path.EndpointB.ChannelID)
	proofInit, proofHeight := path.EndpointB.Chain.QueryProof(channelKey)

	// use chainA (controller) for ChanOpenTry
	msg := channeltypes.NewMsgChannelOpenTry(path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID, TestVersion, channeltypes.ORDERED, []string{path.EndpointA.ConnectionID}, path.EndpointB.ChannelConfig.PortID, path.EndpointB.ChannelID, types.VersionPrefix, proofInit, proofHeight, types.ModuleName)
	handler := suite.chainA.GetSimApp().MsgServiceRouter().Handler(msg)
	_, err = handler(suite.chainA.GetContext(), msg)

	suite.Require().Error(err)

	// call application callback directly
	module, _, err := suite.chainA.App.GetIBCKeeper().PortKeeper.LookupModuleByPort(suite.chainA.GetContext(), path.EndpointB.ChannelConfig.PortID)
	suite.Require().NoError(err)

	cbs, ok := suite.chainA.App.GetIBCKeeper().Router.GetRoute(module)
	suite.Require().True(ok)

	counterparty := channeltypes.NewCounterparty(path.EndpointB.ChannelConfig.PortID, path.EndpointB.ChannelID)
	chanCap, found := suite.chainA.App.GetScopedIBCKeeper().GetCapability(suite.chainA.GetContext(), host.ChannelCapabilityPath(path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID))
	suite.Require().True(found)

	err = cbs.OnChanOpenTry(
		suite.chainA.GetContext(), path.EndpointA.ChannelConfig.Order, []string{path.EndpointA.ConnectionID},
		path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID, chanCap,
		counterparty, path.EndpointA.ChannelConfig.Version, path.EndpointB.ChannelConfig.Version,
	)
	suite.Require().Error(err)
}

func (suite *InterchainAccountsTestSuite) TestOnChanOpenAck() {
	var (
		path *ibctesting.Path
	)

	testCases := []struct {
		name     string
		malleate func()
		expPass  bool
	}{
		{
			"success", func() {}, true,
		},
		{
			"ICA OnChanOpenACK fails - invalid version", func() {
				path.EndpointB.ChannelConfig.Version = "invalid|version"
			}, false,
		},
		{
			"ICA auth module callback fails", func() {
				suite.chainA.GetSimApp().ICAAuthModule.IBCApp.OnChanOpenAck = func(
					ctx sdk.Context, portID, channelID string, counterpartyVersion string,
				) error {
					return fmt.Errorf("mock ica auth fails")
				}
			}, false,
		},
	}

	for _, tc := range testCases {
		tc := tc

		suite.Run(tc.name, func() {
			suite.SetupTest() // reset

			path = NewICAPath(suite.chainA, suite.chainB)
			suite.coordinator.SetupConnections(path)

			err := InitInterchainAccount(path.EndpointA, TestOwnerAddress)
			suite.Require().NoError(err)

			err = path.EndpointB.ChanOpenTry()
			suite.Require().NoError(err)

			tc.malleate() // malleate mutates test data

			module, _, err := suite.chainA.App.GetIBCKeeper().PortKeeper.LookupModuleByPort(suite.chainA.GetContext(), path.EndpointA.ChannelConfig.PortID)
			suite.Require().NoError(err)

			cbs, ok := suite.chainA.App.GetIBCKeeper().Router.GetRoute(module)
			suite.Require().True(ok)

			err = cbs.OnChanOpenAck(suite.chainA.GetContext(), path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID, path.EndpointB.ChannelConfig.Version)

			if tc.expPass {
				suite.Require().NoError(err)
			} else {
				suite.Require().Error(err)
			}

		})
	}

}

// Test initiating a ChanOpenConfirm using the controller chain instead of the host chain
// ChainA is the controller chain. ChainB is the host chain
// Sending a MsgChanOpenConfirm will never reach the application callback due to
// core IBC checks not passing, so a call to the application callback is also
// done directly.
func (suite *InterchainAccountsTestSuite) TestChanOpenConfirm() {
	suite.SetupTest() // reset
	path := NewICAPath(suite.chainA, suite.chainB)
	suite.coordinator.SetupConnections(path)

	err := InitInterchainAccount(path.EndpointA, TestOwnerAddress)
	suite.Require().NoError(err)

	err = path.EndpointB.ChanOpenTry()
	suite.Require().NoError(err)

	// chainB maliciously sets channel to OPEN
	channel := channeltypes.NewChannel(channeltypes.OPEN, channeltypes.ORDERED, channeltypes.NewCounterparty(path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID), []string{path.EndpointB.ConnectionID}, TestVersion)
	suite.chainB.GetSimApp().GetIBCKeeper().ChannelKeeper.SetChannel(suite.chainB.GetContext(), path.EndpointB.ChannelConfig.PortID, path.EndpointB.ChannelID, channel)

	// commit state changes so proof can be created
	suite.chainB.App.Commit()
	suite.chainB.NextBlock()

	path.EndpointA.UpdateClient()

	// query proof from ChainB
	channelKey := host.ChannelKey(path.EndpointB.ChannelConfig.PortID, path.EndpointB.ChannelID)
	proofAck, proofHeight := path.EndpointB.Chain.QueryProof(channelKey)

	// use chainA (controller) for ChanOpenConfirm
	msg := channeltypes.NewMsgChannelOpenConfirm(path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID, proofAck, proofHeight, types.ModuleName)
	handler := suite.chainA.GetSimApp().MsgServiceRouter().Handler(msg)
	_, err = handler(suite.chainA.GetContext(), msg)

	suite.Require().Error(err)

	// call application callback directly
	module, _, err := suite.chainA.App.GetIBCKeeper().PortKeeper.LookupModuleByPort(suite.chainA.GetContext(), path.EndpointA.ChannelConfig.PortID)
	suite.Require().NoError(err)

	cbs, ok := suite.chainA.App.GetIBCKeeper().Router.GetRoute(module)
	suite.Require().True(ok)

	err = cbs.OnChanOpenConfirm(
		suite.chainA.GetContext(), path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID,
	)
	suite.Require().Error(err)

}

// OnChanCloseInit on controller (chainA)
func (suite *InterchainAccountsTestSuite) TestOnChanCloseInit() {
	path := NewICAPath(suite.chainA, suite.chainB)
	suite.coordinator.SetupConnections(path)

	err := SetupICAPath(path, TestOwnerAddress)
	suite.Require().NoError(err)

	module, _, err := suite.chainA.App.GetIBCKeeper().PortKeeper.LookupModuleByPort(suite.chainA.GetContext(), path.EndpointA.ChannelConfig.PortID)
	suite.Require().NoError(err)

	cbs, ok := suite.chainA.App.GetIBCKeeper().Router.GetRoute(module)
	suite.Require().True(ok)

	err = cbs.OnChanCloseInit(
		suite.chainA.GetContext(), path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID,
	)

	suite.Require().Error(err)
}

func (suite *InterchainAccountsTestSuite) TestOnChanCloseConfirm() {
	var (
		path *ibctesting.Path
	)

	testCases := []struct {
		name     string
		malleate func()
		expPass  bool
	}{

		{
			"success", func() {}, true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.SetupTest() // reset

			path = NewICAPath(suite.chainA, suite.chainB)
			suite.coordinator.SetupConnections(path)

			err := SetupICAPath(path, TestOwnerAddress)
			suite.Require().NoError(err)

			tc.malleate() // malleate mutates test data
			module, _, err := suite.chainA.App.GetIBCKeeper().PortKeeper.LookupModuleByPort(suite.chainA.GetContext(), path.EndpointA.ChannelConfig.PortID)
			suite.Require().NoError(err)

			cbs, ok := suite.chainA.App.GetIBCKeeper().Router.GetRoute(module)
			suite.Require().True(ok)

			err = cbs.OnChanCloseConfirm(
				suite.chainA.GetContext(), path.EndpointA.ChannelConfig.PortID, path.EndpointA.ChannelID)

			activeChannelID, found := suite.chainA.GetSimApp().ICAHostKeeper.GetActiveChannelID(suite.chainA.GetContext(), path.EndpointA.ChannelConfig.PortID)

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().False(found)
				suite.Require().Empty(activeChannelID)
			} else {
				suite.Require().Error(err)
			}

		})
	}
}

func (suite *InterchainAccountsTestSuite) TestOnRecvPacket() {

	testCases := []struct {
		name     string
		malleate func()
		expPass  bool
	}{
		{
			"ICA OnRecvPacket fails with ErrInvalidChannelFlow", func() {}, false,
		},
	}

	for _, tc := range testCases {
		tc := tc

		suite.Run(tc.name, func() {
			suite.SetupTest() // reset

			path := NewICAPath(suite.chainA, suite.chainB)
			suite.coordinator.SetupConnections(path)

			err := SetupICAPath(path, TestOwnerAddress)
			suite.Require().NoError(err)

			tc.malleate() // malleate mutates test data

			module, _, err := suite.chainB.App.GetIBCKeeper().PortKeeper.LookupModuleByPort(suite.chainB.GetContext(), path.EndpointB.ChannelConfig.PortID)
			suite.Require().NoError(err)

			cbs, ok := suite.chainB.App.GetIBCKeeper().Router.GetRoute(module)
			suite.Require().True(ok)

			packet := channeltypes.NewPacket(
				[]byte("empty packet data"),
				suite.chainA.SenderAccount.GetSequence(),
				path.EndpointB.ChannelConfig.PortID,
				path.EndpointB.ChannelID,
				path.EndpointA.ChannelConfig.PortID,
				path.EndpointA.ChannelID,
				clienttypes.NewHeight(0, 100),
				0,
			)

			ack := cbs.OnRecvPacket(suite.chainA.GetContext(), packet, TestAccAddress)
			suite.Require().Equal(tc.expPass, ack.Success())
		})
	}
}

func (suite *InterchainAccountsTestSuite) TestNegotiateAppVersion() {
	var (
		proposedVersion string
	)
	testCases := []struct {
		name     string
		malleate func()
		expPass  bool
	}{
		{
			"ICA OnRecvPacket fails with ErrInvalidChannelFlow", func() {}, false,
		},
	}

	for _, tc := range testCases {
		tc := tc

		suite.Run(tc.name, func() {
			suite.SetupTest()
			path := NewICAPath(suite.chainA, suite.chainB)
			suite.coordinator.SetupConnections(path)

			err := InitInterchainAccount(path.EndpointA, TestOwnerAddress)
			suite.Require().NoError(err)

			module, _, err := suite.chainA.GetSimApp().GetIBCKeeper().PortKeeper.LookupModuleByPort(suite.chainA.GetContext(), path.EndpointA.ChannelConfig.PortID)
			suite.Require().NoError(err)

			cbs, ok := suite.chainA.GetSimApp().GetIBCKeeper().Router.GetRoute(module)
			suite.Require().True(ok)

			counterpartyPortID, err := types.GeneratePortID(TestOwnerAddress, path.EndpointA.ConnectionID, path.EndpointB.ConnectionID)
			suite.Require().NoError(err)

			counterparty := channeltypes.Counterparty{
				PortId:    counterpartyPortID,
				ChannelId: path.EndpointB.ChannelID,
			}

			proposedVersion = types.VersionPrefix

			tc.malleate()

			version, err := cbs.NegotiateAppVersion(suite.chainA.GetContext(), channeltypes.ORDERED, path.EndpointA.ConnectionID, path.EndpointA.ChannelConfig.PortID, counterparty, proposedVersion)
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().NoError(types.ValidateVersion(version))
				suite.Require().Equal(TestVersion, version)
			} else {
				suite.Require().Error(err)
				suite.Require().Empty(version)
			}
		})
	}
}
