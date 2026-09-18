package job

import (
	"github.com/mhsanaei/3x-ui/v3/internal/cisco"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/openvpn"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// OpenvpnJob reconciles running openvpn daemons with the enabled inbounds,
// restarts crashed ones, and folds per-client status-file traffic into the
// usual accounting.
type OpenvpnJob struct {
	inboundService service.InboundService
}

// NewOpenvpnJob creates the OpenVPN reconcile/traffic job.
func NewOpenvpnJob() *OpenvpnJob {
	return new(OpenvpnJob)
}

// Run reconciles daemons and records per-client traffic deltas and online status.
func (j *OpenvpnJob) Run() {
	desired, err := j.inboundService.DesiredOpenvpnInstances()
	if err != nil {
		logger.Warning("openvpn job: get desired instances failed:", err)
		return
	}
	tags := make([]string, 0, len(desired))
	for _, inst := range desired {
		tags = append(tags, inst.Tag)
	}
	mgr := openvpn.GetManager()
	mgr.Reconcile(desired)
	deltas, onlineEmails := mgr.CollectTraffic()

	clientTraffics := make([]*xray.ClientTraffic, 0, len(deltas))
	inboundUp := make(map[string]int64)
	inboundDown := make(map[string]int64)
	for _, d := range deltas {
		clientTraffics = append(clientTraffics, &xray.ClientTraffic{
			Email: d.Email,
			Up:    d.Up,
			Down:  d.Down,
		})
		inboundUp[d.Tag] += d.Up
		inboundDown[d.Tag] += d.Down
	}
	traffics := make([]*xray.Traffic, 0, len(inboundUp))
	for tag, up := range inboundUp {
		traffics = append(traffics, &xray.Traffic{
			IsInbound: true,
			Tag:       tag,
			Up:        up,
			Down:      inboundDown[tag],
		})
	}
	if len(traffics) > 0 || len(clientTraffics) > 0 {
		if _, _, err := j.inboundService.AddTraffic(traffics, clientTraffics); err != nil {
			logger.Warning("openvpn job: add traffic failed:", err)
		}
	}
	j.inboundService.RefreshLocalOnlineClients(onlineEmails, tags)
}

// CiscoJob reconciles running ocserv daemons and folds occtl traffic in.
type CiscoJob struct {
	inboundService service.InboundService
}

// NewCiscoJob creates the ocserv reconcile/traffic job.
func NewCiscoJob() *CiscoJob {
	return new(CiscoJob)
}

// Run reconciles daemons and records per-client traffic deltas and online status.
func (j *CiscoJob) Run() {
	desired, err := j.inboundService.DesiredCiscoInstances()
	if err != nil {
		logger.Warning("cisco job: get desired instances failed:", err)
		return
	}
	tags := make([]string, 0, len(desired))
	for _, inst := range desired {
		tags = append(tags, inst.Tag)
	}
	mgr := cisco.GetManager()
	mgr.Reconcile(desired)
	deltas, onlineEmails := mgr.CollectTraffic()

	clientTraffics := make([]*xray.ClientTraffic, 0, len(deltas))
	inboundUp := make(map[string]int64)
	inboundDown := make(map[string]int64)
	for _, d := range deltas {
		clientTraffics = append(clientTraffics, &xray.ClientTraffic{
			Email: d.Email,
			Up:    d.Up,
			Down:  d.Down,
		})
		inboundUp[d.Tag] += d.Up
		inboundDown[d.Tag] += d.Down
	}
	traffics := make([]*xray.Traffic, 0, len(inboundUp))
	for tag, up := range inboundUp {
		traffics = append(traffics, &xray.Traffic{
			IsInbound: true,
			Tag:       tag,
			Up:        up,
			Down:      inboundDown[tag],
		})
	}
	if len(traffics) > 0 || len(clientTraffics) > 0 {
		if _, _, err := j.inboundService.AddTraffic(traffics, clientTraffics); err != nil {
			logger.Warning("cisco job: add traffic failed:", err)
		}
	}
	j.inboundService.RefreshLocalOnlineClients(onlineEmails, tags)
}
