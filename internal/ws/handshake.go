package ws

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"strings"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func Upgrade(w http.ResponseWriter, r *http.Request) (net.Conn, *bufio.ReadWriter, error) {
	if !headerContains(r.Header, "Connection", "upgrade") || !headerContains(r.Header, "Upgrade", "websocket") {
		return nil, nil, errors.New("not a websocket upgrade request")
	}

	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		return nil, nil, errors.New("missing Sec-WebSocket-Key")
	}

	accept := computeAccept(key)

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijacking not supported")
	}

	conn, rw, err := hijacker.Hijack()
	if err != nil {
		return nil, nil, err
	}

	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"

	if _, err := rw.WriteString(response); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	if err := rw.Flush(); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}

	return conn, rw, nil
}

func computeAccept(key string) string {
	sum := sha1.Sum([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func headerContains(header http.Header, name, value string) bool {
	for _, v := range header.Values(name) {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), value) {
				return true
			}
		}
	}
	return false
}
