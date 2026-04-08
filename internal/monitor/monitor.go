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
	cfg        *config.Config
	primaryMon *PingMonitor
	backupMon  *PingMonitor
	engine     *decision.DecisionEngine
	switcher   *switcher.Switcher
	comm       *communicator.Communicator
}

func New(cfg *config.Config, comm *communicator.Communicator) *Monitor {
	engine := decision.NewEngine(decision.ThresholdConfig{
		MaxLatencyMs:    float64(cfg.Primary.ICMPThresholdMs),
		MaxPacketLoss:   float64(cfg.Primary.PacketLossThresholdPct),
		MaxJitterMs:     50,
		RecoveryLatency: float64(cfg.Primary.ICMPThresholdMs * 70 / 100),
		RecoveryLoss:    float64(cfg.Primary.PacketLossThresholdPct * 70 / 100),
		RecoveryJitter:  30,
		Cooldown:        15 * time.Second,
		FailCount:       cfg.Primary.FailCount,
	})

	sw, err := switcher.NewSwitcher(cfg.Primary.Interface, cfg.Backup.Interface)
	if err != nil {
		fmt.Printf("[ERROR] failed to initialize switcher: %v\n", err)
	}

	hostP := "8.8.8.8"
	if len(cfg.Primary.TestHosts) > 0 {
		hostP = cfg.Primary.TestHosts[0]
	}
	hostB := "8.8.8.8"
	if len(cfg.Backup.TestHosts) > 0 {
		hostB = cfg.Backup.TestHosts[0]
	}

	return &Monitor{
		cfg:        cfg,
		primaryMon: NewPingMonitor(hostP, cfg.Primary.Interface, time.Duration(cfg.Agent.IntervalSeconds)*time.Second),
		backupMon:  NewPingMonitor(hostB, cfg.Backup.Interface, time.Duration(cfg.Agent.IntervalSeconds)*time.Second),
		engine:     engine,
		switcher:   sw,
		comm:       comm,
	}
}

func (m *Monitor) Run(ctx context.Context) {
	fmt.Printf("[INFO] Monitor started with Parallel Health Checks enabled\n")
	
	demo.PrintActiveState(m.switcher)

	m.comm.SendLog(communicator.LogPayload{
		Level:   "info",
		Time:    time.Now().UTC().Format(time.RFC3339),
		Message: "Agent Monitor started",
	})

	// Start independent parallel health check loops bound strictly to interface
	go func() {
		for {
			select {
			case <-ctx.Done(): return
			default:
				m.primaryMon.RunOnce()
				time.Sleep(1 * time.Second)
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done(): return
			default:
				m.backupMon.RunOnce()
				time.Sleep(1 * time.Second)
			}
		}
	}()

	ticker := time.NewTicker(time.Duration(m.cfg.Agent.IntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Printf("[INFO] Monitor stopping\n")
			return

		case <-ticker.C:
			primaryMetrics := m.primaryMon.GetMetrics()
			backupMetrics := m.backupMon.GetMetrics()
			
			logMsg := fmt.Sprintf("Primary latency: %.0f ms, loss: %.0f%% | Backup latency: %.0f ms, loss: %.0f%%", 
				primaryMetrics.AvgLatencyMs, primaryMetrics.PacketLoss, backupMetrics.AvgLatencyMs, backupMetrics.PacketLoss)

			mp := communicator.MetricsPayload{
				Level:      "info",
				IspState:   m.engine.GetStateString(),
				LatencyMs:  int(primaryMetrics.AvgLatencyMs),
				PacketLoss: int(primaryMetrics.PacketLoss),
				JitterMs:   int(primaryMetrics.JitterMs),
				Time:       time.Now().UTC().Format(time.RFC3339),
				Message:    logMsg,
			}
			m.comm.SendMetrics(mp)

			snapshotP := decision.HealthSnapShot{
				AvgLatencyMs: primaryMetrics.AvgLatencyMs,
				PacketLoss:   primaryMetrics.PacketLoss,
				JitterMs:     primaryMetrics.JitterMs,
			}
			snapshotB := decision.HealthSnapShot{
				AvgLatencyMs: backupMetrics.AvgLatencyMs,
				PacketLoss:   backupMetrics.PacketLoss,
				JitterMs:     backupMetrics.JitterMs,
			}

			state := m.engine.Evaluate(snapshotP, snapshotB)

			switch state {
			case decision.PrimaryDegraded:
				fmt.Printf("[ALERT] Primary degraded (%s) -> switching to backup\n", logMsg)
				m.engine.SetState(decision.SwitchingToBackup)
				m.switchToBackup()
			case decision.FailingBack:
				fmt.Printf("[ALERT] Primary stable -> failing back\n")
				m.engine.SetState(decision.FailingBack)
				m.switchToPrimary()
			}
		}
	}
}

func (m *Monitor) switchToBackup() {
	m.comm.SendLog(communicator.LogPayload{
		Level:   "warn",
		Time:    time.Now().UTC().Format(time.RFC3339),
		Message: "Primary degraded -> switching to backup gateway",
	})

	if err := m.switcher.SwitchToBackup(); err != nil {
		fmt.Printf("[ERROR] failed to switch to backup: %v\n", err)
		m.engine.SetState(decision.PrimaryDegraded) // Revert state so evaluate retriggers try
		return
	}

	if m.switcher.IsUsingGateway(m.switcher.BackupGW) {
		fmt.Printf("[SUCCESS] Route changed to backup gateway\n")
		extIP := m.switcher.VerifyTrafficFlow()
		fmt.Printf("[VERIFY] External Traffic IP: %s\n", extIP)
		
		m.comm.SendLog(communicator.LogPayload{
			Level: "info", Time: time.Now().UTC().Format(time.RFC3339), Message: fmt.Sprintf("Route verified. External IP: %s", extIP),
		})
		m.engine.SetState(decision.BackupActive)
	} else {
		fmt.Printf("[ERROR] Route switch verified failed for backup GW\n")
	}
}

func (m *Monitor) switchToPrimary() {
	m.comm.SendLog(communicator.LogPayload{
		Level:   "info",
		Time:    time.Now().UTC().Format(time.RFC3339),
		Message: "Primary stable -> failing back to primary gateway",
	})

	if err := m.switcher.SwitchToPrimary(); err != nil {
		fmt.Printf("[ERROR] failed to switch to primary: %v\n", err)
		return
	}

	if m.switcher.IsUsingGateway(m.switcher.PrimaryGW) {
		fmt.Printf("[SUCCESS] Route changed to primary gateway\n")
		extIP := m.switcher.VerifyTrafficFlow()
		fmt.Printf("[VERIFY] External Traffic IP: %s\n", extIP)
		
		m.comm.SendLog(communicator.LogPayload{
			Level: "info", Time: time.Now().UTC().Format(time.RFC3339), Message: fmt.Sprintf("Failback verified. External IP: %s", extIP),
		})
		m.engine.SetState(decision.PrimaryActive)
	} else {
		fmt.Printf("[ERROR] Route switch verified failed for primary GW\n")
	}
}
