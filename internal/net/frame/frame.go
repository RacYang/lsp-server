// Package frame 实现 ADR-0003 约定的紧凑帧头与载荷编解码。
package frame

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	// Magic 为帧同步魔数，固定为 ASCII「LS」。
	Magic uint16 = 0x4c53
	// Version 为协议版本，Phase 1 固定为 1。
	Version uint8 = 1
	// HeaderSize 为帧头字节长度。
	HeaderSize = 9
)

// Header 表示解析后的帧头信息。
type Header struct {
	Magic      uint16
	Version    uint8
	MsgID      uint16
	PayloadLen uint32
	Payload    []byte
}

var (
	// ErrBadMagic 表示魔数不匹配。
	ErrBadMagic = errors.New("bad frame magic")
	// ErrBadVersion 表示版本不支持。
	ErrBadVersion = errors.New("bad frame version")
	// ErrPayloadTooLarge 表示载荷超过安全上限。
	ErrPayloadTooLarge = errors.New("payload too large")
)

// MaxPayload 为单帧最大载荷字节数，防止 OOM。
const MaxPayload = 4 << 20 // 4MiB

// Encode 将 msg_id 与载荷编码为完整帧字节。
func Encode(msgID uint16, payload []byte) []byte {
	if len(payload) > MaxPayload {
		panic(fmt.Sprintf("payload overflow: %d", len(payload)))
	}
	out := make([]byte, HeaderSize+len(payload))
	binary.BigEndian.PutUint16(out[0:2], Magic)
	out[2] = Version
	binary.BigEndian.PutUint16(out[3:5], msgID)
	// 载荷长度已由 MaxPayload 限制，可安全落入 uint32。
	binary.BigEndian.PutUint32(out[5:9], uint32(len(payload))) //nolint:gosec // G115
	copy(out[HeaderSize:], payload)
	return out
}

// ReadFrame 从 r 读取一帧并返回 Header（含载荷副本）。
func ReadFrame(r io.Reader) (Header, error) {
	var h Header
	hdr := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return h, fmt.Errorf("read header: %w", err)
	}
	h.Magic = binary.BigEndian.Uint16(hdr[0:2])
	if h.Magic != Magic {
		return h, fmt.Errorf("%w: got 0x%04x", ErrBadMagic, h.Magic)
	}
	h.Version = hdr[2]
	if h.Version != Version {
		return h, fmt.Errorf("%w: %d", ErrBadVersion, h.Version)
	}
	h.MsgID = binary.BigEndian.Uint16(hdr[3:5])
	h.PayloadLen = binary.BigEndian.Uint32(hdr[5:9])
	if h.PayloadLen > MaxPayload {
		return h, fmt.Errorf("%w: %d", ErrPayloadTooLarge, h.PayloadLen)
	}
	if h.PayloadLen == 0 {
		return h, nil
	}
	h.Payload = make([]byte, h.PayloadLen)
	if _, err := io.ReadFull(r, h.Payload); err != nil {
		return h, fmt.Errorf("read payload: %w", err)
	}
	return h, nil
}
