package tun

import (
	"context"
	"encoding/binary"
	"io"
	"math/rand/v2"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/xtls/xray-core/common/errors"
	xnet "github.com/xtls/xray-core/common/net"
	tunicmp "github.com/xtls/xray-core/proxy/tun/icmp"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
)

const (
	protocolTCP    = 6
	protocolUDP    = 17
	protocolICMPv4 = 1
	protocolICMPv6 = 58

	tcpFlagFIN = 1
	tcpFlagSYN = 2
	tcpFlagRST = 4
	tcpFlagACK = 16

	defaultTTL = 64
)

type flowKey struct {
	proto uint8
	src   netip.AddrPort
	dst   netip.AddrPort
}

type tcpFlowConn struct {
	key       flowKey
	stack     *systemStack
	srcIP     netip.Addr
	dstIP     netip.Addr
	srcPort   uint16
	dstPort   uint16
	clientISN uint32
	serverISN uint32

	mu         sync.Mutex
	clientNext uint32
	serverNext uint32

	incoming chan []byte
	closeOnce sync.Once
}

type udpFlowConn struct {
	key       flowKey
	stack     *systemStack
	srcIP     netip.Addr
	dstIP     netip.Addr
	srcPort   uint16
	dstPort   uint16

	incoming chan []byte
	closeOnce sync.Once
}

type packetIO interface {
	ReadPacket() ([]byte, error)
	WritePacket([]byte) error
}

type systemStack struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pio         packetIO
	handler     *Handler
	idleTimeout time.Duration

	mu       sync.Mutex
	tcpFlows map[flowKey]*tcpFlowConn
	udpFlows map[flowKey]*udpFlowConn

	wg sync.WaitGroup
}

func newStackSystem(ctx context.Context, options StackOptions, handler *Handler) (Stack, error) {
	pio, err := newPacketIO(options.Tun)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	return &systemStack{
		ctx:         ctx,
		cancel:      cancel,
		pio:         pio,
		handler:     handler,
		idleTimeout: options.IdleTimeout,
		tcpFlows:    make(map[flowKey]*tcpFlowConn),
		udpFlows:    make(map[flowKey]*udpFlowConn),
	}, nil
}

func (s *systemStack) Start() error {
	s.wg.Add(1)
	go s.eventLoop()
	return nil
}

func (s *systemStack) Close() error {
	s.cancel()
	s.mu.Lock()
	for _, f := range s.tcpFlows {
		f.Close()
	}
	for _, f := range s.udpFlows {
		f.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
	return nil
}

func (s *systemStack) eventLoop() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}
		data, err := s.pio.ReadPacket()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
			}
			continue
		}
		s.handlePacket(data)
	}
}

func (s *systemStack) handlePacket(data []byte) {
	if len(data) < 1 {
		return
	}
	version := data[0] >> 4
	switch version {
	case 4:
		s.handleIPv4(data)
	case 6:
		s.handleIPv6(data)
	}
}

func (s *systemStack) handleIPv4(data []byte) {
	hdrLen, totalLen, proto, srcIP, dstIP, ok := parseIPv4(data)
	if !ok || totalLen > len(data) {
		return
	}
	payload := data[hdrLen:totalLen]
	switch proto {
	case protocolTCP:
		s.handleTCP(srcIP, dstIP, payload)
	case protocolUDP:
		s.handleUDP(srcIP, dstIP, payload)
	case protocolICMPv4:
		s.handleICMPv4(data)
	}
}

func (s *systemStack) handleIPv6(data []byte) {
	if len(data) < 40 {
		return
	}
	nextHeader := data[6]
	payloadLen := int(binary.BigEndian.Uint16(data[4:6])) + 40
	if payloadLen > len(data) {
		return
	}
	srcIP, ok := netip.AddrFromSlice(data[8:24])
	if !ok {
		return
	}
	dstIP, ok2 := netip.AddrFromSlice(data[24:40])
	if !ok2 {
		return
	}
	payload := data[40:payloadLen]
	switch nextHeader {
	case protocolTCP:
		s.handleTCP(srcIP, dstIP, payload)
	case protocolUDP:
		s.handleUDP(srcIP, dstIP, payload)
	case protocolICMPv6:
		s.handleICMPv6(data)
	}
}

func parseIPv4(data []byte) (hdrLen int, totalLen int, protocol uint8, srcIP, dstIP netip.Addr, ok bool) {
	if len(data) < 20 {
		return 0, 0, 0, netip.Addr{}, netip.Addr{}, false
	}
	hdrLen = int(data[0]&0x0F) * 4
	if hdrLen < 20 || hdrLen > len(data) {
		return 0, 0, 0, netip.Addr{}, netip.Addr{}, false
	}
	totalLen = int(binary.BigEndian.Uint16(data[2:4]))
	if totalLen < hdrLen || totalLen > len(data) {
		totalLen = len(data)
	}
	protocol = data[9]
	srcIP, _ = netip.AddrFromSlice(data[12:16])
	dstIP, _ = netip.AddrFromSlice(data[16:20])
	return hdrLen, totalLen, protocol, srcIP, dstIP, true
}

func (s *systemStack) handleTCP(srcIP, dstIP netip.Addr, data []byte) {
	if len(data) < 20 {
		return
	}
	srcPort := binary.BigEndian.Uint16(data[0:2])
	dstPort := binary.BigEndian.Uint16(data[2:4])
	flags := data[13]
	hdrLen := int(data[12]>>4) * 4
	if hdrLen < 20 || hdrLen > len(data) {
		return
	}
	payload := data[hdrLen:]

	key := flowKey{
		proto: protocolTCP,
		src:   netip.AddrPortFrom(srcIP, srcPort),
		dst:   netip.AddrPortFrom(dstIP, dstPort),
	}

	s.mu.Lock()
	f := s.tcpFlows[key]
	s.mu.Unlock()

	if flags&tcpFlagSYN != 0 && f == nil {
		s.handleTCPSYN(key, srcIP, dstIP, srcPort, dstPort, data)
		return
	}
	if f == nil {
		return
	}
	f.mu.Lock()
	f.clientNext = binary.BigEndian.Uint32(data[4:8]) + uint32(len(payload))
	if flags&tcpFlagFIN != 0 {
		f.clientNext++
	}
	f.mu.Unlock()

	if flags&tcpFlagRST != 0 {
		f.Close()
		return
	}

	if len(payload) > 0 {
		select {
		case f.incoming <- payload:
		default:
		}
	}

	if flags&tcpFlagFIN != 0 {
		f.Close()
	}
}

// ponytail: SYN-ACK sent before outbound dial completes; if dial fails the client retransmits
func (s *systemStack) handleTCPSYN(key flowKey, srcIP, dstIP netip.Addr, srcPort, dstPort uint16, data []byte) {
	clientISN := binary.BigEndian.Uint32(data[4:8])
	serverISN := rand.Uint32()

	f := &tcpFlowConn{
		key:       key,
		stack:     s,
		srcIP:     srcIP,
		dstIP:     dstIP,
		srcPort:   srcPort,
		dstPort:   dstPort,
		clientISN: clientISN,
		serverISN: serverISN,
		clientNext: clientISN + 1,
		serverNext: serverISN + 1,
		incoming:  make(chan []byte, 1024),
	}

	s.mu.Lock()
	if _, exists := s.tcpFlows[key]; exists {
		s.mu.Unlock()
		return
	}
	s.tcpFlows[key] = f
	s.mu.Unlock()

	s.writeRawPacket(buildTCPPacket(dstIP, srcIP, dstPort, srcPort,
		serverISN, clientISN+1, tcpFlagSYN|tcpFlagACK, nil))

	dest := xnet.TCPDestination(xnet.IPAddress(dstIP.AsSlice()), xnet.Port(dstPort))
	go s.handler.HandleConnection(f, dest)
}

func (s *systemStack) handleUDP(srcIP, dstIP netip.Addr, data []byte) {
	if len(data) < 8 {
		return
	}
	srcPort := binary.BigEndian.Uint16(data[0:2])
	dstPort := binary.BigEndian.Uint16(data[2:4])
	length := int(binary.BigEndian.Uint16(data[4:6]))
	if length < 8 || length > len(data) {
		return
	}
	payload := data[8:length]

	key := flowKey{
		proto: protocolUDP,
		src:   netip.AddrPortFrom(srcIP, srcPort),
		dst:   netip.AddrPortFrom(dstIP, dstPort),
	}

	s.mu.Lock()
	f, ok := s.udpFlows[key]
	s.mu.Unlock()
	if ok {
		select {
		case f.incoming <- payload:
		default:
		}
		return
	}

	f = &udpFlowConn{
		key:      key,
		stack:    s,
		srcIP:    srcIP,
		dstIP:    dstIP,
		srcPort:  srcPort,
		dstPort:  dstPort,
		incoming: make(chan []byte, 1024),
	}
	s.mu.Lock()
	s.udpFlows[key] = f
	s.mu.Unlock()

	select {
	case f.incoming <- payload:
	default:
	}

	dest := xnet.UDPDestination(xnet.IPAddress(dstIP.AsSlice()), xnet.Port(dstPort))
	go s.handler.HandleConnection(f, dest)
}

func (s *systemStack) handleICMPv4(data []byte) {
	if len(data) < 20 {
		return
	}
	ipHdrLen := int(data[0]&0x0F) * 4
	icmpData := data[ipHdrLen:]
	if len(icmpData) < 4 || icmpData[0] != 8 {
		return
	}
	srcIP, _ := netip.AddrFromSlice(data[12:16])
	dstIP, _ := netip.AddrFromSlice(data[16:20])
	ident, sequence, ok := tunicmp.ParseEchoRequest(header.IPv4ProtocolNumber, icmpData)
	if !ok {
		return
	}
	reply, err := tunicmp.BuildLocalEchoReply(header.IPv4ProtocolNumber, icmpData, tcpip.AddrFromSlice(srcIP.AsSlice()), tcpip.AddrFromSlice(dstIP.AsSlice()))
	if err != nil {
		return
	}
	errors.LogDebug(s.ctx, "[tun][icmp] v4 local echo reply ", dstIP, " -> ", srcIP, " id=", ident, " seq=", sequence)
	packet := buildIPv4(protocolICMPv4, dstIP, srcIP, reply)
	s.writeRawPacket(packet)
}

func (s *systemStack) handleICMPv6(data []byte) {
	if len(data) < 40 {
		return
	}
	icmpData := data[40:]
	if len(icmpData) < 4 || icmpData[0] != 128 {
		return
	}
	srcIP, _ := netip.AddrFromSlice(data[8:24])
	dstIP, _ := netip.AddrFromSlice(data[24:40])
	ident, sequence, ok := tunicmp.ParseEchoRequest(header.IPv6ProtocolNumber, icmpData)
	if !ok {
		return
	}
	reply, err := tunicmp.BuildLocalEchoReply(header.IPv6ProtocolNumber, icmpData, tcpip.AddrFromSlice(srcIP.AsSlice()), tcpip.AddrFromSlice(dstIP.AsSlice()))
	if err != nil {
		return
	}
	errors.LogDebug(s.ctx, "[tun][icmp] v6 local echo reply ", dstIP, " -> ", srcIP, " id=", ident, " seq=", sequence)
	packet := buildIPv6(protocolICMPv6, dstIP, srcIP, reply)
	s.writeRawPacket(packet)
}

func (s *systemStack) removeFlow(key flowKey) {
	s.mu.Lock()
	delete(s.tcpFlows, key)
	s.mu.Unlock()
}

func (s *systemStack) removeUDPFlow(key flowKey) {
	s.mu.Lock()
	delete(s.udpFlows, key)
	s.mu.Unlock()
}

func (s *systemStack) writeRawPacket(data []byte) {
	_ = s.pio.WritePacket(data)
}

func (c *tcpFlowConn) Read(b []byte) (int, error) {
	data, ok := <-c.incoming
	if !ok {
		return 0, io.EOF
	}
	return copy(b, data), nil
}

func (c *tcpFlowConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	seq := c.serverNext
	c.serverNext += uint32(len(b))
	ack := c.clientNext
	c.mu.Unlock()

	packet := buildTCPPacket(c.dstIP, c.srcIP, c.dstPort, c.srcPort,
		seq, ack, tcpFlagACK, b)
	err := c.stack.pio.WritePacket(packet)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *tcpFlowConn) Close() error {
	c.closeOnce.Do(func() {
		seq := c.serverNext
		ack := c.clientNext

		packet := buildTCPPacket(c.dstIP, c.srcIP, c.dstPort, c.srcPort,
			seq, ack, tcpFlagFIN|tcpFlagACK, nil)
		c.stack.pio.WritePacket(packet)
		close(c.incoming)
		c.stack.removeFlow(c.key)
	})
	return nil
}

func (c *tcpFlowConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: c.dstIP.AsSlice(), Port: int(c.dstPort)}
}

func (c *tcpFlowConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: c.srcIP.AsSlice(), Port: int(c.srcPort)}
}

func (c *tcpFlowConn) SetDeadline(_ time.Time) error  { return nil }
func (c *tcpFlowConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *tcpFlowConn) SetWriteDeadline(_ time.Time) error { return nil }

func (c *udpFlowConn) Read(b []byte) (int, error) {
	data, ok := <-c.incoming
	if !ok {
		return 0, io.EOF
	}
	return copy(b, data), nil
}

func (c *udpFlowConn) Write(b []byte) (int, error) {
	packet := buildUDPPacket(c.dstIP, c.srcIP, c.dstPort, c.srcPort, b)
	err := c.stack.pio.WritePacket(packet)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *udpFlowConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.incoming)
		c.stack.removeUDPFlow(c.key)
	})
	return nil
}

func (c *udpFlowConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: c.dstIP.AsSlice(), Port: int(c.dstPort)}
}

func (c *udpFlowConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: c.srcIP.AsSlice(), Port: int(c.srcPort)}
}

func (c *udpFlowConn) SetDeadline(_ time.Time) error  { return nil }
func (c *udpFlowConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *udpFlowConn) SetWriteDeadline(_ time.Time) error { return nil }

func buildTCPPacket(srcIP, dstIP netip.Addr, srcPort, dstPort uint16, seq, ack uint32, flags byte, payload []byte) []byte {
	tcpLen := 20 + len(payload)
	tcpHdr := make([]byte, tcpLen)
	binary.BigEndian.PutUint16(tcpHdr[0:2], srcPort)
	binary.BigEndian.PutUint16(tcpHdr[2:4], dstPort)
	binary.BigEndian.PutUint32(tcpHdr[4:8], seq)
	binary.BigEndian.PutUint32(tcpHdr[8:12], ack)
	tcpHdr[12] = 0x50
	tcpHdr[13] = flags
	binary.BigEndian.PutUint16(tcpHdr[14:16], 65535)
	copy(tcpHdr[20:], payload)

	sum := tcpChecksum(srcIP, dstIP, tcpHdr)
	tcpHdr[16] = byte(sum >> 8)
	tcpHdr[17] = byte(sum)

	if srcIP.Is4() {
		return buildIPv4(protocolTCP, srcIP, dstIP, tcpHdr)
	}
	return buildIPv6(protocolTCP, srcIP, dstIP, tcpHdr)
}

func buildUDPPacket(srcIP, dstIP netip.Addr, srcPort, dstPort uint16, payload []byte) []byte {
	udpLen := 8 + len(payload)
	seg := make([]byte, udpLen)
	binary.BigEndian.PutUint16(seg[0:2], srcPort)
	binary.BigEndian.PutUint16(seg[2:4], dstPort)
	binary.BigEndian.PutUint16(seg[4:6], uint16(udpLen))
	copy(seg[8:], payload)

	if srcIP.Is4() {
		return buildIPv4(protocolUDP, srcIP, dstIP, seg)
	}
	return buildIPv6(protocolUDP, srcIP, dstIP, seg)
}

func buildIPv4(protocol uint8, srcIP, dstIP netip.Addr, payload []byte) []byte {
	totalLen := 20 + len(payload)
	pkt := make([]byte, totalLen)
	pkt[0] = 0x45
	pkt[1] = 0
	binary.BigEndian.PutUint16(pkt[2:4], uint16(totalLen))
	binary.BigEndian.PutUint16(pkt[4:6], 0)
	binary.BigEndian.PutUint16(pkt[6:8], 0)
	pkt[8] = defaultTTL
	pkt[9] = protocol
	src := srcIP.AsSlice()
	dst := dstIP.AsSlice()
	copy(pkt[12:16], src)
	copy(pkt[16:20], dst)
	copy(pkt[20:], payload)
	cs := ipv4Checksum(pkt[:20])
	pkt[10] = byte(cs >> 8)
	pkt[11] = byte(cs)
	return pkt
}

func buildIPv6(nextHeader uint8, srcIP, dstIP netip.Addr, payload []byte) []byte {
	totalLen := 40 + len(payload)
	pkt := make([]byte, totalLen)
	pkt[0] = 0x60
	binary.BigEndian.PutUint16(pkt[4:6], uint16(len(payload)))
	pkt[6] = nextHeader
	pkt[7] = defaultTTL
	src := srcIP.AsSlice()
	dst := dstIP.AsSlice()
	copy(pkt[8:24], src)
	copy(pkt[24:40], dst)
	copy(pkt[40:], payload)
	return pkt
}

func ipv4Checksum(hdr []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(hdr); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(hdr[i:]))
	}
	for sum > 0xFFFF {
		sum = (sum >> 16) + (sum & 0xFFFF)
	}
	return uint16(^sum)
}

func tcpChecksum(srcIP, dstIP netip.Addr, segment []byte) uint16 {
	var sum uint32
	src := srcIP.AsSlice()
	dst := dstIP.AsSlice()
	sum = pseudoSum(src, dst, protocolTCP, uint32(len(segment)))
	for i := 0; i+1 < len(segment); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(segment[i:]))
	}
	if len(segment)%2 == 1 {
		sum += uint32(segment[len(segment)-1]) << 8
	}
	for sum > 0xFFFF {
		sum = (sum >> 16) + (sum & 0xFFFF)
	}
	return uint16(^sum)
}

func pseudoSum(src, dst []byte, protocol uint8, length uint32) uint32 {
	var sum uint32
	for i := 0; i+1 < len(src); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(src[i:]))
	}
	for i := 0; i+1 < len(dst); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(dst[i:]))
	}
	sum += uint32(protocol)
	sum += length
	return sum
}
