package p2p

import "time"

// NetworkMetrics holds a snapshot of the P2P network state and statistics.
type NetworkMetrics struct {
	// ConnectedPeers is the number of currently connected peers.
	ConnectedPeers int

	// Topics is the list of GossipSub topics the node is participating in.
	Topics []string

	// BandwidthIn is the total number of bytes received since startup.
	BandwidthIn int64

	// BandwidthOut is the total number of bytes sent since startup.
	BandwidthOut int64

	// RateIn is the current inbound rate in bytes per second.
	RateIn int64

	// RateOut is the current outbound rate in bytes per second.
	RateOut int64

	// Uptime is the duration since the host was created.
	Uptime time.Duration
}

// Metrics collects and returns current network metrics.
func (h *Host) Metrics() NetworkMetrics {
	m := NetworkMetrics{
		ConnectedPeers: h.PeerCount(),
		Uptime:         time.Since(h.startTime),
	}

	// Bandwidth stats from the counter.
	if h.bwCounter != nil {
		stats := h.bwCounter.GetBandwidthTotals()
		m.BandwidthIn = stats.TotalIn
		m.BandwidthOut = stats.TotalOut
		m.RateIn = int64(stats.RateIn)
		m.RateOut = int64(stats.RateOut)
	}

	// Active topics.
	if h.pubsub != nil {
		m.Topics = h.pubsub.GetTopics()
	}

	return m
}
