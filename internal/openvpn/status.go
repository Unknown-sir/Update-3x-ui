package openvpn

import (
	"os"
	"strconv"
	"strings"
)

// ClientCounters holds cumulative bytes per client CN from the daemon status.
type ClientCounters struct {
	Received int64
	Sent     int64
}

// ParseStatusFile reads an OpenVPN status file (version 2/3) and returns
// per-CN counters plus the connected CN set.
func ParseStatusFile(path string) (map[string]ClientCounters, []string) {
	counters := map[string]ClientCounters{}
	var online []string
	data, err := os.ReadFile(path)
	if err != nil {
		return counters, nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		// CLIENT_LIST,cn,realAddr,virtAddr,rx,tx,connectedSince,...
		f := strings.Split(strings.TrimSpace(line), ",")
		if len(f) < 7 || f[0] != "CLIENT_LIST" || f[1] == "Common Name" {
			continue
		}
		rx, _ := strconv.ParseInt(strings.TrimSpace(f[4]), 10, 64)
		tx, _ := strconv.ParseInt(strings.TrimSpace(f[5]), 10, 64)
		cn := strings.TrimSpace(f[1])
		counters[cn] = ClientCounters{Received: rx, Sent: tx}
		online = append(online, cn)
	}
	return counters, online
}
