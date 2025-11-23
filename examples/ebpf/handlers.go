package main

import (
	"context"
	"net"

	"github.com/squadracorsepolito/acmetel/ingress"
	"github.com/squadracorsepolito/acmetel/processor"
)

type pingHandler struct {
	processor.CustomHandlerBase
}

func newPingHandler() *pingHandler {
	return &pingHandler{}
}

func (h *pingHandler) Handle(_ context.Context, msgIn, _ *ingress.EBPFMessage[PingEvent]) error {
	pingEvent := msgIn.Data

	srcIP := h.getIP(pingEvent.SrcIP)
	dstIP := h.getIP(pingEvent.DstIP)

	h.Telemetry.LogInfo("ping packet", "src_ip", srcIP.String(), "dst_ip", dstIP.String(), "id", pingEvent.ID, "seq", pingEvent.Seq)

	return nil
}

func (h *pingHandler) getIP(ip uint32) net.IP {
	return net.IPv4(
		byte(ip),
		byte(ip>>8),
		byte(ip>>16),
		byte(ip>>24),
	)
}
