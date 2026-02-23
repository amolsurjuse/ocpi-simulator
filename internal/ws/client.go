package ws

import (
	"bufio"
	"errors"
	"net"
	"sync"
)

type Client struct {
	conn  net.Conn
	rw    *bufio.ReadWriter
	send  chan []byte
	close sync.Once
}

func NewClient(conn net.Conn, rw *bufio.ReadWriter) *Client {
	return &Client{
		conn: conn,
		rw:   rw,
		send: make(chan []byte, 64),
	}
}

func (c *Client) Run(hub *Hub) {
	hub.Register(c)
	go c.writeLoop(hub)
	c.readLoop(hub)
}

func (c *Client) readLoop(hub *Hub) {
	defer hub.Unregister(c)
	for {
		if err := discardFrame(c.rw); err != nil {
			return
		}
	}
}

func (c *Client) writeLoop(hub *Hub) {
	defer hub.Unregister(c)
	for payload := range c.send {
		if err := writeTextFrame(c.rw, payload); err != nil {
			return
		}
	}
}

func (c *Client) Send(payload []byte) {
	select {
	case c.send <- payload:
	default:
	}
}

func (c *Client) Close() {
	c.close.Do(func() {
		close(c.send)
		_ = c.conn.Close()
	})
}

func discardFrame(r *bufio.ReadWriter) error {
	opcode, payload, err := readFrame(r)
	if err != nil {
		return err
	}
	if opcode == 0x8 {
		return errors.New("client closed")
	}
	if opcode == 0x9 {
		return writeControlFrame(r, 0xA, payload)
	}
	return nil
}
