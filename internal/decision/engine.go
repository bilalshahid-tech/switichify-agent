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
	PrimaryActive     ISPState = "PRIMARY_ACTIVE"
	PrimaryDegraded   ISPState = "PRIMARY_DEGRADED"
	SwitchingToBackup ISPState = "SWITCHING_TO_BACKUP"
	BackupActive      ISPState = "BACKUP_ACTIVE"
	FailingBack       ISPState = "FAILING_BACK"
)

type DecisionEngine struct {
	mu             sync.Mutex
	state          ISPState
	lastSwitchTime time.Time
	cooldown       time.Duration

	primaryFailCount   int
	primaryStableStart time.Time
	requiredFailures   int
	failbackWait       time.Duration

	MaxLatencyMs    float64
	MaxPacketLoss   float64
	MaxJitterMs     float64
	RecoveryLatency float64
	RecoveryLoss    float64
	RecoveryJitter  float64
}

type ThresholdConfig struct {
	MaxLatencyMs    float64
	MaxPacketLoss   float64
	MaxJitterMs     float64
	RecoveryLatency float64
	RecoveryLoss    float64
	RecoveryJitter  float64
	Cooldown        time.Duration
	FailCount       int
}

func NewEngine(cfgThresholds ThresholdConfig) *DecisionEngine {
	fc := cfgThresholds.FailCount
	if fc <= 0 {
		fc = 3 // Default strict 3 consecutive failures requirements to avoid flap
	}

	return &DecisionEngine{
		state:            PrimaryActive,
		cooldown:         cfgThresholds.Cooldown,
		requiredFailures: fc,
		failbackWait:     30 * time.Second, // Stabilize timer for failback fixes loop issue
		MaxLatencyMs:     cfgThresholds.MaxLatencyMs,
		MaxPacketLoss:    cfgThresholds.MaxPacketLoss,
		MaxJitterMs:      cfgThresholds.MaxJitterMs,
		RecoveryLatency:  cfgThresholds.RecoveryLatency,
		RecoveryLoss:     cfgThresholds.RecoveryLoss,
		RecoveryJitter:   cfgThresholds.RecoveryJitter,
	}
}

// Evaluate evaluates constraints using robust degradation requirement
func (e *DecisionEngine) Evaluate(primary HealthSnapShot, backup HealthSnapShot) ISPState {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()

	if now.Sub(e.lastSwitchTime) < e.cooldown {
		return e.state
	}

	isPrimaryFailing := primary.AvgLatencyMs > e.MaxLatencyMs || primary.PacketLoss > e.MaxPacketLoss || primary.JitterMs > e.MaxJitterMs
	isPrimaryRecovered := primary.AvgLatencyMs < e.RecoveryLatency && primary.PacketLoss < e.RecoveryLoss && primary.JitterMs < e.RecoveryJitter

	if isPrimaryFailing {
		e.primaryFailCount++
		e.primaryStableStart = time.Time{}
	} else if isPrimaryRecovered {
		e.primaryFailCount = 0
		if e.primaryStableStart.IsZero() {
			e.primaryStableStart = now
		}
	} else {
		e.primaryFailCount = 0
		e.primaryStableStart = time.Time{}
	}

	switch e.state {

	case PrimaryActive:
		if e.primaryFailCount >= e.requiredFailures {
			e.state = PrimaryDegraded
			e.lastSwitchTime = now
			return PrimaryDegraded
		}

	case SwitchingToBackup:
		// Transition happens from setstate externally upon success

	case BackupActive:
		if !e.primaryStableStart.IsZero() && now.Sub(e.primaryStableStart) >= e.failbackWait {
			e.state = FailingBack
			e.lastSwitchTime = now
			return FailingBack
		}

	case FailingBack:
		// Transition happens from external

	case PrimaryDegraded:
		// external handled
	}

	return e.state
}

func (e *DecisionEngine) SetState(newState ISPState) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.state = newState
	e.lastSwitchTime = time.Now()
}

func (e *DecisionEngine) GetStateString() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return string(e.state)
}
