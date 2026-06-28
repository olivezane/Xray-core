//go:build darwin || freebsd

// ponytail: darwin+freebsd share the same 4-byte tun/utun protocol prefix.
// The File() method is injected in tun_darwin.go and tun_freebsd.go.

package tun

import (
	"os"

	"github.com/xtls/xray-core/common/errors"
	"golang.org/x/sys/unix"
)

type bsdPacketIO struct {
	fd int
}

type fileProvider interface {
	File() *os.File
}

func newPacketIO(t Tun) (packetIO, error) {
	fp, ok := t.(fileProvider)
	if !ok {
		return nil, errors.New("system stack requires a tun with *os.File access")
	}
	fd := int(fp.File().Fd())
	_ = unix.SetNonblock(fd, false)
	return &bsdPacketIO{fd: fd}, nil
}

func (w *bsdPacketIO) ReadPacket() ([]byte, error) {
	buf := make([]byte, 65535+4)
	n, err := unix.Read(w.fd, buf)
	if err != nil {
		return nil, err
	}
	if n < 4 {
		return nil, nil
	}
	data := make([]byte, n-4)
	copy(data, buf[4:n])
	return data, nil
}

func (w *bsdPacketIO) WritePacket(data []byte) error {
	family := byte(unix.AF_INET)
	if len(data) > 0 && data[0]>>4 == 6 {
		family = byte(unix.AF_INET6)
	}
	pkt := make([]byte, 4+len(data))
	pkt[3] = family
	copy(pkt[4:], data)
	_, err := unix.Write(w.fd, pkt)
	return err
}
