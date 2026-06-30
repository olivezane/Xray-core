package metrics

import "github.com/xtls/xray-core/common/net/cnc"

// OutboundListener is a net.Listener for listening metrics http connections.
type OutboundListener = cnc.OutboundListener

// Outbound is an outbound.Handler that handles metrics http connections.
type Outbound = cnc.Outbound
