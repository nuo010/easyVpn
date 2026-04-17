package packet

import (
	"fmt"
	"net/netip"
)

func Source(packet []byte) (netip.Addr, error) {
	return parseAddr(packet, true)
}

func Destination(packet []byte) (netip.Addr, error) {
	return parseAddr(packet, false)
}

func parseAddr(packet []byte, source bool) (netip.Addr, error) {
	if len(packet) < 1 {
		return netip.Addr{}, fmt.Errorf("empty packet")
	}

	version := packet[0] >> 4
	switch version {
	case 4:
		if len(packet) < 20 {
			return netip.Addr{}, fmt.Errorf("short IPv4 packet")
		}
		offset := 16
		if source {
			offset = 12
		}
		var raw [4]byte
		copy(raw[:], packet[offset:offset+4])
		return netip.AddrFrom4(raw), nil
	case 6:
		if len(packet) < 40 {
			return netip.Addr{}, fmt.Errorf("short IPv6 packet")
		}
		offset := 24
		if source {
			offset = 8
		}
		var raw [16]byte
		copy(raw[:], packet[offset:offset+16])
		return netip.AddrFrom16(raw), nil
	default:
		return netip.Addr{}, fmt.Errorf("unsupported IP version %d", version)
	}
}
