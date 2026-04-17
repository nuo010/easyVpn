package server

import (
	"fmt"
	"net/netip"
)

type IPAllocator struct {
	prefix    netip.Prefix
	serverIP  netip.Addr
	allocated map[netip.Addr]string
}

func NewIPAllocator(tunnelAddress string) (*IPAllocator, error) {
	prefix, err := netip.ParsePrefix(tunnelAddress)
	if err != nil {
		return nil, fmt.Errorf("parse tunnel_address: %w", err)
	}

	return &IPAllocator{
		prefix:    prefix.Masked(),
		serverIP:  prefix.Addr(),
		allocated: make(map[netip.Addr]string),
	}, nil
}

func (a *IPAllocator) ServerAddress() string {
	return a.serverIP.String()
}

func (a *IPAllocator) PrefixBits() int {
	return a.prefix.Bits()
}

func (a *IPAllocator) Allocate(ownerID, requested string) (string, error) {
	if requested != "" {
		addr, err := parseAssignedAddr(requested)
		if err != nil {
			return "", err
		}
		if err := a.reserve(ownerID, addr); err != nil {
			return "", err
		}
		return netip.PrefixFrom(addr, a.prefix.Bits()).String(), nil
	}

	for candidate := a.prefix.Addr().Next(); a.prefix.Contains(candidate); candidate = candidate.Next() {
		if candidate == a.serverIP || candidate == a.prefix.Addr() {
			continue
		}
		if _, ok := a.allocated[candidate]; ok {
			continue
		}
		a.allocated[candidate] = ownerID
		return netip.PrefixFrom(candidate, a.prefix.Bits()).String(), nil
	}

	return "", fmt.Errorf("tunnel address pool %s is exhausted", a.prefix)
}

func (a *IPAllocator) Release(ownerID, assigned string) {
	addr, err := parseAssignedAddr(assigned)
	if err != nil {
		return
	}
	currentOwner, ok := a.allocated[addr]
	if !ok || currentOwner != ownerID {
		return
	}
	delete(a.allocated, addr)
}

func (a *IPAllocator) reserve(ownerID string, addr netip.Addr) error {
	if !a.prefix.Contains(addr) {
		return fmt.Errorf("requested client address %s is outside pool %s", addr, a.prefix)
	}
	if addr == a.serverIP {
		return fmt.Errorf("requested client address %s conflicts with server tunnel IP", addr)
	}
	if currentOwner, ok := a.allocated[addr]; ok && currentOwner != ownerID {
		return fmt.Errorf("requested client address %s is already in use", addr)
	}
	a.allocated[addr] = ownerID
	return nil
}

func parseAssignedAddr(raw string) (netip.Addr, error) {
	if prefix, err := netip.ParsePrefix(raw); err == nil {
		return prefix.Addr(), nil
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("parse assigned client address %q: %w", raw, err)
	}
	return addr, nil
}
