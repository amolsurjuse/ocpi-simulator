package ws

import (
	"bufio"
	"encoding/binary"
	"io"
)

func readFrame(r *bufio.ReadWriter) (byte, []byte, error) {
	b1, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	b2, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}

	opcode := b1 & 0x0F
	masked := (b2 & 0x80) != 0
	payloadLen := int64(b2 & 0x7F)

	switch payloadLen {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return 0, nil, err
		}
		payloadLen = int64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return 0, nil, err
		}
		payloadLen = int64(binary.BigEndian.Uint64(ext[:]))
	}

	payload := make([]byte, payloadLen)

	if masked {
		var mask [4]byte
		if _, err := io.ReadFull(r, mask[:]); err != nil {
			return 0, nil, err
		}
		if payloadLen > 0 {
			if _, err := io.ReadFull(r, payload); err != nil {
				return 0, nil, err
			}
			for i := int64(0); i < payloadLen; i++ {
				payload[i] ^= mask[i%4]
			}
		}
	} else if payloadLen > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return 0, nil, err
		}
	}

	return opcode, payload, nil
}

func writeTextFrame(w *bufio.ReadWriter, payload []byte) error {
	finOpcode := byte(0x81)
	if err := w.WriteByte(finOpcode); err != nil {
		return err
	}

	length := len(payload)
	switch {
	case length <= 125:
		if err := w.WriteByte(byte(length)); err != nil {
			return err
		}
	case length <= 65535:
		if err := w.WriteByte(126); err != nil {
			return err
		}
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(length))
		if _, err := w.Write(ext[:]); err != nil {
			return err
		}
	default:
		if err := w.WriteByte(127); err != nil {
			return err
		}
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(length))
		if _, err := w.Write(ext[:]); err != nil {
			return err
		}
	}

	if _, err := w.Write(payload); err != nil {
		return err
	}
	return w.Flush()
}

func writeControlFrame(w *bufio.ReadWriter, opcode byte, payload []byte) error {
	finOpcode := 0x80 | (opcode & 0x0F)
	if err := w.WriteByte(finOpcode); err != nil {
		return err
	}
	length := len(payload)
	if length > 125 {
		length = 125
	}
	if err := w.WriteByte(byte(length)); err != nil {
		return err
	}
	if length > 0 {
		if _, err := w.Write(payload[:length]); err != nil {
			return err
		}
	}
	return w.Flush()
}
