package push

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestHTTPClient(handler http.HandlerFunc) *http.Client {
	return &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			handler(rec, r)
			return rec.Result(), nil
		}),
	}
}

func generateTestSubscriberKeys(t *testing.T) (*ecdh.PrivateKey, string, string) {
	t.Helper()
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed generating test subscriber key: %v", err)
	}
	authSecret := make([]byte, 16)
	if _, err := rand.Read(authSecret); err != nil {
		t.Fatalf("failed generating auth secret: %v", err)
	}
	return priv,
		base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()),
		base64.RawURLEncoding.EncodeToString(authSecret)
}

func TestVAPID_KeyGenAndJWTVerification(t *testing.T) {
	pubB64, privB64, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys failed: %v", err)
	}

	if err := ValidateVAPIDPublicKey(pubB64); err != nil {
		t.Fatalf("ValidateVAPIDPublicKey failed: %v", err)
	}

	privKey, derivedPub, err := ParseVAPIDPrivateKey(privB64)
	if err != nil {
		t.Fatalf("ParseVAPIDPrivateKey failed: %v", err)
	}
	if privKey == nil {
		t.Fatal("expected parsed ECDSA private key")
	}
	if derivedPub != pubB64 {
		t.Fatalf("derived public key mismatch: got %s want %s", derivedPub, pubB64)
	}

	now := time.Now().UTC()
	endpoint := "https://fcm.googleapis.com/fcm/send/test-token-123"
	header, err := BuildVAPIDAuthHeader(endpoint, "mailto:alerts@gocart.com", pubB64, privB64, 12*time.Hour, now)
	if err != nil {
		t.Fatalf("BuildVAPIDAuthHeader failed: %v", err)
	}

	if !strings.HasPrefix(header, "vapid t=") {
		t.Fatalf("unexpected Authorization header format: %s", header)
	}

	parts := strings.Split(strings.TrimPrefix(header, "vapid "), ", ")
	if len(parts) != 2 {
		t.Fatalf("expected t= and k= parts, got %v", parts)
	}
	tokenStr := strings.TrimPrefix(parts[0], "t=")
	keyStr := strings.TrimPrefix(parts[1], "k=")
	if keyStr != pubB64 {
		t.Errorf("expected k=%s, got %s", pubB64, keyStr)
	}

	parsedToken, err := jwt.Parse(tokenStr, func(tok *jwt.Token) (any, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, errors.New("unexpected signing method")
		}
		pubRaw, _ := DecodeURLSafeBase64(keyStr)
		return &ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     new(big.Int).SetBytes(pubRaw[1:33]),
			Y:     new(big.Int).SetBytes(pubRaw[33:65]),
		}, nil
	}, jwt.WithAudience("https://fcm.googleapis.com"))
	if err != nil {
		t.Fatalf("failed verifying signed VAPID JWT: %v", err)
	}
	if !parsedToken.Valid {
		t.Fatalf("expected valid VAPID JWT")
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatalf("expected MapClaims")
	}
	if claims["sub"] != "mailto:alerts@gocart.com" {
		t.Errorf("unexpected sub claim: %v", claims["sub"])
	}
}

func TestEncryptAndDecrypt_RFC8291Roundtrip(t *testing.T) {
	uaPriv, uaPubB64, authB64 := generateTestSubscriberKeys(t)

	if err := ValidateSubscriptionKeys(uaPubB64, authB64); err != nil {
		t.Fatalf("ValidateSubscriptionKeys failed: %v", err)
	}

	plaintext := []byte(`{"title":"Order Confirmed","body":"Your order #ORD-42 is confirmed!"}`)
	encrypted, err := EncryptPayload(plaintext, uaPubB64, authB64)
	if err != nil {
		t.Fatalf("EncryptPayload failed: %v", err)
	}

	if bytes.Contains(encrypted, []byte("ORD-42")) {
		t.Fatalf("encrypted payload must not contain plaintext")
	}

	decrypted, err := DecryptPayload(encrypted, uaPriv, authB64)
	if err != nil {
		t.Fatalf("DecryptPayload failed: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("roundtrip mismatch: got %q want %q", decrypted, plaintext)
	}
}

// TestEncrypt_RFC8291Vector verifies exact compliance against RFC 8291 Appendix A test vector.
func TestEncrypt_RFC8291Vector(t *testing.T) {
	uaPrivRaw, err := DecodeURLSafeBase64("q1dXpw3UpT5VOmu_cf_v6ih07Aems3njxI-JWgLcM94")
	if err != nil {
		t.Fatalf("decode uaPriv failed: %v", err)
	}
	uaPriv, err := ecdh.P256().NewPrivateKey(uaPrivRaw)
	if err != nil {
		t.Fatalf("parse uaPriv failed: %v", err)
	}
	uaPubB64 := base64.RawURLEncoding.EncodeToString(uaPriv.PublicKey().Bytes())
	if !strings.HasPrefix(uaPubB64, "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcx") {
		t.Fatalf("unexpected uaPub derived: %s", uaPubB64)
	}

	authB64 := "BTBZMqHH6r4Tts7J_aSIgg"
	asPrivB64 := "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"
	saltB64 := "DGv6ra1nlYgDCS1FRnbzlw"
	plaintext := []byte("When I grow up, I want to be a watermelon")
	expectedCipherB64 := "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"

	asPrivRaw, err := DecodeURLSafeBase64(asPrivB64)
	if err != nil {
		t.Fatalf("decode asPriv failed: %v", err)
	}
	asPriv, err := ecdh.P256().NewPrivateKey(asPrivRaw)
	if err != nil {
		t.Fatalf("parse asPriv failed: %v", err)
	}
	salt, err := DecodeURLSafeBase64(saltB64)
	if err != nil {
		t.Fatalf("decode salt failed: %v", err)
	}

	got, err := encryptPayloadWithParams(plaintext, uaPubB64, authB64, salt, asPriv, 4096)
	if err != nil {
		t.Fatalf("encryptPayloadWithParams failed: %v", err)
	}

	expected, err := DecodeURLSafeBase64(expectedCipherB64)
	if err != nil {
		t.Fatalf("decode expected failed: %v", err)
	}

	if !bytes.Equal(got, expected) {
		t.Fatalf("RFC 8291 Appendix A vector mismatch:\ngot:  %s\nwant: %s",
			base64.RawURLEncoding.EncodeToString(got),
			expectedCipherB64,
		)
	}

	decrypted, decErr := DecryptPayload(got, uaPriv, authB64)
	if decErr != nil {
		t.Fatalf("DecryptPayload failed on RFC 8291 vector: %v", decErr)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("decrypted mismatch: got %q want %q", decrypted, plaintext)
	}
}

func TestWebPushClient_SendSuccessGoneAndRetry(t *testing.T) {
	pubB64, privB64, err := GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys failed: %v", err)
	}

	uaPriv, uaPubB64, authB64 := generateTestSubscriberKeys(t)

	t.Run("successful push delivery encrypts payload and signs VAPID", func(t *testing.T) {
		var receivedAuth, receivedEnc, receivedTTL string
		var receivedBody []byte

		httpClient := newTestHTTPClient(func(w http.ResponseWriter, r *http.Request) {
			receivedAuth = r.Header.Get("Authorization")
			receivedEnc = r.Header.Get("Content-Encoding")
			receivedTTL = r.Header.Get("TTL")
			receivedBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusCreated)
		})

		var logBuf bytes.Buffer
		log := slog.New(slog.NewJSONHandler(&logBuf, nil))

		client := NewWebPushClient(
			config.VAPIDConfig{
				PublicKey:  pubB64,
				PrivateKey: privB64,
				Subject:    "mailto:push@gocart.local",
			},
			config.PushConfig{
				RequestTimeout: 2 * time.Second,
				MaxRetries:     2,
				RetryInterval:  10 * time.Millisecond,
				TTLSeconds:     3600,
			},
			httpClient,
			log,
		)

		sub := &model.PushSubscription{
			ID:       "sub-1",
			UserID:   "user-1",
			Endpoint: "https://push.example.com/push/sub-1",
			P256DH:   uaPubB64,
			Auth:     authB64,
			IsActive: true,
		}
		payload := []byte(`{"title":"Shipped","body":"Order shipped"}`)

		res, err := client.Send(context.Background(), sub, payload)
		if err != nil {
			t.Fatalf("expected push send success, got error: %v", err)
		}
		if res.StatusCode != http.StatusCreated || res.Attempts != 1 || res.SubscriptionGone {
			t.Fatalf("unexpected send result: %+v", res)
		}
		if !strings.HasPrefix(receivedAuth, "vapid t=") {
			t.Errorf("expected vapid auth header, got %q", receivedAuth)
		}
		if receivedEnc != "aes128gcm" {
			t.Errorf("expected aes128gcm header, got %q", receivedEnc)
		}
		if receivedTTL != "3600" {
			t.Errorf("expected TTL 3600, got %q", receivedTTL)
		}

		decrypted, decErr := DecryptPayload(receivedBody, uaPriv, authB64)
		if decErr != nil {
			t.Fatalf("server received undecryptable payload: %v", decErr)
		}
		if !bytes.Equal(decrypted, payload) {
			t.Errorf("decrypted body mismatch: got %q want %q", decrypted, payload)
		}

		// Ensure VAPID private key never leaks in logs
		if strings.Contains(logBuf.String(), privB64) {
			t.Fatalf("security violation: VAPID private key leaked in logs")
		}
	})

	t.Run("expired subscription HTTP 410 and 404 detected immediately without retry", func(t *testing.T) {
		for _, status := range []int{http.StatusNotFound, http.StatusGone} {
			var calls atomic.Int32
			httpClient := newTestHTTPClient(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(status)
				_, _ = w.Write([]byte("subscription expired"))
			})

			client := NewWebPushClient(
				config.VAPIDConfig{PublicKey: pubB64, PrivateKey: privB64, Subject: "mailto:push@gocart.local"},
				config.PushConfig{RequestTimeout: time.Second, MaxRetries: 3, RetryInterval: 5 * time.Millisecond},
				httpClient,
				slog.New(slog.NewTextHandler(io.Discard, nil)),
			)

			sub := &model.PushSubscription{ID: "sub-gone", Endpoint: "https://push.example.com/gone", P256DH: uaPubB64, Auth: authB64}
			res, sendErr := client.Send(context.Background(), sub, []byte("test"))

			if !errors.Is(sendErr, ErrSubscriptionGone) {
				t.Fatalf("expected ErrSubscriptionGone for status %d, got: %v", status, sendErr)
			}
			if res == nil || !res.SubscriptionGone || res.StatusCode != status {
				t.Fatalf("expected SubscriptionGone result for %d, got: %+v", status, res)
			}
			if calls.Load() != 1 {
				t.Errorf("expected 1 attempt (no retry on %d), got %d", status, calls.Load())
			}
		}
	})

	t.Run("transient 503 retries and succeeds", func(t *testing.T) {
		var calls atomic.Int32
		httpClient := newTestHTTPClient(func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusCreated)
		})

		client := NewWebPushClient(
			config.VAPIDConfig{PublicKey: pubB64, PrivateKey: privB64, Subject: "mailto:push@gocart.local"},
			config.PushConfig{RequestTimeout: time.Second, MaxRetries: 3, RetryInterval: 5 * time.Millisecond},
			httpClient,
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		)

		sub := &model.PushSubscription{ID: "sub-retry", Endpoint: "https://push.example.com/retry", P256DH: uaPubB64, Auth: authB64}
		res, sendErr := client.Send(context.Background(), sub, []byte("hello"))
		if sendErr != nil {
			t.Fatalf("expected retry recovery, got error: %v", sendErr)
		}
		if res.Attempts != 3 || res.StatusCode != http.StatusCreated {
			t.Fatalf("expected 3 attempts and 201 status, got %+v", res)
		}
	})
}
