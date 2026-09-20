// Package p2p defines the reviewed intent for the one-way Hybrid transition.
package p2p

import (
	"fmt"
	"net/netip"
)

// UpgradeRequest binds consent to one IPv4 address. The host must independently
// observe the same address before applying it; this is not an address override.
type UpgradeRequest struct{ address netip.Addr }

func NewUpgradeRequest(address string) (UpgradeRequest, error) {
	ip, err := netip.ParseAddr(address)
	if err != nil || !ip.Is4() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return UpgradeRequest{}, fmt.Errorf("hybrid P2P requires a public IPv4 address")
	}
	return UpgradeRequest{address: ip}, nil
}

func (r UpgradeRequest) Address() string {
	if !r.address.IsValid() {
		return ""
	}
	return r.address.String()
}
