package spf

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMechanism(t *testing.T) {

	t.Run("unknown mechanism returns an error", func(t *testing.T) {
		mechanism, err := parseMechanism(fmt.Sprintf("%s", AMechanismType))
		require.ErrorContains(t, err, fmt.Sprintf("unsupported mechanism: %s", string(AMechanismType)))
		require.Nil(t, mechanism)
	})

	t.Run("empty string returns error", func(t *testing.T) {
		mechanism, err := parseMechanism("")
		require.ErrorContains(t, err, "mechanism can not be an empty string")
		require.Nil(t, mechanism)
	})

	t.Run("all mechanism parses correctly", func(t *testing.T) {
		mechanism, err := parseMechanism(string(AllMechanismType))
		require.NoError(t, err)
		require.NotNil(t, mechanism)
		require.Equal(t, AllMechanismType, mechanism.mechanismType)
	})

	t.Run("ip4 mechanism", func(t *testing.T) {
		t.Run("parses ip without cidr correctly", func(t *testing.T) {
			ip4 := net.IPv4(192, 0, 0, 128)
			mechanism, err := parseMechanism(fmt.Sprintf("%s:%s", IP4MechanismType, ip4))
			require.NoError(t, err)
			require.NotNil(t, mechanism)
			require.Equal(t, IP4MechanismType, mechanism.mechanismType)
			require.Equal(t, ip4.String(), *mechanism.value)
		})

		t.Run("fails if not a valid ip without cidr", func(t *testing.T) {
			mechanism, err := parseMechanism(fmt.Sprintf("%s:%s", IP4MechanismType, "192.0.0."))
			require.ErrorContains(t, err, "failed to parse ip4")
			require.Nil(t, mechanism)
		})

		t.Run("parses ip with cidr correctly", func(t *testing.T) {
			_, ipNet, err := net.ParseCIDR("192.0.0.1/24")
			require.NoError(t, err)
			mechanism, err := parseMechanism(fmt.Sprintf("%s:%s", IP4MechanismType, ipNet.String()))
			require.NoError(t, err)
			require.NotNil(t, mechanism)
			require.Equal(t, IP4MechanismType, mechanism.mechanismType)
			require.Equal(t, ipNet.String(), *mechanism.value)
		})

		t.Run("fails if missing semicolon", func(t *testing.T) {
			mechanism, err := parseMechanism(string(IP4MechanismType))
			require.ErrorContains(t, err, `ip4 mechanism requires a value separated by ":"`)
			require.Nil(t, mechanism)
		})
	})

	t.Run("ip6 mechanism", func(t *testing.T) {
		t.Run("parses ip without cidr correctly", func(t *testing.T) {
			ip6 := net.IPv6interfacelocalallnodes
			mechanism, err := parseMechanism(fmt.Sprintf("%s:%s", IP6MechanismType, ip6))
			require.NoError(t, err)
			require.NotNil(t, mechanism)
			require.Equal(t, IP6MechanismType, mechanism.mechanismType)
			require.Equal(t, ip6.String(), *mechanism.value)
		})

		t.Run("fails if not a valid ip without cidr", func(t *testing.T) {
			mechanism, err := parseMechanism(fmt.Sprintf("%s:%s", IP6MechanismType, net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}))
			require.ErrorContains(t, err, "failed to parse ip6")
			require.Nil(t, mechanism)
		})

		t.Run("parses ip with cidr correctly", func(t *testing.T) {
			_, ipNet, err := net.ParseCIDR(net.IPv6interfacelocalallnodes.String() + "/128")
			require.NoError(t, err)
			mechanism, err := parseMechanism(fmt.Sprintf("%s:%s", IP6MechanismType, ipNet.String()))
			require.NoError(t, err)
			require.NotNil(t, mechanism)
			require.Equal(t, IP6MechanismType, mechanism.mechanismType)
			require.Equal(t, ipNet.String(), *mechanism.value)
		})

		t.Run("fails if missing semicolon", func(t *testing.T) {
			mechanism, err := parseMechanism(string(IP6MechanismType))
			require.ErrorContains(t, err, `ip6 mechanism requires a value separated by ":"`)
			require.Nil(t, mechanism)
		})
	})
}
