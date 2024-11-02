package totp

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"time"
)

const secretSize = 20
const validInterval = 30
const codeDigits = 1000000

func NewSecret() string {
	secret := make([]byte, secretSize)
	rand.Read(secret)
	var buf bytes.Buffer
	enc := base32.NewEncoder(base32.StdEncoding, &buf)
	enc.Write(secret)
	enc.Close()
	return buf.String()
}

func GenerateCode(secret string) int {
	key, err := base32.StdEncoding.DecodeString(secret)
	if err != nil {
		return -1
	}
	now := time.Now().Unix()
	now /= validInterval
	return generateCodeCounter(key, now)
}

func generateCodeCounter(key []byte, when int64) int {
	mac := hmac.New(sha1.New, key)
	binary.Write(mac, binary.BigEndian, when)
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0xf
	code := ((int(sum[offset]) & 0x7f) << 24) |
		((int(sum[offset+1] & 0xff)) << 16) |
		((int(sum[offset+2] & 0xff)) << 8) |
		(int(sum[offset+3]) & 0xff)
	return code % codeDigits
}

func CheckCode(secret string, code int) bool {
	key, err := base32.StdEncoding.DecodeString(secret)
	if err != nil {
		return false
	}
	if code < 0 || code >= codeDigits {
		return false
	}
	now := time.Now().Unix()
	now /= validInterval
	c1 := generateCodeCounter(key, now-1)
	c2 := generateCodeCounter(key, now)
	okay := (code == c1) || (code == c2)
	return okay
}
