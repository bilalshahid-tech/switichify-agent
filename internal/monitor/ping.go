package monitor

import (
	"log"
	"sync"
	"time"

	"github.com/go-ping/ping"
)

type PingMetrics struct {
	AvgLatencyMs float64
	PacketLoss   float64
	JitterMs     float64
}

type PingMonitor struct {
	host     string
	mutex    sync.RWMutex
	metrics  PingMetrics
	interval time.Duration
}

func NewPingMonitor(host string, interval time.Duration) *PingMonitor {
	return &PingMonitor{
		host:     host,
		interval: interval,
	}
}

func (pm *PingMonitor) RunOnce() {
	pinger, err := ping.NewPinger(pm.host)
	if err != nil {
		log.Println("Ping create error:", err)
		return
	}

	pinger.Count = 5
	pinger.Timeout = 5 * time.Second
	pinger.SetPrivileged(false)

	var previousRTT time.Duration
	var jitterTotal float64
	var jitterCount int

	pinger.OnRecv = func(pkt *ping.Packet) {
		if previousRTT != 0 {
			diff := pkt.Rtt - previousRTT
			if diff < 0 {
				diff = -diff
			}
			jitterTotal += float64(diff.Milliseconds())
			jitterCount++
		}
		previousRTT = pkt.Rtt
	}

	err = pinger.Run()
	if err != nil {
		log.Println("Ping run error:", err)
		return
	}

	stats := pinger.Statistics()

	jitter := 0.0
	if jitterCount > 0 {
		jitter = jitterTotal / float64(jitterCount)
	}

	pm.mutex.Lock()
	pm.metrics = PingMetrics{
		AvgLatencyMs: float64(stats.AvgRtt.Milliseconds()),
		PacketLoss:   stats.PacketLoss,
		JitterMs:     jitter,
	}
	pm.mutex.Unlock()
}

func (pm *PingMonitor) GetMetrics() PingMetrics {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	return pm.metrics
}
