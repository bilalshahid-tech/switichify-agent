package switcher

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"

	"github.com/rs/zerolog/log"
)

type Switcher struct {
	PrimaryInterface string
	BackupInterface  string
	PrimaryGW        string
	BackupGW         string
}

func NewSwitcher(primaryInterface, backupInterface string) (*Switcher, error) {
	autoDetectInterfaces(&primaryInterface, &backupInterface)

	// Dynamically detect gateways based on interface names
	pGW, err := detectGateway(primaryInterface)
	if err != nil {
		log.Warn().Str("interface", primaryInterface).Msg("Could not detect primary gateway, using empty string")
	}

	bGW, err := detectGateway(backupInterface)
	if err != nil {
		log.Warn().Str("interface", backupInterface).Msg("Could not detect backup gateway, using empty string")
	}

	return &Switcher{
		PrimaryInterface: primaryInterface,
		BackupInterface:  backupInterface,
		PrimaryGW:        pGW,
		BackupGW:         bGW,
	}, nil
}

func detectGateway(iface string) (string, error) {
	if iface == "" {
		return "", errors.New("empty interface")
	}

	// Wait, ip route might return something like "default via 192.168.1.1 dev eth0..."
	// For Windows (the OS is Windows, but the instructions say "Linux commands: ip route change default via <backup_gateway> dev <backup_interface>").
	// I will just implement the linux command as requested by the user.
	// ip route show dev <iface> might show the default.
	// Let's implement getting the gateway via `ip route`
	cmd := exec.Command("ip", "route", "show", "dev", iface)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, "default via") {
			parts := strings.Fields(line)
			for i, p := range parts {
				if p == "via" && i+1 < len(parts) {
					return parts[i+1], nil
				}
			}
		}
	}

	return "", fmt.Errorf("no default gateway found for dev %s", iface)
}

// autoDetectInterfaces finds available interfaces if they are omitted from config
func autoDetectInterfaces(primary, backup *string) {
	if *primary != "" && *backup != "" {
		return
	}

	cmd := exec.Command("ip", "route", "show", "default")
	out, err := cmd.Output()
	var defaultIfaces []string
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			parts := strings.Fields(line)
			for i, p := range parts {
				if p == "dev" && i+1 < len(parts) {
					defaultIfaces = append(defaultIfaces, parts[i+1])
				}
			}
		}
	}

	if *primary == "" && len(defaultIfaces) > 0 {
		*primary = defaultIfaces[0]
		log.Info().Str("interface", *primary).Msg("Auto-detected primary interface")
	}

	if *backup == "" {
		// Attempt to find a second interface that is UP
		cmdLink := exec.Command("ip", "-o", "link", "show")
		outLink, errLink := cmdLink.Output()
		if errLink == nil {
			lines := strings.Split(string(outLink), "\n")
			for _, line := range lines {
				if strings.Contains(line, "UP") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						name := strings.TrimRight(parts[1], ":")
						// Skip loopback and primary
						if name != "lo" && name != *primary {
							*backup = name
							log.Info().Str("interface", *backup).Msg("Auto-detected backup interface")
							break
						}
					}
				}
			}
		}
	}
}

// -----------------------------------------------------------------------------
// Switch to backup gateway
// -----------------------------------------------------------------------------

func (s *Switcher) SwitchToBackup() error {
	if s.BackupGW == "" || s.BackupInterface == "" {
		return errors.New("backup gateway or interface is not detected/configured")
	}
	return s.changeDefaultGateway(s.BackupGW, s.BackupInterface)
}

// -----------------------------------------------------------------------------
// Switch back to primary gateway
// -----------------------------------------------------------------------------

func (s *Switcher) SwitchToPrimary() error {
	if s.PrimaryGW == "" || s.PrimaryInterface == "" {
		return errors.New("primary gateway or interface is not detected/configured")
	}
	return s.changeDefaultGateway(s.PrimaryGW, s.PrimaryInterface)
}

// -----------------------------------------------------------------------------
// Core Logic: Change Default Route using exec
// -----------------------------------------------------------------------------

func (s *Switcher) changeDefaultGateway(gw string, iface string) error {
	cmd := exec.Command("ip", "route", "change", "default", "via", gw, "dev", iface)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Error().Str("stderr", stderr.String()).Msg("failed to execute ip route change")
		return fmt.Errorf("failed changing default route: %v, stderr: %s", err, stderr.String())
	}

	log.Info().
		Str("gateway", gw).
		Str("interface", iface).
		Msg("default gateway switched")

	return nil
}

// -----------------------------------------------------------------------------
// Validate the current route
// -----------------------------------------------------------------------------

func (s *Switcher) IsUsingGateway(gw string) bool {
	cmd := exec.Command("ip", "route", "show", "default")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), gw)
}

func (s *Switcher) GetActiveGateway() string {
	cmd := exec.Command("ip", "route", "show", "default")
	out, err := cmd.Output()
	if err != nil  {
		return "unknown"
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, "default via") {
			parts := strings.Fields(line)
			for i, p := range parts {
				if p == "via" && i+1 < len(parts) {
					return parts[i+1]
				}
			}
		}
	}
	return "unknown"
}
