package cisco

import (
	"encoding/json"
	"os/exec"
	"strconv"
)

// occtlUser is one row of `occtl -j show users`.
type occtlUser struct {
	Username string `json:"Username"`
	Rx       any    `json:"RX"`
	Tx       any    `json:"TX"`
}

// OcctlUsers queries connected users with cumulative byte counters.
// Best-effort: any failure yields no data rather than an error, so traffic
// accounting degrades gracefully when occtl is unavailable.
func OcctlUsers(id int) (map[string][2]int64, []string) {
	out := map[string][2]int64{}
	var online []string
	occtl := FindOcctl()
	if occtl == "" {
		return out, nil
	}
	cmd := exec.Command(occtl, "-s", socketPath(id), "-j", "show", "users")
	raw, err := cmd.Output()
	if err != nil {
		return out, nil
	}
	var users []occtlUser
	if err := json.Unmarshal(raw, &users); err != nil {
		return out, nil
	}
	for _, u := range users {
		if u.Username == "" {
			continue
		}
		out[u.Username] = [2]int64{toBytes(u.Rx), toBytes(u.Tx)}
		online = append(online, u.Username)
	}
	return out, online
}

func toBytes(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case string:
		if i, err := strconv.ParseInt(n, 10, 64); err == nil {
			return i
		}
	}
	return 0
}
