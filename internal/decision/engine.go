package decision

import (
	"sync"
	"time"
)

type ISPState string

type HealthSnapShot struct {
	AvgLatencyMs float64
	PacketLoss   float64
	JitterMs     float64
}

const (
	PrimaryActive ISPState = "PRIMARY_ACTIVE"
	BackupActive  ISPState = "BACKUP_ACTIVE"
	FailingOver   ISPState = "FAILING_OVER"
	FailingBack   ISPState = "FAILING_BACK"
)

type DecisionEngine struct {
	mu             sync.Mutex
	state          ISPState
	lastSwitchTime time.Time
	cooldown       time.Duration

	// Thresholds
	MaxLatencyMs    float64
	MaxPacketLoss   float64
	MaxJitterMs     float64
	RecoveryLatency float64
	RecoveryLoss    float64
	RecoveryJitter  float64
}

func NewEngine(cfgThresholds ThresholdConfig) *DecisionEngine {
	return &DecisionEngine{
		state:           PrimaryActive,
		cooldown:        cfgThresholds.Cooldown,
		MaxLatencyMs:    cfgThresholds.MaxLatencyMs,
		MaxPacketLoss:   cfgThresholds.MaxPacketLoss,
		MaxJitterMs:     cfgThresholds.MaxJitterMs,
		RecoveryLatency: cfgThresholds.RecoveryLatency,
		RecoveryLoss:    cfgThresholds.RecoveryLoss,
		RecoveryJitter:  cfgThresholds.RecoveryJitter,
	}
}

type ThresholdConfig struct {
	MaxLatencyMs    float64
	MaxPacketLoss   float64
	MaxJitterMs     float64
	RecoveryLatency float64
	RecoveryLoss    float64
	RecoveryJitter  float64
	Cooldown        time.Duration
}

func (e *DecisionEngine) Evaluate(metrics HealthSnapShot) ISPState {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()

	// Prevent flapping, allow stabilization
	if now.Sub(e.lastSwitchTime) < e.cooldown {
		return e.state
	}

	switch e.state {

	case PrimaryActive:
		if metrics.AvgLatencyMs > e.MaxLatencyMs ||
			metrics.PacketLoss > e.MaxPacketLoss ||
			metrics.JitterMs > e.MaxJitterMs {

			e.state = FailingOver
			e.lastSwitchTime = now
			return FailingOver
		}

	case BackupActive:
		if metrics.AvgLatencyMs < e.RecoveryLatency &&
			metrics.PacketLoss < e.RecoveryLoss &&
			metrics.JitterMs < e.RecoveryJitter {

			e.state = FailingBack
			e.lastSwitchTime = now
			return FailingBack
		}

	case FailingOver:
		e.state = BackupActive
		return BackupActive

	case FailingBack:
		e.state = PrimaryActive
		return PrimaryActive
	}

	return e.state
}
