package demo

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/bilal/switchify-agent/internal/switcher"
)

// GetPublicIP shells out to curl ifconfig.me to get the external IP.
func GetPublicIP() (string, error) {
	cmd := exec.Command("curl", "-s", "ifconfig.me")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// PrintActiveState is for demo support
func PrintActiveState(sw *switcher.Switcher) {
	fmt.Printf("\n=== Agent State Details ===\n")
	ip, err := GetPublicIP()
	if err != nil {
		fmt.Printf("Public IP: <Error: %v>\n", err)
	} else {
		fmt.Printf("Public IP: %s\n", ip)
	}

	activeGw := sw.GetActiveGateway()
	
	activeIf := "Unknown"
	if activeGw == sw.PrimaryGW && sw.PrimaryGW != "" {
		activeIf = sw.PrimaryInterface + " (PRIMARY)"
	} else if activeGw == sw.BackupGW && sw.BackupGW != "" {
		activeIf = sw.BackupInterface + " (BACKUP)"
	}

	fmt.Printf("Active Interface: %s\n", activeIf)
	fmt.Printf("Active Gateway: %s\n", activeGw)
	fmt.Printf("===========================\n\n")
}
