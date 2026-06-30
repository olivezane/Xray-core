# Changes since Xray-core v26.6.27 (45cf2898)

## QUIC v2 Sniff Support
- Added QUIC version 2 (`0x6b3343cf`) recognition in `common/protocol/quic/sniff.go`
- Added `quicSaltV2` for QUIC v2 initial packet decryption

## WebRTC Sniff
- New file: `common/protocol/webrtc/webrtc.go`
- Detects STUN packets (magic cookie `0x2112A442`) as WebRTC traffic indicator
- Registered as UDP sniffer in dispatcher

## System Stack for TUN
New alternative TCP/IP stack implementation using host OS network stack (raw sockets/Winsock) instead of gVisor userspace stack. Selectable via `stackType` config field.

### New files
- `proxy/tun/stack.go` — `StackType` constants, `stackTypeFromString()` parser, `StackOptions` extended with `StackType`
- `proxy/tun/stack_system.go` (~600 lines) — core system stack: raw IP packet parsing, TCP flow state machine (SYN-SYN/ACK handshake, seq/ack translation, FIN/RST handling), UDP relay, ICMP echo reply, IPv4/IPv6 checksum helpers
- `proxy/tun/stack_system_windows.go` — Wintun packet I/O via `session.ReceivePacket`/`SendPacket`
- `proxy/tun/stack_system_linux.go` — raw socket fd read/write via `unix.Read`/`unix.Write`
- `proxy/tun/stack_system_android.go` — Android TUN fd read/write
- `proxy/tun/stack_system_bsd.go` — Darwin/FreeBSD with 4-byte protocol prefix handling
- `proxy/tun/stack_system_default.go` — unsupported platform error

### Modified files
- `proxy/tun/config.proto` / `config.pb.go` — added `stack_type` field
- `infra/conf/tun.go` — added `StackType` config option
- `proxy/tun/stack_gvisor.go` — `NewStack()` dispatches to gVisor or system stack based on config
- `proxy/tun/handler.go` — passes `StackType` to options
- `proxy/tun/tun_darwin.go` — added `File()` method for `DarwinTun`
- `proxy/tun/tun_freebsd.go` — added `File()` method for `FreeBSDTun`

## Distribute IP Address on Linux
- `proxy/tun/tun_linux.go`: `setup()` now accepts `addresses []netip.Prefix` and calls `netlink.AddrAdd()` for each gateway address on the TUN interface
- `NewTun()` parses gateway addresses into `netip.Prefix` before calling `setup()`

## Auto Outbound Interface Fallback (Linux)
- `proxy/tun/tun_linux.go`: `findOutboundInterface()` falls back to default route lookup when probe IPs are unreachable

## Auto Outbound Interface Retry
- `proxy/tun/handler.go`: `Start()` retries interface lookup up to 30 seconds before registering dialer controller

## Logger Rewrite
- `common/log/logger.go`: replaced `log.Logger` wrapper with direct `io.Writer` implementation
- Console writer: custom timestamp format (`2006-01-02T15:04:05.000Z07:00`) + colorized output (red for Error, yellow for Warning, green for Info, gray for Debug)
- File writer: size-based log rotation (10MB max, 3 numbered backups `.1`, `.2`, `.3`)

## Error Caller Formatting
- `common/errors/errors.go`: caller info changed from function-name-based to file:line format using `runtime.Caller` file path
- `shortFile()` helper strips the base path, showing path relative to `xray-core/`

## Code Cleanup
- Removed unused `HasLatency` interface from `common/peer/latency.go`
- Deleted empty package files `common/peer/peer.go`, `common/protocol/udp/udp.go`, `common/drain/drain.go` (merged into `drainer.go`)
- Removed `go-cmp` dependency from error tests
- Fixed typo: `OPTIMISTE` → `OPTIMISTIC` in DNS log
- Removed stale "do not remove" comments

## Configuration
```json
{
  "inbounds": [
    {
      "protocol": "tun",
      "settings": {
        "stackType": "gvisor",
        "...": "..."
      }
    }
  ]
}
```
- `stackType`: `"gvisor"` (default) or `"system"`
- System stack currently supported on Windows, Linux, Android, Darwin, FreeBSD
