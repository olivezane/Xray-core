package fakedns

import (
	"strconv"
	"testing"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/uuid"
	"github.com/xtls/xray-core/features/dns"
	"golang.org/x/sync/errgroup"
)

var ipPrefix = "198.1"

func requireEqual(t *testing.T, want, got any) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func requireNotEqual(t *testing.T, a, b any) {
	t.Helper()
	if a == b {
		t.Fatalf("got equal values %v", a)
	}
}

func requireBool(t *testing.T, want, got bool) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func requireLen(t *testing.T, got []net.Address, want int) {
	t.Helper()
	if len(got) != want {
		t.Fatalf("got length %d, want %d", len(got), want)
	}
}

func TestNewFakeDnsHolder(_ *testing.T) {
	_, err := NewFakeDNSHolder()
	common.Must(err)
}

func TestFakeDnsHolderCreateMapping(t *testing.T) {
	fkdns, err := NewFakeDNSHolder()
	common.Must(err)

	addr := fkdns.GetFakeIPForDomain("fakednstest.example.com")
	requireEqual(t, ipPrefix, addr[0].IP().String()[0:len(ipPrefix)])
}

func TestFakeDnsHolderCreateMappingMany(t *testing.T) {
	fkdns, err := NewFakeDNSHolder()
	common.Must(err)

	addr := fkdns.GetFakeIPForDomain("fakednstest.example.com")
	requireEqual(t, ipPrefix, addr[0].IP().String()[0:len(ipPrefix)])

	addr2 := fkdns.GetFakeIPForDomain("fakednstest2.example.com")
	requireEqual(t, ipPrefix, addr2[0].IP().String()[0:len(ipPrefix)])
	requireNotEqual(t, addr[0].IP().String(), addr2[0].IP().String())
}

func TestFakeDnsHolderCreateMappingManyAndResolve(t *testing.T) {
	fkdns, err := NewFakeDNSHolder()
	common.Must(err)

	addr := fkdns.GetFakeIPForDomain("fakednstest.example.com")
	addr2 := fkdns.GetFakeIPForDomain("fakednstest2.example.com")

	{
		result := fkdns.GetDomainFromFakeDNS(addr[0])
		requireEqual(t, "fakednstest.example.com", result)
	}

	{
		result := fkdns.GetDomainFromFakeDNS(addr2[0])
		requireEqual(t, "fakednstest2.example.com", result)
	}
}

func TestFakeDnsHolderCreateMappingManySingleDomain(t *testing.T) {
	fkdns, err := NewFakeDNSHolder()
	common.Must(err)

	addr := fkdns.GetFakeIPForDomain("fakednstest.example.com")
	addr2 := fkdns.GetFakeIPForDomain("fakednstest.example.com")
	requireEqual(t, addr[0].IP().String(), addr2[0].IP().String())
}

func TestGetFakeIPForDomainConcurrently(t *testing.T) {
	fkdns, err := NewFakeDNSHolder()
	common.Must(err)

	total := 200
	addr := make([][]net.Address, total)
	var errg errgroup.Group
	for i := 0; i < total; i++ {
		errg.Go(testGetFakeIP(i, addr, fkdns))
	}
	errg.Wait()
	for i := 0; i < total; i++ {
		for j := i + 1; j < total; j++ {
			requireNotEqual(t, addr[i][0].IP().String(), addr[j][0].IP().String())
		}
	}
}

func testGetFakeIP(index int, addr [][]net.Address, fkdns *Holder) func() error {
	return func() error {
		addr[index] = fkdns.GetFakeIPForDomain("fakednstest" + strconv.Itoa(index) + ".example.com")
		return nil
	}
}

func TestFakeDnsHolderCreateMappingAndRollOver(t *testing.T) {
	fkdns, err := NewFakeDNSHolderConfigOnly(&FakeDnsPool{
		IpPool:  dns.FakeIPv4Pool,
		LruSize: 256,
	})
	common.Must(err)

	err = fkdns.Start()

	common.Must(err)

	addr := fkdns.GetFakeIPForDomain("fakednstest.example.com")
	addr2 := fkdns.GetFakeIPForDomain("fakednstest2.example.com")

	for i := 0; i <= 8192; i++ {
		{
			result := fkdns.GetDomainFromFakeDNS(addr[0])
			requireEqual(t, "fakednstest.example.com", result)
		}

		{
			result := fkdns.GetDomainFromFakeDNS(addr2[0])
			requireEqual(t, "fakednstest2.example.com", result)
		}

		{
			uuid := uuid.New()
			domain := uuid.String() + ".fakednstest.example.com"
			tempAddr := fkdns.GetFakeIPForDomain(domain)
			rsaddr := tempAddr[0].IP().String()

			result := fkdns.GetDomainFromFakeDNS(net.ParseAddress(rsaddr))
			requireEqual(t, domain, result)
		}
	}
}

func TestFakeDNSMulti(t *testing.T) {
	fakeMulti, err := NewFakeDNSHolderMulti(
		&FakeDnsPoolMulti{
			Pools: []*FakeDnsPool{{
				IpPool:  "240.0.0.0/12",
				LruSize: 256,
			}, {
				IpPool:  "fddd:c5b4:ff5f:f4f0::/64",
				LruSize: 256,
			}},
		},
	)
	common.Must(err)

	err = fakeMulti.Start()

	common.Must(err)

	if err != nil {
		t.Fatal(err)
	}
	_ = fakeMulti

	t.Run("checkInRange", func(t *testing.T) {
		t.Run("ipv4", func(t *testing.T) {
			inPool := fakeMulti.IsIPInIPPool(net.IPAddress([]byte{240, 0, 0, 5}))
			requireBool(t, true, inPool)
		})
		t.Run("ipv6", func(t *testing.T) {
			ip, err := net.ResolveIPAddr("ip", "fddd:c5b4:ff5f:f4f0::5")
			if err != nil {
				t.Fatal(err)
			}
			inPool := fakeMulti.IsIPInIPPool(net.IPAddress(ip.IP))
			requireBool(t, true, inPool)
		})
		t.Run("ipv4_inverse", func(t *testing.T) {
			inPool := fakeMulti.IsIPInIPPool(net.IPAddress([]byte{241, 0, 0, 5}))
			requireBool(t, false, inPool)
		})
		t.Run("ipv6_inverse", func(t *testing.T) {
			ip, err := net.ResolveIPAddr("ip", "fcdd:c5b4:ff5f:f4f0::5")
			if err != nil {
				t.Fatal(err)
			}
			inPool := fakeMulti.IsIPInIPPool(net.IPAddress(ip.IP))
			requireBool(t, false, inPool)
		})
	})

	t.Run("allocateTwoAddressForTwoPool", func(t *testing.T) {
		address := fakeMulti.GetFakeIPForDomain("fakednstest.example.com")
		requireLen(t, address, 2)
		t.Run("eachOfThemShouldResolve:0", func(t *testing.T) {
			domain := fakeMulti.GetDomainFromFakeDNS(address[0])
			requireEqual(t, "fakednstest.example.com", domain)
		})
		t.Run("eachOfThemShouldResolve:1", func(t *testing.T) {
			domain := fakeMulti.GetDomainFromFakeDNS(address[1])
			requireEqual(t, "fakednstest.example.com", domain)
		})
	})

	t.Run("understandIPTypeSelector", func(t *testing.T) {
		t.Run("ipv4", func(t *testing.T) {
			address := fakeMulti.GetFakeIPForDomain3("fakednstestipv4.example.com", true, false)
			requireLen(t, address, 1)
			requireBool(t, true, address[0].Family().IsIPv4())
		})
		t.Run("ipv6", func(t *testing.T) {
			address := fakeMulti.GetFakeIPForDomain3("fakednstestipv6.example.com", false, true)
			requireLen(t, address, 1)
			requireBool(t, true, address[0].Family().IsIPv6())
		})
		t.Run("ipv46", func(t *testing.T) {
			address := fakeMulti.GetFakeIPForDomain3("fakednstestipv46.example.com", true, true)
			requireLen(t, address, 2)
			requireBool(t, true, address[0].Family().IsIPv4())
			requireBool(t, true, address[1].Family().IsIPv6())
		})
	})
}
