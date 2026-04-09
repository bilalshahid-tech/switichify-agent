package monitor

import (
	"fmt"
	"log"
	"net"
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
	host      string
	ifaceName string
	mutex     sync.RWMutex
	metrics   PingMetrics
	interval  time.Duration
}

func NewPingMonitor(host, iface string, interval time.Duration) *PingMonitor {
	return &PingMonitor{
		host:      host,
		ifaceName: iface,
		interval:  interval,
	}
}

func getInterfaceIP(ifaceName string) (string, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return "", err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no IPv4 address found for %s", ifaceName)
}

func (pm *PingMonitor) RunOnce() {
	pinger, err := ping.NewPinger(pm.host)
	if err != nil {
		log.Println("Ping create error:", err)
		return
	}

	// Interface Binding: Resolves to IP on interface and binds ping packet source to bypass default routing
	if pm.ifaceName != "" {
		if ip, err := getInterfaceIP(pm.ifaceName); err == nil {
			pinger.Source = ip
		} else {
			log.Printf("Ping error: failed to bind to interface %s: %v", pm.ifaceName, err)
			pm.mutex.Lock()
			pm.metrics = PingMetrics{
				AvgLatencyMs: 0,
				PacketLoss:   100,
				JitterMs:     0,
			}
			pm.mutex.Unlock()
			return
		}
	}

	pinger.Count = 5
	pinger.Timeout = 5 * time.Second
	pinger.SetPrivileged(true) // Ensure sudo capabilities are fully utilized for direct socket access

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
