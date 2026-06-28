//go:build linux && !android

package tun

import (
	"github.com/xtls/xray-core/common/errors"
	"golang.org/x/sys/unix"
)

type linuxPacketIO struct {
	fd int
}

func newPacketIO(t Tun) (packetIO, error) {
	lt, ok := t.(*LinuxTun)
	if !ok {
		return nil, errors.New("system stack requires *LinuxTun")
	}
	_ = unix.SetNonblock(lt.tunFd, false)
	return &linuxPacketIO{fd: lt.tunFd}, nil
}

func (w *linuxPacketIO) ReadPacket() ([]byte, error) {
	buf := make([]byte, 65535)
	n, err := unix.Read(w.fd, buf)
	if err != nil {
		return nil, err
	}
	data := make([]byte, n)
	copy(data, buf[:n])
	return data, nil
}

func (w *linuxPacketIO) WritePacket(data []byte) error {
	_, err := unix.Write(w.fd, data)
	return err
}
