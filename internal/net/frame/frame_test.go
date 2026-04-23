// 帧编解码单元测试，覆盖魔数、版本、载荷边界与过大载荷错误路径。
package frame

import (
	"bytes"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	payload := []byte{1, 2, 3}
	b := Encode(9, payload)
	h, err := ReadFrame(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if h.MsgID != 9 || string(h.Payload) != string(payload) {
		t.Fatalf("got %+v", h)
	}
}

func TestBadMagic(t *testing.T) {
	b := []byte{0, 0, 1, 0, 9, 0, 0, 0, 0}
	_, err := ReadFrame(bytes.NewReader(b))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBadVersion(t *testing.T) {
	b := make([]byte, 9)
	b[0], b[1] = 0x4c, 0x53
	b[2] = 9
	_, err := ReadFrame(bytes.NewReader(b))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPayloadTooLarge(t *testing.T) {
	b := make([]byte, 9)
	b[0], b[1] = 0x4c, 0x53
	b[2] = 1
	b[3], b[4] = 0, 1
	b[5], b[6], b[7], b[8] = 0xff, 0xff, 0xff, 0xff
	_, err := ReadFrame(bytes.NewReader(b))
	if err == nil {
		t.Fatal("expected error")
	}
}
