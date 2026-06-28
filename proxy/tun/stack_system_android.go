//go:build android

package tun

import (
	"github.com/xtls/xray-core/common/errors"
	"golang.org/x/sys/unix"
)

type androidPacketIO struct {
	fd int
}

func newPacketIO(t Tun) (packetIO, error) {
	at, ok := t.(*AndroidTun)
	if !ok {
		return nil, errors.New("system stack requires *AndroidTun")
	}
	_ = unix.SetNonblock(at.tunFd, false)
	return &androidPacketIO{fd: at.tunFd}, nil
}

func (w *androidPacketIO) ReadPacket() ([]byte, error) {
	buf := make([]byte, 65535)
	n, err := unix.Read(w.fd, buf)
	if err != nil {
		return nil, err
	}
	data := make([]byte, n)
	copy(data, buf[:n])
	return data, nil
}

func (w *androidPacketIO) WritePacket(data []byte) error {
	_, err := unix.Write(w.fd, data)
	return err
}
