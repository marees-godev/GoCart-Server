package push

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	defaultRecordSize = 4096
	authSecretLength  = 16
	saltLength        = 16
	p256PublicKeyLen  = 65
	aesGCMTagLen      = 16
)

func ValidateSubscriptionKeys(p256dhB64, authB64 string) error {
	pubBytes, err := DecodeURLSafeBase64(p256dhB64)
	if err != nil {
		return fmt.Errorf("invalid p256dh key encoding: %w", err)
	}
	if len(pubBytes) != p256PublicKeyLen {
		return fmt.Errorf("p256dh must decode to %d bytes, got %d", p256PublicKeyLen, len(pubBytes))
	}
	if _, err := ecdh.P256().NewPublicKey(pubBytes); err != nil {
		return fmt.Errorf("invalid P-256 subscriber public key: %w", err)
	}

	authBytes, err := DecodeURLSafeBase64(authB64)
	if err != nil {
		return fmt.Errorf("invalid auth secret encoding: %w", err)
	}
	if len(authBytes) != authSecretLength {
		return fmt.Errorf("auth secret must decode to %d bytes, got %d", authSecretLength, len(authBytes))
	}
	return nil
}

// EncryptPayload encrypts a Web Push message using RFC 8291 (Content-Encoding: aes128gcm).
func EncryptPayload(plaintext []byte, uaPublicB64, authSecretB64 string) ([]byte, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed generating random salt: %w", err)
	}
	asPriv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed generating ephemeral ECDH key: %w", err)
	}
	return encryptPayloadWithParams(plaintext, uaPublicB64, authSecretB64, salt, asPriv, defaultRecordSize)
}

func encryptPayloadWithParams(
	plaintext []byte,
	uaPublicB64 string,
	authSecretB64 string,
	salt []byte,
	asPriv *ecdh.PrivateKey,
	recordSize uint32,
) ([]byte, error) {
	if len(salt) != saltLength {
		return nil, fmt.Errorf("salt must be %d bytes", saltLength)
	}
	if asPriv == nil {
		return nil, errors.New("ephemeral private key cannot be nil")
	}

	uaPubBytes, err := DecodeURLSafeBase64(uaPublicB64)
	if err != nil {
		return nil, fmt.Errorf("invalid subscriber p256dh key: %w", err)
	}
	if len(uaPubBytes) != p256PublicKeyLen {
		return nil, fmt.Errorf("invalid subscriber p256dh length: %d", len(uaPubBytes))
	}
	uaPubKey, err := ecdh.P256().NewPublicKey(uaPubBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid subscriber P-256 key: %w", err)
	}

	authSecret, err := DecodeURLSafeBase64(authSecretB64)
	if err != nil {
		return nil, fmt.Errorf("invalid subscriber auth secret: %w", err)
	}
	if len(authSecret) != authSecretLength {
		return nil, fmt.Errorf("invalid subscriber auth secret length: %d", len(authSecret))
	}

	sharedSecret, err := asPriv.ECDH(uaPubKey)
	if err != nil {
		return nil, fmt.Errorf("ECDH key agreement failed: %w", err)
	}

	asPubBytes := asPriv.PublicKey().Bytes()

	// RFC 8291 Section 3.3: Derive IKM from shared ECDH secret and subscriber auth secret
	prkKey, err := hkdf.Extract(sha256.New, sharedSecret, authSecret)
	if err != nil {
		return nil, fmt.Errorf("HKDF extract key failed: %w", err)
	}

	keyInfo := make([]byte, 0, 14+len(uaPubBytes)+len(asPubBytes))
	keyInfo = append(keyInfo, []byte("WebPush: info\x00")...)
	keyInfo = append(keyInfo, uaPubBytes...)
	keyInfo = append(keyInfo, asPubBytes...)

	ikm, err := hkdf.Expand(sha256.New, prkKey, string(keyInfo), 32)
	if err != nil {
		return nil, fmt.Errorf("HKDF expand ikm failed: %w", err)
	}

	// RFC 8188 / RFC 8291 Section 3.4: Derive Content Encryption Key and Nonce from salt and IKM
	prk, err := hkdf.Extract(sha256.New, ikm, salt)
	if err != nil {
		return nil, fmt.Errorf("HKDF extract cek failed: %w", err)
	}

	cek, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, fmt.Errorf("HKDF expand cek failed: %w", err)
	}

	nonce, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, fmt.Errorf("HKDF expand nonce failed: %w", err)
	}

	// Append final record delimiter 0x02 per RFC 8188 Section 2
	padded := make([]byte, len(plaintext)+1)
	copy(padded, plaintext)
	padded[len(plaintext)] = 0x02

	minRS := uint32(len(padded) + aesGCMTagLen + 1)
	if recordSize < minRS {
		recordSize = minRS
	}

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, fmt.Errorf("aes cipher init failed: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aes gcm init failed: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, padded, nil)

	// Header: salt (16) || rs (4) || idlen (1) || keyid (65)
	headerLen := saltLength + 4 + 1 + len(asPubBytes)
	out := make([]byte, headerLen+len(ciphertext))
	copy(out[:saltLength], salt)
	binary.BigEndian.PutUint32(out[saltLength:saltLength+4], recordSize)
	out[saltLength+4] = byte(len(asPubBytes))
	copy(out[saltLength+5:headerLen], asPubBytes)
	copy(out[headerLen:], ciphertext)

	return out, nil
}

// DecryptPayload decrypts an RFC 8291 (aes128gcm) Web Push payload using the subscriber's private key and auth secret.
func DecryptPayload(encrypted []byte, uaPriv *ecdh.PrivateKey, authSecretB64 string) ([]byte, error) {
	if uaPriv == nil {
		return nil, errors.New("subscriber private key cannot be nil")
	}
	if len(encrypted) < saltLength+5 {
		return nil, errors.New("encrypted payload too short")
	}

	salt := encrypted[:saltLength]
	idLen := int(encrypted[saltLength+4])
	headerLen := saltLength + 5 + idLen
	if idLen != p256PublicKeyLen || len(encrypted) <= headerLen {
		return nil, errors.New("invalid aes128gcm header")
	}

	asPubBytes := encrypted[saltLength+5 : headerLen]
	asPubKey, err := ecdh.P256().NewPublicKey(asPubBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid sender public key in header: %w", err)
	}

	authSecret, err := DecodeURLSafeBase64(authSecretB64)
	if err != nil || len(authSecret) != authSecretLength {
		return nil, errors.New("invalid auth secret")
	}

	sharedSecret, err := uaPriv.ECDH(asPubKey)
	if err != nil {
		return nil, fmt.Errorf("ECDH agreement failed: %w", err)
	}

	uaPubBytes := uaPriv.PublicKey().Bytes()

	prkKey, err := hkdf.Extract(sha256.New, sharedSecret, authSecret)
	if err != nil {
		return nil, err
	}

	keyInfo := make([]byte, 0, 14+len(uaPubBytes)+len(asPubBytes))
	keyInfo = append(keyInfo, []byte("WebPush: info\x00")...)
	keyInfo = append(keyInfo, uaPubBytes...)
	keyInfo = append(keyInfo, asPubBytes...)

	ikm, err := hkdf.Expand(sha256.New, prkKey, string(keyInfo), 32)
	if err != nil {
		return nil, err
	}

	prk, err := hkdf.Extract(sha256.New, ikm, salt)
	if err != nil {
		return nil, err
	}

	cek, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}

	nonce, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	padded, err := gcm.Open(nil, nonce, encrypted[headerLen:], nil)
	if err != nil {
		return nil, fmt.Errorf("gcm decryption failed: %w", err)
	}

	// Strip trailing RFC 8188 padding delimiter 0x02 and any zero padding after it
	end := len(padded) - 1
	for end >= 0 && padded[end] == 0x00 {
		end--
	}
	if end < 0 || padded[end] != 0x02 {
		return nil, errors.New("invalid RFC 8188 padding delimiter")
	}
	return padded[:end], nil
}

