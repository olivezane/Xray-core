package tun

import (
	"strings"
	"time"
)

// StackType constants
const (
	StackTypeGVisor = "gvisor"
	StackTypeSystem = "system"
)

// stackTypeFromString parses a stack type string, defaulting to gVisor
func stackTypeFromString(s string) string {
	switch strings.ToLower(s) {
	case StackTypeSystem:
		return StackTypeSystem
	default:
		return StackTypeGVisor
	}
}

// Stack interface implement ip protocol stack, bridging raw network packets and data streams
type Stack interface {
	Start() error
	Close() error
}

// StackOptions for the stack implementation
type StackOptions struct {
	Tun         Tun
	IdleTimeout time.Duration
	StackType   string
}
