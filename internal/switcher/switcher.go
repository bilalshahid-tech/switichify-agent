package switcher

import (
	"bytes"
	"errors"
	"fmt"
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

	pGW, err := detectGatewayRobust(primaryInterface)
	if err != nil {
		log.Warn().Str("interface", primaryInterface).Msg("Could not detect primary gateway explicitly")
	}

	bGW, err := detectGatewayRobust(backupInterface)
	if err != nil {
		log.Warn().Str("interface", backupInterface).Msg("Could not detect backup gateway explicitly")
	}

	return &Switcher{
		PrimaryInterface: primaryInterface,
		BackupInterface:  backupInterface,
		PrimaryGW:        pGW,
		BackupGW:         bGW,
	}, nil
}

// detectGatewayRobust explicitly requires gateway per dev
func detectGatewayRobust(iface string) (string, error) {
	if iface == "" {
		return "", errors.New("empty interface")
	}

	cmd := exec.Command("sudo", "ip", "route", "show", "dev", iface)
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
	return "", fmt.Errorf("no explicitly defined gateway found for dev %s", iface)
}

func autoDetectInterfaces(primary, backup *string) {
	if *primary != "" && *backup != "" {
		return
	}

	cmd := exec.Command("sudo", "ip", "route", "show", "default")
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
		cmdLink := exec.Command("sudo", "ip", "-o", "link", "show")
		outLink, errLink := cmdLink.Output()
		if errLink == nil {
			lines := strings.Split(string(outLink), "\n")
			for _, line := range lines {
				if strings.Contains(line, "UP") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						name := strings.TrimRight(parts[1], ":")
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

func (s *Switcher) SwitchToBackup() error {
	if s.BackupGW == "" {
		gw, err := detectGatewayRobust(s.BackupInterface)
		if err == nil {
			s.BackupGW = gw
		}
	}
	if s.BackupGW == "" || s.BackupInterface == "" {
		return errors.New("backup gateway or interface is not configured")
	}
	return s.changeDefaultGateway(s.BackupGW, s.BackupInterface)
}

func (s *Switcher) SwitchToPrimary() error {
	if s.PrimaryGW == "" {
		gw, err := detectGatewayRobust(s.PrimaryInterface)
		if err == nil {
			s.PrimaryGW = gw
		}
	}
	if s.PrimaryGW == "" || s.PrimaryInterface == "" {
		return errors.New("primary gateway or interface is not configured")
	}
	return s.changeDefaultGateway(s.PrimaryGW, s.PrimaryInterface)
}

func (s *Switcher) changeDefaultGateway(gw string, iface string) error {
	cmd := exec.Command("sudo", "ip", "route", "replace", "default", "via", gw, "dev", iface)
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

func (s *Switcher) IsUsingGateway(gw string) bool {
	cmd := exec.Command("sudo", "ip", "route", "show", "default")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), gw)
}

func (s *Switcher) GetActiveGateway() string {
	cmd := exec.Command("sudo", "ip", "route", "show", "default")
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

// VerifyTrafficFlow performs external HTTP check to ensure live route success 
func (s *Switcher) VerifyTrafficFlow() string {
	cmd := exec.Command("curl", "-s", "--max-time", "5", "ifconfig.me/ip")
	out, err := cmd.Output()
	if err != nil {
		return "failed/timeout"
	}
	return strings.TrimSpace(string(out))
}
