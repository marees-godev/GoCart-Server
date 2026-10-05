package utils

import (
	"crypto/rand"
	"time"
)

const skuCharset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// GenerateSKU generates a unique random SKU string with prefix "SKU-".
func GenerateSKU() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		now := time.Now().UnixNano()
		for i := range bytes {
			bytes[i] = skuCharset[int(now+int64(i*31))%len(skuCharset)]
		}
	} else {
		for i := range bytes {
			bytes[i] = skuCharset[int(bytes[i])%len(skuCharset)]
		}
	}
	return "SKU-" + string(bytes)
}
