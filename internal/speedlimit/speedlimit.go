// Package speedlimit resolves the effective per-user cap in Mbps.
// Precedence: client limit > reseller limit > inbound default. 0 = unlimited.
package speedlimit

// EffectiveMbps returns the cap that applies to one user.
// Any positive value wins; the most specific scope takes precedence.
func EffectiveMbps(clientMbps, resellerMbps, inboundMbps int) int {
	if clientMbps > 0 {
		return clientMbps
	}
	if resellerMbps > 0 {
		return resellerMbps
	}
	if inboundMbps > 0 {
		return inboundMbps
	}
	return 0
}

// BytesPerSec converts an Mbps cap to bytes/sec for shaper directives.
func BytesPerSec(mbps int) int64 {
	if mbps <= 0 {
		return 0
	}
	return int64(mbps) * 1024 * 1024 / 8
}
