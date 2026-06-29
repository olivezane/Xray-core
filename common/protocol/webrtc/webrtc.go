package webrtc

import (
	"encoding/binary"
	"errors"

	"github.com/xtls/xray-core/common"
)

type SniffHeader struct{}

func (h *SniffHeader) Protocol() string {
	return "webrtc"
}

func (h *SniffHeader) Domain() string {
	return ""
}

var errNotWebRTC = errors.New("not webrtc (stun)")

// SniffSTUN detects STUN packets, which are the most reliable indicator of WebRTC ICE/TURN traffic.
func SniffSTUN(b []byte) (*SniffHeader, error) {
	if len(b) < 20 {
		return nil, common.ErrNoClue
	}
	// STUN: top 2 bits = 0, magic cookie at offset 4 = 0x2112A442
	if b[0]&0xC0 != 0 {
		return nil, errNotWebRTC
	}
	if binary.BigEndian.Uint32(b[4:8]) != 0x2112A442 {
		return nil, errNotWebRTC
	}
	return &SniffHeader{}, nil
}
