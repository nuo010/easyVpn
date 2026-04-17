package transport

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
)

const maxFrameSize = 1 << 20

type FramedConn struct {
	conn   net.Conn
	reader *bufio.Reader
	mu     sync.Mutex
}

func NewFramedConn(conn net.Conn) *FramedConn {
	return &FramedConn{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}
}

func (c *FramedConn) ReadFrame() ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(c.reader, header); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(header)
	if size == 0 {
		return nil, nil
	}
	if size > maxFrameSize {
		return nil, fmt.Errorf("frame too large: %d", size)
	}

	payload := make([]byte, size)
	if _, err := io.ReadFull(c.reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *FramedConn) WriteFrame(payload []byte) error {
	if len(payload) > maxFrameSize {
		return fmt.Errorf("frame too large: %d", len(payload))
	}

	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(payload)))

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.conn.Write(header); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	_, err := c.conn.Write(payload)
	return err
}

func (c *FramedConn) ReadJSON(target any) error {
	payload, err := c.ReadFrame()
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, target)
}

func (c *FramedConn) WriteJSON(payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.WriteFrame(data)
}

func (c *FramedConn) Close() error {
	return c.conn.Close()
}
