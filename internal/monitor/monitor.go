package monitor

import (
	"context"
	"fmt"
	"time"

	"github.com/bilal/switchify-agent/internal/communicator"
	"github.com/bilal/switchify-agent/internal/config"
	"github.com/bilal/switchify-agent/internal/decision"
	"github.com/bilal/switchify-agent/internal/demo"
	"github.com/bilal/switchify-agent/internal/switcher"
)

type Monitor struct {
	cfg      *config.Config
	pingMon  *PingMonitor
	engine   *decision.DecisionEngine
	switcher *switcher.Switcher
	comm     *communicator.Communicator
}

func New(cfg *config.Config, comm *communicator.Communicator) *Monitor {
	engine := decision.NewEngine(decision.ThresholdConfig{
		MaxLatencyMs:    float64(cfg.Primary.ICMPThresholdMs),
		MaxPacketLoss:   float64(cfg.Primary.PacketLossThresholdPct),
		MaxJitterMs:     50,
		RecoveryLatency: float64(cfg.Primary.ICMPThresholdMs * 70 / 100),
		RecoveryLoss:    float64(cfg.Primary.PacketLossThresholdPct * 70 / 100),
		RecoveryJitter:  30,
		Cooldown:        15 * time.Second, // 10-15s flap prevention
	})

	sw, err := switcher.NewSwitcher(cfg.Primary.Interface, cfg.Backup.Interface)
	if err != nil {
		fmt.Printf("[ERROR] failed to initialize switcher: %v\n", err)
	}

	return &Monitor{
		cfg:      cfg,
		pingMon:  NewPingMonitor(cfg.Agent.TestHosts[0], time.Duration(cfg.Agent.IntervalSeconds)*time.Second),
		engine:   engine,
		switcher: sw,
		comm:     comm,
	}
}

func (m *Monitor) Run(ctx context.Context) {
	fmt.Printf("[INFO] Monitor started\n")
	
	demo.PrintActiveState(m.switcher)

	m.comm.SendLog(communicator.LogPayload{
		Level:   "info",
		Time:    time.Now().UTC().Format(time.RFC3339),
		Message: "Agent Monitor started",
	})

	ticker := time.NewTicker(time.Duration(m.cfg.Agent.IntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Printf("[INFO] Monitor stopping\n")
			return

		case <-ticker.C:
			go m.pingMon.RunOnce()

			metrics := m.pingMon.GetMetrics()
			
			// create telemetry payload
			mp := communicator.MetricsPayload{
				Level:      "info",
				IspState:   "PRIMARY_ACTIVE",
				LatencyMs:  int(metrics.AvgLatencyMs),
				PacketLoss: int(metrics.PacketLoss),
				JitterMs:   int(metrics.JitterMs),
				Time:       time.Now().UTC().Format(time.RFC3339),
				Message:    "ping metrics collected",
			}
			m.comm.SendMetrics(mp)

			snapshot := decision.HealthSnapShot{
				AvgLatencyMs: metrics.AvgLatencyMs,
				PacketLoss:   metrics.PacketLoss,
				JitterMs:     metrics.JitterMs,
			}
			
			if metrics.AvgLatencyMs > float64(m.cfg.Primary.ICMPThresholdMs) {
				fmt.Printf("[WARN] High latency detected: %.0fms\n", metrics.AvgLatencyMs)
			}
			if metrics.PacketLoss > float64(m.cfg.Primary.PacketLossThresholdPct) {
				fmt.Printf("[WARN] High packet loss detected: %.0f%%\n", metrics.PacketLoss)
			}

			state := m.engine.Evaluate(snapshot)

			// Simple manual checks for logging before failover
			switch state {
			case decision.PrimaryDegraded:
				fmt.Printf("[ALERT] Primary ISP degraded\n")
				fmt.Printf("[ACTION] Switching to Backup ISP\n")
				m.switchToBackup()
			case decision.FailingBack:
				fmt.Printf("[ALERT] Backup ISP recovered\n")
				fmt.Printf("[ACTION] Switching to Primary ISP\n")
				m.switchToPrimary()
			}
		}
	}
}

func (m *Monitor) switchToBackup() {
	m.comm.SendLog(communicator.LogPayload{
		Level:   "warn",
		Time:    time.Now().UTC().Format(time.RFC3339),
		Message: "Switching to BACKUP ISP",
	})

	if err := m.switcher.SwitchToBackup(); err != nil {
		fmt.Printf("[ERROR] failed to switch to backup: %v\n", err)
		return
	}

	if m.switcher.IsUsingGateway(m.switcher.BackupGW) {
		fmt.Printf("[SUCCESS] Route updated successfully\n")
		fmt.Printf("[INFO] Current Active ISP: BACKUP\n")
	} else {
		fmt.Printf("[ERROR] Route switch verified failed for backup GW\n")
	}
}

func (m *Monitor) switchToPrimary() {
	m.comm.SendLog(communicator.LogPayload{
		Level:   "info",
		Time:    time.Now().UTC().Format(time.RFC3339),
		Message: "Switching to PRIMARY ISP",
	})

	if err := m.switcher.SwitchToPrimary(); err != nil {
		fmt.Printf("[ERROR] failed to switch to primary: %v\n", err)
		return
	}

	if m.switcher.IsUsingGateway(m.switcher.PrimaryGW) {
		fmt.Printf("[SUCCESS] Route updated successfully\n")
		fmt.Printf("[INFO] Current Active ISP: PRIMARY\n")
	} else {
		fmt.Printf("[ERROR] Route switch verified failed for primary GW\n")
	}
}

