package commander

import "github.com/xtls/xray-core/common/net/cnc"

// OutboundListener is a net.Listener for listening gRPC connections.
type OutboundListener = cnc.OutboundListener

// Outbound is a outbound.Handler that handles gRPC connections.
type Outbound = cnc.Outbound
