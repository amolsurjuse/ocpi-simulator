package ws

import (
	"bufio"
	"errors"
	"net"
)

type Conn struct {
	conn net.Conn
	rw   *bufio.ReadWriter
}

func NewConn(conn net.Conn, rw *bufio.ReadWriter) *Conn {
	return &Conn{conn: conn, rw: rw}
}

func (c *Conn) ReadText() (string, error) {
	for {
		opcode, payload, err := readFrame(c.rw)
		if err != nil {
			return "", err
		}
		switch opcode {
		case 0x1:
			return string(payload), nil
		case 0x8:
			return "", errors.New("client closed")
		case 0x9:
			if err := writeControlFrame(c.rw, 0xA, payload); err != nil {
				return "", err
			}
		default:
			continue
		}
	}
}

func (c *Conn) WriteText(payload string) error {
	return writeTextFrame(c.rw, []byte(payload))
}

func (c *Conn) Close() error {
	return c.conn.Close()
}
