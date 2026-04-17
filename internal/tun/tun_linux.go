//go:build linux

package tun

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const (
	tunSetIFF = 0x400454ca
	iffTUN    = 0x0001
	iffNoPI   = 0x1000
)

type ifreq struct {
	Name  [syscall.IFNAMSIZ]byte
	Flags uint16
	Pad   [22]byte
}

type Device struct {
	file *os.File
	name string
}

func Open(name string) (*Device, error) {
	fd, err := syscall.Open("/dev/net/tun", syscall.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open /dev/net/tun: %w", err)
	}

	req := ifreq{Flags: iffTUN | iffNoPI}
	copy(req.Name[:], name)

	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(tunSetIFF), uintptr(unsafe.Pointer(&req)))
	if errno != 0 {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("ioctl TUNSETIFF: %w", errno)
	}

	actualName := strings.TrimRight(string(req.Name[:]), "\x00")
	return &Device{
		file: os.NewFile(uintptr(fd), "/dev/net/tun"),
		name: actualName,
	}, nil
}

func (d *Device) Name() string {
	return d.name
}

func (d *Device) ReadPacket(buffer []byte) (int, error) {
	return d.file.Read(buffer)
}

func (d *Device) WritePacket(packet []byte) (int, error) {
	return d.file.Write(packet)
}

func (d *Device) Configure(address string, mtu int) error {
	if address != "" {
		if err := runIP("addr", "replace", address, "dev", d.name); err != nil {
			return err
		}
	}
	if mtu > 0 {
		if err := runIP("link", "set", "dev", d.name, "mtu", strconv.Itoa(mtu)); err != nil {
			return err
		}
	}
	return runIP("link", "set", "dev", d.name, "up")
}

func (d *Device) Close() error {
	return d.file.Close()
}

func runIP(args ...string) error {
	output, err := exec.Command("ip", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}
