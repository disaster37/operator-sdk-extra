package helper

import (
	"crypto/rand"
	"math/big"
)

const charset = "abcdefghijklmnopqrstuvwxyz"

func randomStringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))
	for i := range b {
		n, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			panic("crypto/rand failed: " + err.Error())
		}
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

// RandomString generates a random string from a subset of characters
func RandomString(length int) string {
	return randomStringWithCharset(length, charset)
}
