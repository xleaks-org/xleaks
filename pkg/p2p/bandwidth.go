package p2p

// BandwidthStats holds a point-in-time snapshot of bandwidth usage.
type BandwidthStats struct {
	// TotalIn is the total number of bytes received.
	TotalIn int64
	// TotalOut is the total number of bytes sent.
	TotalOut int64
	// RateIn is the current inbound rate in bytes per second.
	RateIn int64
	// RateOut is the current outbound rate in bytes per second.
	RateOut int64
}

// BandwidthStats returns the current bandwidth statistics by reading from
// the libp2p bandwidth counter attached to the host.
func (h *Host) BandwidthStats() BandwidthStats {
	if h.bwCounter == nil {
		return BandwidthStats{}
	}

	stats := h.bwCounter.GetBandwidthTotals()
	return BandwidthStats{
		TotalIn:  stats.TotalIn,
		TotalOut: stats.TotalOut,
		RateIn:   int64(stats.RateIn),
		RateOut:  int64(stats.RateOut),
	}
}
