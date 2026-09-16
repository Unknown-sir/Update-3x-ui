package service

import (
	"context"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// applyLocalSidecarInbound re-records desired sidecar state after a
// client/inbound mutation, mirroring applyLocalTuic. The runtime dispatch
// converges the OpenVPN/ocserv daemon from this state.
func applyLocalSidecarInbound(s *InboundService, inbound *model.Inbound, name string) {
	rt, err := s.runtimeFor(inbound)
	if err != nil {
		return
	}
	payload := inbound
	if inbound.Enable {
		if built, bErr := s.buildInboundForLocalRuntime(database.GetDB(), inbound); bErr == nil {
			payload = built
		}
	}
	if err := rt.UpdateInbound(context.Background(), inbound, payload); err != nil {
		logger.Debug(name+": immediate client apply failed for inbound", inbound.Id, ":", err)
	}
}

// applyLocalOpenvpn re-records the desired OpenVPN sidecar state.
func (s *InboundService) applyLocalOpenvpn(inboundId int) {
	inbound, err := s.GetInbound(inboundId)
	if err != nil || inbound == nil || inbound.Protocol != model.OpenVPN || inbound.NodeID != nil {
		return
	}
	applyLocalSidecarInbound(s, inbound, "openvpn")
}

// applyLocalCisco re-records the desired ocserv sidecar state.
func (s *InboundService) applyLocalCisco(inboundId int) {
	inbound, err := s.GetInbound(inboundId)
	if err != nil || inbound == nil || inbound.Protocol != model.Cisco || inbound.NodeID != nil {
		return
	}
	applyLocalSidecarInbound(s, inbound, "cisco")
}
