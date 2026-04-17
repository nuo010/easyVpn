//go:build !linux

package tun

import "fmt"

type Device struct {
	name string
}

func Open(name string) (*Device, error) {
	return nil, fmt.Errorf("TUN is currently supported only on Linux")
}

func (d *Device) Name() string {
	if d == nil {
		return ""
	}
	return d.name
}

func (d *Device) ReadPacket(_ []byte) (int, error) {
	return 0, fmt.Errorf("TUN is currently supported only on Linux")
}

func (d *Device) WritePacket(_ []byte) (int, error) {
	return 0, fmt.Errorf("TUN is currently supported only on Linux")
}

func (d *Device) Configure(_ string, _ int) error {
	return fmt.Errorf("TUN is currently supported only on Linux")
}

func (d *Device) Close() error {
	return nil
}
