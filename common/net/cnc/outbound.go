package cnc

import (
	"context"
	"sync"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/errors"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/common/signal/done"
	"github.com/xtls/xray-core/transport"
)

// OutboundListener is a net.Listener for accepting proxied transport links.
type OutboundListener struct {
	buffer chan net.Conn
	done   *done.Instance
}

func NewOutboundListener(size int) *OutboundListener {
	return &OutboundListener{
		buffer: make(chan net.Conn, size),
		done:   done.New(),
	}
}

func (l *OutboundListener) add(conn net.Conn) {
	select {
	case l.buffer <- conn:
	case <-l.done.Wait():
		conn.Close()
	default:
		conn.Close()
	}
}

func (l *OutboundListener) Accept() (net.Conn, error) {
	select {
	case <-l.done.Wait():
		return nil, errors.New("listen closed")
	case c := <-l.buffer:
		return c, nil
	}
}

func (l *OutboundListener) Close() error {
	common.Must(l.done.Close())
	for {
		select {
		case c := <-l.buffer:
			c.Close()
		default:
			return nil
		}
	}
}

func (l *OutboundListener) Addr() net.Addr {
	return &net.TCPAddr{
		IP:   net.IP{0, 0, 0, 0},
		Port: 0,
	}
}

// Outbound dispatches links into an OutboundListener.
type Outbound struct {
	tag      string
	listener *OutboundListener
	access   sync.RWMutex
	closed   bool
}

func NewOutbound(tag string, listener *OutboundListener) *Outbound {
	return &Outbound{
		tag:      tag,
		listener: listener,
	}
}

func (co *Outbound) Dispatch(ctx context.Context, link *transport.Link) {
	co.access.RLock()

	if co.closed {
		common.Interrupt(link.Reader)
		common.Interrupt(link.Writer)
		co.access.RUnlock()
		return
	}

	closeSignal := done.New()
	c := NewConnection(ConnectionInputMulti(link.Writer), ConnectionOutputMulti(link.Reader), ConnectionOnClose(closeSignal))
	co.listener.add(c)
	co.access.RUnlock()
	<-closeSignal.Wait()
}

func (co *Outbound) Tag() string {
	return co.tag
}

func (co *Outbound) Start() error {
	co.access.Lock()
	co.closed = false
	co.access.Unlock()
	return nil
}

func (co *Outbound) Close() error {
	co.access.Lock()
	defer co.access.Unlock()

	co.closed = true
	return co.listener.Close()
}

func (co *Outbound) SenderSettings() *serial.TypedMessage {
	return nil
}

func (co *Outbound) ProxySettings() *serial.TypedMessage {
	return nil
}
