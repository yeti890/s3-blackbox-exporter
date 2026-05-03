package keygen

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

func NewObjectKey(basePrefix, cluster, az string) (string, error) {
	id, err := UUIDv7()
	if err != nil {
		return "", err
	}

	parts := []string{
		safePart(basePrefix),
		safePart(cluster),
		safePart(az),
		id + ".bin",
	}
	return strings.Join(parts, "/"), nil
}

func UUIDv7() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	ms := uint64(time.Now().UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)

	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(b[0:4]),
		binary.BigEndian.Uint16(b[4:6]),
		binary.BigEndian.Uint16(b[6:8]),
		binary.BigEndian.Uint16(b[8:10]),
		b[10:16],
	), nil
}

func safePart(value string) string {
	value = strings.Trim(value, "/ ")
	value = strings.ReplaceAll(value, "/", "-")
	if value == "" {
		return "unknown"
	}
	return value
}
