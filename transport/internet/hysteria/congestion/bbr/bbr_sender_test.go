package bbr

import (
	"testing"

	"github.com/apernet/quic-go/congestion"
)

func requireEqual(t *testing.T, want, got any) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSetMaxDatagramSizeRescalesPacketSizedWindows(t *testing.T) {
	const oldMaxDatagramSize = congestion.ByteCount(1000)
	const newMaxDatagramSize = congestion.ByteCount(1400)
	const initialCongestionWindowPackets = congestion.ByteCount(20)
	const maxCongestionWindowPackets = congestion.ByteCount(80)

	b := newBbrSender(
		DefaultClock{},
		oldMaxDatagramSize,
		initialCongestionWindowPackets*oldMaxDatagramSize,
		maxCongestionWindowPackets*oldMaxDatagramSize,
		ProfileStandard,
	)
	b.congestionWindow = b.initialCongestionWindow

	b.SetMaxDatagramSize(newMaxDatagramSize)

	requireEqual(t, initialCongestionWindowPackets*newMaxDatagramSize, b.initialCongestionWindow)
	requireEqual(t, maxCongestionWindowPackets*newMaxDatagramSize, b.maxCongestionWindow)
	requireEqual(t, minCongestionWindowPackets*newMaxDatagramSize, b.minCongestionWindow)
	requireEqual(t, initialCongestionWindowPackets*newMaxDatagramSize, b.congestionWindow)
}

func TestSetMaxDatagramSizeClampsCongestionWindow(t *testing.T) {
	const oldMaxDatagramSize = congestion.ByteCount(1000)
	const newMaxDatagramSize = congestion.ByteCount(1400)

	b := NewBbrSender(DefaultClock{}, oldMaxDatagramSize, ProfileStandard)
	b.congestionWindow = b.minCongestionWindow + oldMaxDatagramSize
	b.recoveryWindow = b.minCongestionWindow + oldMaxDatagramSize

	b.SetMaxDatagramSize(newMaxDatagramSize)

	requireEqual(t, b.minCongestionWindow, b.congestionWindow)
	requireEqual(t, b.minCongestionWindow, b.recoveryWindow)
}

func TestNewBbrSenderAppliesProfiles(t *testing.T) {
	testCases := []struct {
		name                                string
		profile                             Profile
		highGain                            float64
		highCwndGain                        float64
		congestionWindowGainConstant        float64
		numStartupRtts                      int64
		drainToTarget                       bool
		detectOvershooting                  bool
		bytesLostMultiplier                 uint8
		enableAckAggregationDuringStartup   bool
		expireAckAggregationInStartup       bool
		enableOverestimateAvoidance         bool
		reduceExtraAckedOnBandwidthIncrease bool
	}{
		{
			name:                         "standard",
			profile:                      ProfileStandard,
			highGain:                     defaultHighGain,
			highCwndGain:                 derivedHighCWNDGain,
			congestionWindowGainConstant: 2.0,
			numStartupRtts:               roundTripsWithoutGrowthBeforeExitingStartup,
			bytesLostMultiplier:          2,
		},
		{
			name:                                "conservative",
			profile:                             ProfileConservative,
			highGain:                            2.25,
			highCwndGain:                        1.75,
			congestionWindowGainConstant:        1.75,
			numStartupRtts:                      2,
			drainToTarget:                       true,
			detectOvershooting:                  true,
			bytesLostMultiplier:                 1,
			enableOverestimateAvoidance:         true,
			reduceExtraAckedOnBandwidthIncrease: true,
		},
		{
			name:                              "aggressive",
			profile:                           ProfileAggressive,
			highGain:                          3.0,
			highCwndGain:                      2.25,
			congestionWindowGainConstant:      2.5,
			numStartupRtts:                    4,
			bytesLostMultiplier:               2,
			enableAckAggregationDuringStartup: true,
			expireAckAggregationInStartup:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBbrSender(DefaultClock{}, congestion.InitialPacketSize, tc.profile)
			requireEqual(t, tc.profile, b.profile)
			requireEqual(t, tc.highGain, b.highGain)
			requireEqual(t, tc.highCwndGain, b.highCwndGain)
			requireEqual(t, tc.congestionWindowGainConstant, b.congestionWindowGainConstant)
			requireEqual(t, tc.numStartupRtts, b.numStartupRtts)
			requireEqual(t, tc.drainToTarget, b.drainToTarget)
			requireEqual(t, tc.detectOvershooting, b.detectOvershooting)
			requireEqual(t, tc.bytesLostMultiplier, b.bytesLostMultiplierWhileDetectingOvershooting)
			requireEqual(t, tc.enableAckAggregationDuringStartup, b.enableAckAggregationDuringStartup)
			requireEqual(t, tc.expireAckAggregationInStartup, b.expireAckAggregationInStartup)
			requireEqual(t, tc.enableOverestimateAvoidance, b.sampler.IsOverestimateAvoidanceEnabled())
			requireEqual(t, tc.reduceExtraAckedOnBandwidthIncrease, b.sampler.maxAckHeightTracker.reduceExtraAckedOnBandwidthIncrease)
			requireEqual(t, b.highGain, b.pacingGain)
			requireEqual(t, b.highCwndGain, b.congestionWindowGain)
		})
	}
}

func TestParseProfile(t *testing.T) {
	profile, err := ParseProfile("")
	if err != nil {
		t.Fatal(err)
	}
	requireEqual(t, ProfileStandard, profile)

	profile, err = ParseProfile("Aggressive")
	if err != nil {
		t.Fatal(err)
	}
	requireEqual(t, ProfileAggressive, profile)

	_, err = ParseProfile("turbo")
	if err == nil || err.Error() != `unsupported BBR profile "turbo"` {
		t.Fatalf("got %v, want unsupported BBR profile error", err)
	}
}
