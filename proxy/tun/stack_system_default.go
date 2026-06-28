//go:build !windows

package tun

import "github.com/xtls/xray-core/common/errors"

func newPacketIO(t Tun) (packetIO, error) {
	return nil, errors.New("system stack is not supported on this platform")
}
