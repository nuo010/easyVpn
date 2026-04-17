package packet

import "testing"

func TestDestinationIPv4(t *testing.T) {
	packet := []byte{
		0x45, 0x00, 0x00, 0x14,
		0x00, 0x00, 0x00, 0x00,
		0x40, 0x11, 0x00, 0x00,
		192, 0, 2, 10,
		203, 0, 113, 5,
	}

	got, err := Destination(packet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.String() != "203.0.113.5" {
		t.Fatalf("unexpected destination: %s", got)
	}
}

func TestDestinationIPv6(t *testing.T) {
	packet := make([]byte, 40)
	packet[0] = 0x60
	copy(packet[8:24], []byte{
		0x20, 0x01, 0x0d, 0xb8,
		0, 0, 0, 0,
		0, 0, 0, 0,
		0, 0, 0, 1,
	})
	copy(packet[24:40], []byte{
		0x20, 0x01, 0x0d, 0xb8,
		0, 0, 0, 0,
		0, 0, 0, 0,
		0, 0, 0, 2,
	})

	got, err := Destination(packet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.String() != "2001:db8::2" {
		t.Fatalf("unexpected destination: %s", got)
	}
}
