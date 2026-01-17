package switcher

import (
	"errors"
	"net"

	"github.com/rs/zerolog/log"
	"github.com/vishvananda/netlink"
)

type Switcher struct {
	PrimaryGW net.IP
	BackupGW  net.IP
}

func NewSwitcher(primary, backup string) (*Switcher, error) {
	pIP := net.ParseIP(primary)
	bIP := net.ParseIP(backup)

	if pIP == nil || bIP == nil {
		return nil, errors.New("invalid gateway IP")
	}

	return &Switcher{
		PrimaryGW: pIP,
		BackupGW:  bIP,
	}, nil
}

// -----------------------------------------------------------------------------
// Read Current Default Route
// -----------------------------------------------------------------------------

func GetCurrentDefaultRoute() (*netlink.Route, error) {
	routes, err := netlink.RouteList(nil, netlink.FAMILY_V4)
	if err != nil {
		return nil, err
	}

	for _, r := range routes {
		if r.Dst == nil { // means default route: 0.0.0.0/0
			return &r, nil
		}
	}
	return nil, errors.New("no default route found")
}

// -----------------------------------------------------------------------------
// Switch to backup gateway
// -----------------------------------------------------------------------------

func (s *Switcher) SwitchToBackup() error {
	return s.changeDefaultGateway(s.BackupGW)
}

// -----------------------------------------------------------------------------
// Switch back to primary gateway
// -----------------------------------------------------------------------------

func (s *Switcher) SwitchToPrimary() error {
	return s.changeDefaultGateway(s.PrimaryGW)
}

// -----------------------------------------------------------------------------
// Core Logic: Change Default Route
// -----------------------------------------------------------------------------

func (s *Switcher) changeDefaultGateway(gw net.IP) error {
	current, err := GetCurrentDefaultRoute()
	if err != nil {
		return err
	}

	// Remove existing default route
	if err := netlink.RouteDel(current); err != nil {
		log.Error().Err(err).Msg("failed removing existing default route")
		return err
	}

	// Add new default route
	newRoute := netlink.Route{
		Gw: gw,
	}

	if err := netlink.RouteAdd(&newRoute); err != nil {
		log.Error().Err(err).Msg("failed adding new default route")
		return err
	}

	log.Info().
		Str("gateway", gw.String()).
		Msg("default gateway switched")

	return nil
}

// -----------------------------------------------------------------------------
// Validate the current route
// -----------------------------------------------------------------------------

func (s *Switcher) IsUsingGateway(gw net.IP) bool {
	r, err := GetCurrentDefaultRoute()
	if err != nil {
		return false
	}
	return r.Gw.Equal(gw)
}
