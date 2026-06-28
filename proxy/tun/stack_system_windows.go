//go:build windows

package tun

import (
	"io"

	"golang.org/x/sys/windows"

	"github.com/xtls/xray-core/common/errors"
)

type windowsPacketIO struct {
	tun *WindowsTun
}

func newPacketIO(t Tun) (packetIO, error) {
	wt, ok := t.(*WindowsTun)
	if !ok {
		return nil, errors.New("system stack requires *WindowsTun")
	}
	return &windowsPacketIO{tun: wt}, nil
}

func (w *windowsPacketIO) ReadPacket() ([]byte, error) {
	packet, err := w.tun.session.ReceivePacket()
	if err != nil {
		if err == windows.ERROR_NO_MORE_ITEMS {
			w.tun.Wait()
			return nil, nil
		}
		return nil, err
	}
	data := make([]byte, len(packet))
	copy(data, packet)
	w.tun.session.ReleaseReceivePacket(packet)
	return data, nil
}

func (w *windowsPacketIO) WritePacket(data []byte) error {
	w.tun.RLock()
	defer w.tun.RUnlock()
	if w.tun.closed {
		return io.ErrClosedPipe
	}
	packet, err := w.tun.session.AllocateSendPacket(len(data))
	if err != nil {
		return err
	}
	copy(packet, data)
	w.tun.session.SendPacket(packet)
	return nil
}
