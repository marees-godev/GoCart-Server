package tests_test

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	notificationpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/notification"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/consumer"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/push"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type demoRoundTripFunc func(*http.Request) (*http.Response, error)

func (f demoRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type demoBrowserKeys struct {
	name     string
	endpoint string
	privKey  *ecdh.PrivateKey
	p256dh   string
	auth     string
}

type demoDeliveredMessage struct {
	DeviceName string
	Endpoint   string
	StatusCode int
	Payload    dto.PushPayload
}

type demoPushGateway struct {
	vapidPubKey string
	vapidSubj   string
	mu          sync.Mutex
	browsers    map[string]demoBrowserKeys
	failOnce    map[string]*atomic.Int32
	goneURLs    map[string]bool
	delivered   []demoDeliveredMessage
}

func newDemoPushGateway(vapidPubKey, vapidSubj string) *demoPushGateway {
	return &demoPushGateway{
		vapidPubKey: vapidPubKey,
		vapidSubj:   vapidSubj,
		browsers:    make(map[string]demoBrowserKeys),
		failOnce:    make(map[string]*atomic.Int32),
		goneURLs:    make(map[string]bool),
	}
}

func (g *demoPushGateway) registerBrowser(b demoBrowserKeys) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.browsers[b.endpoint] = b
}

func (g *demoPushGateway) setTransient429Once(endpoint string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.failOnce[endpoint] = &atomic.Int32{}
}

func (g *demoPushGateway) setGone(endpoint string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.goneURLs[endpoint] = true
}

func (g *demoPushGateway) snapshotDelivered() []demoDeliveredMessage {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]demoDeliveredMessage, len(g.delivered))
	copy(out, g.delivered)
	return out
}

func (g *demoPushGateway) httpClient(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{
		Transport: demoRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			rec := httptest.NewRecorder()
			endpoint := r.URL.String()

			if r.Method != http.MethodPost {
				rec.WriteHeader(http.StatusMethodNotAllowed)
				return rec.Result(), nil
			}
			if enc := r.Header.Get("Content-Encoding"); enc != "aes128gcm" {
				t.Errorf("expected Content-Encoding=aes128gcm, got %q", enc)
				rec.WriteHeader(http.StatusBadRequest)
				return rec.Result(), nil
			}
			if ct := r.Header.Get("Content-Type"); ct != "application/octet-stream" {
				t.Errorf("expected Content-Type=application/octet-stream, got %q", ct)
			}
			if r.Header.Get("TTL") == "" || r.Header.Get("Urgency") != "normal" {
				t.Errorf("missing expected push headers TTL=%q Urgency=%q", r.Header.Get("TTL"), r.Header.Get("Urgency"))
			}

			if err := verifyDemoVAPIDHeader(r.Header.Get("Authorization"), endpoint, g.vapidSubj, g.vapidPubKey); err != nil {
				t.Errorf("VAPID verification failed for %s: %v", endpoint, err)
				rec.WriteHeader(http.StatusUnauthorized)
				return rec.Result(), nil
			}

			rawBody, _ := io.ReadAll(r.Body)

			g.mu.Lock()
			gone := g.goneURLs[endpoint]
			retryCounter := g.failOnce[endpoint]
			browser, hasBrowser := g.browsers[endpoint]
			g.mu.Unlock()

			if gone {
				rec.WriteHeader(http.StatusGone)
				_, _ = rec.Write([]byte("subscription expired"))
				return rec.Result(), nil
			}
			if retryCounter != nil && retryCounter.Add(1) == 1 {
				rec.WriteHeader(http.StatusTooManyRequests)
				_, _ = rec.Write([]byte("rate limited, retry with backoff"))
				return rec.Result(), nil
			}
			if !hasBrowser {
				rec.WriteHeader(http.StatusNotFound)
				return rec.Result(), nil
			}

			plain, decErr := push.DecryptPayload(rawBody, browser.privKey, browser.auth)
			if decErr != nil {
				t.Errorf("failed decrypting push wire payload for %s: %v", browser.name, decErr)
				rec.WriteHeader(http.StatusBadRequest)
				return rec.Result(), nil
			}

			var decoded dto.PushPayload
			if err := json.Unmarshal(plain, &decoded); err != nil {
				t.Errorf("failed unmarshaling decrypted push payload for %s: %v", browser.name, err)
				rec.WriteHeader(http.StatusBadRequest)
				return rec.Result(), nil
			}
			if decoded.Title != "" && bytes.Contains(rawBody, []byte(decoded.Title)) {
				t.Errorf("wire body leaked plaintext title %q", decoded.Title)
			}

			g.mu.Lock()
			g.delivered = append(g.delivered, demoDeliveredMessage{
				DeviceName: browser.name,
				Endpoint:   endpoint,
				StatusCode: http.StatusCreated,
				Payload:    decoded,
			})
			g.mu.Unlock()

			rec.WriteHeader(http.StatusCreated)
			return rec.Result(), nil
		}),
	}
}

func verifyDemoVAPIDHeader(authHeader, endpoint, expectedSub, expectedPubKey string) error {
	if !strings.HasPrefix(authHeader, "vapid ") {
		return fmt.Errorf("missing vapid prefix in %q", authHeader)
	}
	parts := strings.Split(strings.TrimPrefix(authHeader, "vapid "), ", ")
	if len(parts) != 2 {
		return fmt.Errorf("unexpected vapid header format: %q", authHeader)
	}
	jwtStr := strings.TrimPrefix(parts[0], "t=")
	pubStr := strings.TrimPrefix(parts[1], "k=")
	if pubStr != expectedPubKey {
		return fmt.Errorf("vapid public key mismatch: got %s want %s", pubStr, expectedPubKey)
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	expectedAud := u.Scheme + "://" + u.Host

	tok, err := jwt.Parse(jwtStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		rawPub, decErr := push.DecodeURLSafeBase64(pubStr)
		if decErr != nil {
			return nil, decErr
		}
		return &ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     new(big.Int).SetBytes(rawPub[1:33]),
			Y:     new(big.Int).SetBytes(rawPub[33:65]),
		}, nil
	}, jwt.WithAudience(expectedAud))
	if err != nil {
		return err
	}
	if !tok.Valid {
		return errors.New("invalid VAPID JWT")
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok || claims["sub"] != expectedSub {
		return fmt.Errorf("unexpected VAPID sub claim: %v", claims["sub"])
	}
	return nil
}

func createDemoBrowser(t *testing.T, name, endpoint string) demoBrowserKeys {
	t.Helper()
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed generating browser P-256 key: %v", err)
	}
	authSecret := make([]byte, 16)
	if _, err := rand.Read(authSecret); err != nil {
		t.Fatalf("failed generating auth secret: %v", err)
	}
	return demoBrowserKeys{
		name:     name,
		endpoint: endpoint,
		privKey:  priv,
		p256dh:   base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()),
		auth:     base64.RawURLEncoding.EncodeToString(authSecret),
	}
}

type demoOrderLookup struct {
	orderUsers map[string]string
	calls      atomic.Int32
}

func (l *demoOrderLookup) LookupOrderUserID(ctx context.Context, orderID string) (string, error) {
	l.calls.Add(1)
	return l.orderUsers[orderID], nil
}

type demoMemRepo struct {
	mu              sync.RWMutex
	seq             int
	channels        map[string]*model.Channel
	prefs           map[string]*model.Preference
	jobs            map[string]*model.NotificationJob
	jobsByEventID   map[string]*model.NotificationJob
	processedEvents map[string]*model.ProcessedEvent
	notifs          map[string]*model.Notification
	byEventChan     map[string]string
	subsByID        map[string]*model.PushSubscription
	subsByEndpoint  map[string]string
}

func newDemoMemRepo() *demoMemRepo {
	r := &demoMemRepo{
		channels:        make(map[string]*model.Channel),
		prefs:           make(map[string]*model.Preference),
		jobs:            make(map[string]*model.NotificationJob),
		jobsByEventID:   make(map[string]*model.NotificationJob),
		processedEvents: make(map[string]*model.ProcessedEvent),
		notifs:          make(map[string]*model.Notification),
		byEventChan:     make(map[string]string),
		subsByID:        make(map[string]*model.PushSubscription),
		subsByEndpoint:  make(map[string]string),
	}
	for _, c := range []string{model.ChannelEmail, model.ChannelPush, model.ChannelSMS, model.ChannelInApp} {
		ch := &model.Channel{ID: uuid.NewString(), Code: c, Name: c, IsActive: true, CreatedAt: time.Now().UTC()}
		r.channels[c] = ch
		r.channels[ch.ID] = ch
	}
	return r
}

func demoCloneNotif(n *model.Notification) *model.Notification {
	if n == nil {
		return nil
	}
	cp := *n
	if n.EventID != nil {
		v := *n.EventID
		cp.EventID = &v
	}
	if n.ErrorMessage != nil {
		v := *n.ErrorMessage
		cp.ErrorMessage = &v
	}
	if n.ReadAt != nil {
		v := *n.ReadAt
		cp.ReadAt = &v
	}
	if n.SentAt != nil {
		v := *n.SentAt
		cp.SentAt = &v
	}
	return &cp
}

func demoCloneSub(s *model.PushSubscription) *model.PushSubscription {
	if s == nil {
		return nil
	}
	cp := *s
	if s.LastError != nil {
		v := *s.LastError
		cp.LastError = &v
	}
	if s.LastUsedAt != nil {
		v := *s.LastUsedAt
		cp.LastUsedAt = &v
	}
	if s.DeactivatedAt != nil {
		v := *s.DeactivatedAt
		cp.DeactivatedAt = &v
	}
	return &cp
}

func (r *demoMemRepo) GetByCode(ctx context.Context, code string) (*model.Channel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.channels[code], nil
}

func (r *demoMemRepo) GetByID(ctx context.Context, id string) (*model.Channel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.channels[id], nil
}

func (r *demoMemRepo) GetByEventType(ctx context.Context, eventType string) (*model.Template, error) {
	return nil, nil
}

func (r *demoMemRepo) GetByEventTypeAndChannel(ctx context.Context, eventType, channelID string) (*model.Template, error) {
	return nil, nil
}

func (r *demoMemRepo) GetPreference(ctx context.Context, userID, channelID, eventType string) (*model.Preference, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.prefs[userID+":"+channelID+":"+eventType], nil
}

func (r *demoMemRepo) UpsertPreference(ctx context.Context, pref *model.Preference) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prefs[pref.UserID+":"+pref.ChannelID+":"+pref.EventType] = pref
	return nil
}

func (r *demoMemRepo) CreateJob(ctx context.Context, job *model.NotificationJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.jobsByEventID[job.EventID]; ok {
		return appErrors.Conflict("job already exists")
	}
	if job.ID == "" {
		job.ID = uuid.NewString()
	}
	cp := *job
	r.jobs[cp.ID] = &cp
	r.jobsByEventID[cp.EventID] = &cp
	return nil
}

func (r *demoMemRepo) GetJobByEventID(ctx context.Context, eventID string) (*model.NotificationJob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if j, ok := r.jobsByEventID[eventID]; ok {
		cp := *j
		return &cp, nil
	}
	return nil, nil
}

func (r *demoMemRepo) GetJobByID(ctx context.Context, id string) (*model.NotificationJob, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if j, ok := r.jobs[id]; ok {
		cp := *j
		return &cp, nil
	}
	return nil, nil
}

func (r *demoMemRepo) UpdateJobStatus(ctx context.Context, id string, status model.JobStatus, errMsg *string) error {
	return nil
}

func (r *demoMemRepo) IncrementRetry(ctx context.Context, id string, errMsg string) error {
	return nil
}

func (r *demoMemRepo) ListPendingJobs(ctx context.Context, limit int) ([]*model.NotificationJob, error) {
	return nil, nil
}

func (r *demoMemRepo) ClaimPendingJobs(ctx context.Context, limit int) ([]*model.NotificationJob, error) {
	return nil, nil
}

func (r *demoMemRepo) IsEventProcessed(ctx context.Context, eventID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.processedEvents[eventID]
	return ok, nil
}

func (r *demoMemRepo) RecordProcessedEvent(ctx context.Context, event *model.ProcessedEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.processedEvents[event.EventID] = event
	return nil
}

func (r *demoMemRepo) CreateNotification(ctx context.Context, n *model.Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n.ID == "" {
		n.ID = uuid.NewString()
	}
	if n.EventID != nil && *n.EventID != "" {
		k := *n.EventID + ":" + n.ChannelID
		if _, exists := r.byEventChan[k]; exists {
			return appErrors.Conflict("notification already exists")
		}
		r.byEventChan[k] = n.ID
	}
	r.seq++
	now := time.Now().UTC().Add(time.Duration(r.seq) * time.Millisecond)
	n.CreatedAt = now
	n.UpdatedAt = now
	r.notifs[n.ID] = demoCloneNotif(n)
	return nil
}

func (r *demoMemRepo) GetNotificationByID(ctx context.Context, id string) (*model.Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return demoCloneNotif(r.notifs[id]), nil
}

func (r *demoMemRepo) GetNotificationByEventIDAndChannel(ctx context.Context, eventID, channelID string) (*model.Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if id, ok := r.byEventChan[eventID+":"+channelID]; ok {
		return demoCloneNotif(r.notifs[id]), nil
	}
	return nil, nil
}

func (r *demoMemRepo) ListNotifications(ctx context.Context, filter model.NotificationFilter) ([]*model.Notification, int, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matching []*model.Notification
	unread := 0
	for _, n := range r.notifs {
		if filter.UserID != "" && n.UserID != filter.UserID {
			continue
		}
		if !n.IsRead {
			unread++
		}
		if filter.Status != "" && string(n.Status) != filter.Status {
			continue
		}
		if filter.Channel != "" && n.ChannelCode != filter.Channel {
			continue
		}
		if filter.EventType != "" && n.EventType != filter.EventType {
			continue
		}
		if filter.IsRead != nil && n.IsRead != *filter.IsRead {
			continue
		}
		matching = append(matching, demoCloneNotif(n))
	}

	sort.Slice(matching, func(i, j int) bool {
		return matching[i].CreatedAt.After(matching[j].CreatedAt)
	})

	total := len(matching)
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []*model.Notification{}, total, unread, nil
	}
	end := offset + filter.Limit
	if filter.Limit <= 0 || end > total {
		end = total
	}
	return matching[offset:end], total, unread, nil
}

func (r *demoMemRepo) UpdateDeliveryStatus(ctx context.Context, id string, status model.DeliveryStatus, errMsg *string, incrementRetry bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n, ok := r.notifs[id]; ok {
		n.Status = status
		if errMsg != nil {
			v := *errMsg
			n.ErrorMessage = &v
		} else {
			n.ErrorMessage = nil
		}
		if incrementRetry {
			n.RetryCount++
		}
		now := time.Now().UTC()
		if status == model.DeliveryStatusSent && n.SentAt == nil {
			n.SentAt = &now
		}
		n.UpdatedAt = now
	}
	return nil
}

func (r *demoMemRepo) MarkAsRead(ctx context.Context, id, userID string, isRead bool) (*model.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.notifs[id]
	if !ok || (userID != "" && n.UserID != userID) {
		return nil, nil
	}
	now := time.Now().UTC()
	n.IsRead = isRead
	if isRead {
		if n.ReadAt == nil {
			n.ReadAt = &now
		}
	} else {
		n.ReadAt = nil
	}
	n.UpdatedAt = now
	return demoCloneNotif(n), nil
}

func (r *demoMemRepo) MarkAllAsRead(ctx context.Context, userID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	updated := 0
	for _, n := range r.notifs {
		if n.UserID == userID && !n.IsRead {
			n.IsRead = true
			n.ReadAt = &now
			n.UpdatedAt = now
			updated++
		}
	}
	return updated, nil
}

func (r *demoMemRepo) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cnt := 0
	for _, n := range r.notifs {
		if n.UserID == userID && !n.IsRead {
			cnt++
		}
	}
	return cnt, nil
}

func (r *demoMemRepo) UpsertSubscription(ctx context.Context, sub *model.PushSubscription) (*model.PushSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	if existingID, ok := r.subsByEndpoint[sub.Endpoint]; ok {
		ex := r.subsByID[existingID]
		ex.UserID = sub.UserID
		ex.P256DH = sub.P256DH
		ex.Auth = sub.Auth
		if sub.UserAgent != "" {
			ex.UserAgent = sub.UserAgent
		}
		if sub.DeviceName != "" {
			ex.DeviceName = sub.DeviceName
		}
		ex.IsActive = true
		ex.FailureCount = 0
		ex.LastError = nil
		ex.DeactivatedAt = nil
		ex.UpdatedAt = now
		return demoCloneSub(ex), nil
	}
	cp := demoCloneSub(sub)
	if cp.ID == "" {
		cp.ID = uuid.NewString()
	}
	cp.IsActive = true
	cp.CreatedAt = now
	cp.UpdatedAt = now
	r.subsByID[cp.ID] = cp
	r.subsByEndpoint[cp.Endpoint] = cp.ID
	return demoCloneSub(cp), nil
}

func (r *demoMemRepo) GetSubscriptionByID(ctx context.Context, id string) (*model.PushSubscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return demoCloneSub(r.subsByID[id]), nil
}

func (r *demoMemRepo) GetSubscriptionByEndpoint(ctx context.Context, endpoint string) (*model.PushSubscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if id, ok := r.subsByEndpoint[endpoint]; ok {
		return demoCloneSub(r.subsByID[id]), nil
	}
	return nil, nil
}

func (r *demoMemRepo) ListSubscriptionsByUserID(ctx context.Context, userID string, activeOnly bool) ([]*model.PushSubscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var list []*model.PushSubscription
	for _, s := range r.subsByID {
		if s.UserID == userID && (!activeOnly || s.IsActive) {
			list = append(list, demoCloneSub(s))
		}
	}
	return list, nil
}

func (r *demoMemRepo) UpdateSubscription(ctx context.Context, sub *model.PushSubscription) (*model.PushSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ex := r.subsByID[sub.ID]
	if ex == nil || (sub.UserID != "" && ex.UserID != sub.UserID) {
		return nil, nil
	}
	if sub.Endpoint != "" && sub.Endpoint != ex.Endpoint {
		if otherID, taken := r.subsByEndpoint[sub.Endpoint]; taken && otherID != ex.ID {
			return nil, errors.New("23505 endpoint already exists")
		}
		delete(r.subsByEndpoint, ex.Endpoint)
		ex.Endpoint = sub.Endpoint
		r.subsByEndpoint[ex.Endpoint] = ex.ID
	}
	if sub.P256DH != "" {
		ex.P256DH = sub.P256DH
	}
	if sub.Auth != "" {
		ex.Auth = sub.Auth
	}
	if sub.UserAgent != "" {
		ex.UserAgent = sub.UserAgent
	}
	if sub.DeviceName != "" {
		ex.DeviceName = sub.DeviceName
	}
	now := time.Now().UTC()
	ex.IsActive = sub.IsActive
	if ex.IsActive {
		ex.DeactivatedAt = nil
		ex.FailureCount = 0
		ex.LastError = nil
	} else if ex.DeactivatedAt == nil {
		ex.DeactivatedAt = &now
	}
	ex.UpdatedAt = now
	return demoCloneSub(ex), nil
}

func (r *demoMemRepo) DeactivateSubscription(ctx context.Context, id, userID, endpoint string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var s *model.PushSubscription
	if id != "" {
		s = r.subsByID[id]
	} else if endpoint != "" {
		if subID, ok := r.subsByEndpoint[endpoint]; ok {
			s = r.subsByID[subID]
		}
	}
	if s == nil || (userID != "" && s.UserID != userID) {
		return false, nil
	}
	now := time.Now().UTC()
	s.IsActive = false
	if s.DeactivatedAt == nil {
		s.DeactivatedAt = &now
	}
	s.UpdatedAt = now
	return true, nil
}

func (r *demoMemRepo) MarkSubscriptionGone(ctx context.Context, id string, errMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.subsByID[id]; ok {
		now := time.Now().UTC()
		s.IsActive = false
		s.FailureCount++
		s.LastError = &errMsg
		if s.DeactivatedAt == nil {
			s.DeactivatedAt = &now
		}
		s.UpdatedAt = now
	}
	return nil
}

func (r *demoMemRepo) RecordSubscriptionFailure(ctx context.Context, id string, errMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.subsByID[id]; ok {
		s.FailureCount++
		s.LastError = &errMsg
		s.UpdatedAt = time.Now().UTC()
	}
	return nil
}

func (r *demoMemRepo) MarkSubscriptionUsed(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.subsByID[id]; ok {
		now := time.Now().UTC()
		s.FailureCount = 0
		s.LastError = nil
		s.LastUsedAt = &now
		s.UpdatedAt = now
	}
	return nil
}

func TestDemo_PushNotificationEndToEnd(t *testing.T) {
	ctx := context.Background()

	// Step 1: Boot notification-service core with VAPID keys, gRPC server (bufconn), and Kafka consumer
	pubVAPID, privVAPID, err := push.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys failed: %v", err)
	}
	vapidSubject := "mailto:alerts@gocart.local"
	t.Logf("[DEMO 1/6] Generated VAPID P-256 keypair (public=%s...)", pubVAPID[:16])

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	pushGateway := newDemoPushGateway(pubVAPID, vapidSubject)
	webPushClient := push.NewWebPushClient(
		config.VAPIDConfig{PublicKey: pubVAPID, PrivateKey: privVAPID, Subject: vapidSubject},
		config.PushConfig{RequestTimeout: 2 * time.Second, MaxRetries: 3, RetryInterval: 5 * time.Millisecond, TTLSeconds: 3600},
		pushGateway.httpClient(t),
		logger,
	)

	customerID := uuid.NewString()
	merchantID := uuid.NewString()
	noSubUserID := uuid.NewString()

	repo := newDemoMemRepo()
	tmplSvc := service.NewTemplateService()
	prefSvc := service.NewPreferenceService(repo, repo)
	subSvc := service.NewPushSubscriptionService(repo, config.VAPIDConfig{
		PublicKey:  pubVAPID,
		PrivateKey: privVAPID,
		Subject:    vapidSubject,
	}, logger)
	notifSvc := service.NewNotificationService(repo, repo, repo, repo, tmplSvc, webPushClient, logger)

	orderLookup := &demoOrderLookup{orderUsers: map[string]string{"ORD-DEMO-1": customerID}}
	pipeline := service.NewPushNotificationPipelineService(tmplSvc, prefSvc, repo, repo, notifSvc, orderLookup, logger)
	evtConsumer := consumer.NewNotificationEventConsumer(pipeline, logger)

	var dlqEvents []*events.EventEnvelope
	evtConsumer.SetDLQPublisher(func(_ context.Context, env *events.EventEnvelope, reason error) error {
		dlqEvents = append(dlqEvents, env)
		return nil
	})

	lis := bufconn.Listen(1024 * 1024)
	grpcSrv := grpc.NewServer(grpc.ChainUnaryInterceptor(grpcclient.UnaryServerInterceptor()))
	notificationpb.RegisterNotificationServiceServer(grpcSrv, handler.NewNotificationGRPCHandler(notifSvc, subSvc))
	go func() { _ = grpcSrv.Serve(lis) }()
	defer grpcSrv.GracefulStop()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient failed: %v", err)
	}
	defer conn.Close()
	grpcClient := notificationpb.NewNotificationServiceClient(conn)

	customerCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs("x-user-id", customerID, "x-user-role", "CUSTOMER"))
	merchantCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs("x-user-id", merchantID, "x-user-role", "MERCHANT"))

	// Step 2: Fetch VAPID public key and register multi-device browser subscriptions via gRPC
	vapidResp, err := grpcClient.GetVAPIDPublicKey(customerCtx, &notificationpb.GetVAPIDPublicKeyRequest{})
	if err != nil || vapidResp.GetPublicKey() != pubVAPID {
		t.Fatalf("GetVAPIDPublicKey failed: %v", err)
	}

	chrome := createDemoBrowser(t, "Desktop Chrome", "https://fcm.googleapis.com/fcm/send/demo-chrome")
	firefox := createDemoBrowser(t, "Mobile Firefox", "https://updates.push.services.mozilla.com/wpush/v2/demo-firefox")
	merchantBrowser := createDemoBrowser(t, "Merchant Chrome", "https://fcm.googleapis.com/fcm/send/demo-merchant")

	pushGateway.registerBrowser(chrome)
	pushGateway.registerBrowser(firefox)
	pushGateway.registerBrowser(merchantBrowser)
	pushGateway.setTransient429Once(firefox.endpoint)

	regChrome, err := grpcClient.RegisterPushSubscription(customerCtx, &notificationpb.RegisterPushSubscriptionRequest{
		UserId:     customerID,
		Endpoint:   chrome.endpoint,
		P256Dh:     chrome.p256dh,
		Auth:       chrome.auth,
		UserAgent:  "Chrome/134",
		DeviceName: "My Chrome",
	})
	if err != nil {
		t.Fatalf("RegisterPushSubscription (Chrome) failed: %v", err)
	}
	if _, err := grpcClient.RegisterPushSubscription(customerCtx, &notificationpb.RegisterPushSubscriptionRequest{
		UserId:     customerID,
		Endpoint:   firefox.endpoint,
		P256Dh:     firefox.p256dh,
		Auth:       firefox.auth,
		UserAgent:  "Firefox/135 Mobile",
		DeviceName: "Pixel Firefox",
	}); err != nil {
		t.Fatalf("RegisterPushSubscription (Firefox) failed: %v", err)
	}
	if _, err := grpcClient.RegisterPushSubscription(merchantCtx, &notificationpb.RegisterPushSubscriptionRequest{
		UserId:     merchantID,
		Endpoint:   merchantBrowser.endpoint,
		P256Dh:     merchantBrowser.p256dh,
		Auth:       merchantBrowser.auth,
		UserAgent:  "Chrome/134",
		DeviceName: "Merchant Portal",
	}); err != nil {
		t.Fatalf("RegisterPushSubscription (Merchant) failed: %v", err)
	}

	updChrome, err := grpcClient.UpdatePushSubscription(customerCtx, &notificationpb.UpdatePushSubscriptionRequest{
		Id:         regChrome.GetSubscription().GetId(),
		UserId:     customerID,
		DeviceName: "Work Chrome Desktop",
	})
	if err != nil || updChrome.GetSubscription().GetDeviceName() != "Work Chrome Desktop" {
		t.Fatalf("UpdatePushSubscription failed: %v (%+v)", err, updChrome)
	}

	if _, crossErr := grpcClient.GetPushSubscriptions(customerCtx, &notificationpb.GetPushSubscriptionsRequest{UserId: noSubUserID}); status.Code(crossErr) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied on cross-user subscription access, got %v", crossErr)
	}
	t.Logf("[DEMO 2/6] Registered 2 customer browsers + 1 merchant browser and verified gRPC isolation")

	// Step 3: Process Order Placed, Payment, Delivery Created, and Merchant Kafka events + Idempotency + DLQ
	orderPlacedEnv, _ := events.NewEventEnvelope(events.EventTypeOrderCreated, "order-service", events.OrderCreatedEvent{
		OrderID:     "ORD-DEMO-1",
		UserID:      customerID,
		TotalAmount: 149.99,
		Currency:    "USD",
		Status:      "PENDING",
	})
	orderPlacedEnv.CorrelationID = "corr-demo-order-1"

	paymentSuccessEnv, _ := events.NewEventEnvelope(events.EventTypePaymentSuccessful, "payment-service", events.PaymentSuccessfulEvent{
		PaymentID: "PAY-DEMO-1",
		OrderID:   "ORD-DEMO-1",
		UserID:    customerID,
		Amount:    149.99,
		Currency:  "USD",
	})
	paymentSuccessEnv.CorrelationID = "corr-demo-pay-1"

	deliveryAssignedEnv, _ := events.NewEventEnvelope(events.EventTypeDeliveryCreated, "delivery-service", events.DeliveryCreatedEvent{
		DeliveryID:      "DEL-DEMO-1",
		OrderID:         "ORD-DEMO-1",
		DeliveryPartner: "BlueDart Express",
		TrackingNumber:  "BDX-88421",
		Status:          "CREATED",
	})
	deliveryAssignedEnv.CorrelationID = "corr-demo-del-1"

	merchantApprovedEnv, _ := events.NewEventEnvelope(events.EventTypeMerchantApproved, "merchant-service", events.MerchantApprovedEvent{
		MerchantID: merchantID,
		UserID:     merchantID,
		NewStatus:  "APPROVED",
	})
	merchantApprovedEnv.CorrelationID = "corr-demo-merchant-1"

	noSubOrderEnv, _ := events.NewEventEnvelope(events.EventTypeOrderCreated, "order-service", events.OrderCreatedEvent{
		OrderID:     "ORD-NOSUB-9",
		UserID:      noSubUserID,
		TotalAmount: 49.99,
		Currency:    "USD",
	})
	noSubOrderEnv.CorrelationID = "corr-demo-nosub-1"

	for _, env := range []*events.EventEnvelope{
		orderPlacedEnv,
		orderPlacedEnv, // duplicate EventID must be ignored idempotently
		paymentSuccessEnv,
		deliveryAssignedEnv,
		merchantApprovedEnv,
		noSubOrderEnv,
	} {
		if hErr := evtConsumer.HandleEvent(ctx, env); hErr != nil {
			t.Fatalf("HandleEvent(%s) failed: %v", env.EventType, hErr)
		}
	}

	poisonEnv := &events.EventEnvelope{
		EventID:   uuid.NewString(),
		EventType: events.EventTypeOrderCreated,
		Source:    "order-service",
		Timestamp: time.Now().UTC(),
		Data:      []byte("{broken-json"),
	}
	if hErr := evtConsumer.HandleEvent(ctx, poisonEnv); hErr != nil {
		t.Fatalf("expected malformed poison event routed to DLQ without consumer error, got %v", hErr)
	}
	if len(dlqEvents) != 1 || dlqEvents[0].EventID != poisonEnv.EventID {
		t.Fatalf("expected 1 poison event in DLQ, got %+v", dlqEvents)
	}
	if orderLookup.calls.Load() != 1 {
		t.Fatalf("expected OrderUserLookup called once for DeliveryCreated without UserID, got %d", orderLookup.calls.Load())
	}

	// Only Order Placed (OrderCreated) and Delivery Created (DeliveryCreated) trigger push notifications.
	// Customer received 2 push events * 2 devices = 4 push deliveries; other events do not trigger push.
	deliveredStep3 := pushGateway.snapshotDelivered()
	if len(deliveredStep3) != 4 {
		t.Fatalf("expected 4 encrypted+verified push deliveries in Step 3, got %d", len(deliveredStep3))
	}
	for _, msg := range deliveredStep3 {
		t.Logf("[DEMO PUSH WIRE DECRYPTED] device=%s title=%q body=%q event=%s corr=%s",
			msg.DeviceName, msg.Payload.Title, msg.Payload.Body, msg.Payload.EventType, msg.Payload.CorrelationID)
	}
	t.Logf("[DEMO 3/6] Verified Kafka processing, retry recovery (429->201), VAPID JWT + RFC 8291 decryption, idempotency, OrderLookup, 0-sub user & DLQ")

	// Step 4: Register expired Safari endpoint (returns 410 Gone) and trigger DeliveryCreated
	safariGone := createDemoBrowser(t, "Old Safari", "https://web.push.apple.com/demo-safari-gone")
	pushGateway.registerBrowser(safariGone)
	pushGateway.setGone(safariGone.endpoint)

	regSafari, err := grpcClient.RegisterPushSubscription(customerCtx, &notificationpb.RegisterPushSubscriptionRequest{
		UserId:     customerID,
		Endpoint:   safariGone.endpoint,
		P256Dh:     safariGone.p256dh,
		Auth:       safariGone.auth,
		UserAgent:  "Safari/16",
		DeviceName: "Old Safari",
	})
	if err != nil {
		t.Fatalf("RegisterPushSubscription (Safari) failed: %v", err)
	}

	secondDeliveryEnv, _ := events.NewEventEnvelope(events.EventTypeDeliveryCreated, "delivery-service", events.DeliveryCreatedEvent{
		DeliveryID:      "DEL-DEMO-2",
		OrderID:         "ORD-DEMO-1",
		DeliveryPartner: "BlueDart Express",
		TrackingNumber:  "BDX-88422",
		Status:          "CREATED",
	})
	secondDeliveryEnv.CorrelationID = "corr-demo-del-2"
	if err := evtConsumer.HandleEvent(ctx, secondDeliveryEnv); err != nil {
		t.Fatalf("HandleEvent(DeliveryCreated) failed: %v", err)
	}

	// Chrome + Firefox succeed (total delivered goes 4 -> 6); Old Safari deactivated automatically on 410 Gone
	deliveredStep4 := pushGateway.snapshotDelivered()
	if len(deliveredStep4) != 6 {
		t.Fatalf("expected 6 total push deliveries after second delivery created, got %d", len(deliveredStep4))
	}
	subsAfterShipped, err := grpcClient.GetPushSubscriptions(customerCtx, &notificationpb.GetPushSubscriptionsRequest{
		UserId:     customerID,
		ActiveOnly: true,
	})
	if err != nil || subsAfterShipped.GetTotal() != 2 {
		t.Fatalf("expected 2 active customer subscriptions after 410 Gone cleanup, got %d (err=%v)", subsAfterShipped.GetTotal(), err)
	}
	safariAfter, _ := repo.GetSubscriptionByID(ctx, regSafari.GetSubscription().GetId())
	if safariAfter == nil || safariAfter.IsActive || safariAfter.DeactivatedAt == nil {
		t.Fatalf("expected expired Safari subscription auto-deactivated, got %+v", safariAfter)
	}
	t.Logf("[DEMO 4/6] Expired 410 Gone subscription auto-deactivated while active devices still received DeliveryCreated")

	// Step 5: Notification History, Filtering, Pagination, Read/Unread Management & Unsubscribe via gRPC
	page1, err := grpcClient.GetNotifications(customerCtx, &notificationpb.GetNotificationsRequest{
		UserId:  customerID,
		Channel: "PUSH",
		Status:  "SENT",
		Limit:   2,
		Offset:  0,
	})
	if err != nil || page1.GetTotal() != 3 || page1.GetUnreadCount() != 3 || len(page1.GetNotifications()) != 2 {
		t.Fatalf("unexpected page1 history: total=%d unread=%d len=%d err=%v", page1.GetTotal(), page1.GetUnreadCount(), len(page1.GetNotifications()), err)
	}

	deliveryOnly, err := grpcClient.GetNotifications(customerCtx, &notificationpb.GetNotificationsRequest{
		UserId:    customerID,
		EventType: events.EventTypeDeliveryCreated,
	})
	if err != nil || deliveryOnly.GetTotal() != 2 || !strings.Contains(deliveryOnly.GetNotifications()[0].GetSubject(), "Delivery Created") {
		t.Fatalf("unexpected filtered DeliveryCreated result: %+v (err=%v)", deliveryOnly, err)
	}

	// User without push subscriptions still has OrderCreated persisted cleanly as PENDING
	noSubCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs("x-user-id", noSubUserID, "x-user-role", "CUSTOMER"))
	noSubList, err := grpcClient.GetNotifications(noSubCtx, &notificationpb.GetNotificationsRequest{UserId: noSubUserID})
	if err != nil || noSubList.GetTotal() != 1 || noSubList.GetNotifications()[0].GetStatus() != "PENDING" {
		t.Fatalf("expected 1 PENDING history record for zero-subscription user, got %+v (err=%v)", noSubList, err)
	}

	readBool := true
	markResp, err := grpcClient.MarkNotificationAsRead(customerCtx, &notificationpb.MarkNotificationAsReadRequest{
		NotificationId: page1.GetNotifications()[0].GetId(),
		UserId:         customerID,
		IsRead:         &readBool,
	})
	if err != nil || !markResp.GetSuccess() || !markResp.GetNotification().GetIsRead() {
		t.Fatalf("MarkNotificationAsRead failed: %v", err)
	}

	unreadResp, err := grpcClient.GetUnreadCount(customerCtx, &notificationpb.GetUnreadCountRequest{UserId: customerID})
	if err != nil || unreadResp.GetUnreadCount() != 2 {
		t.Fatalf("expected UnreadCount=2, got %d (err=%v)", unreadResp.GetUnreadCount(), err)
	}

	markAllResp, err := grpcClient.MarkAllNotificationsAsRead(customerCtx, &notificationpb.MarkAllNotificationsAsReadRequest{UserId: customerID})
	if err != nil || !markAllResp.GetSuccess() || markAllResp.GetUpdatedCount() != 2 {
		t.Fatalf("MarkAllNotificationsAsRead failed: updated=%d err=%v", markAllResp.GetUpdatedCount(), err)
	}

	deactChrome, err := grpcClient.DeactivatePushSubscription(customerCtx, &notificationpb.DeactivatePushSubscriptionRequest{
		UserId:   customerID,
		Endpoint: chrome.endpoint,
	})
	if err != nil || !deactChrome.GetSuccess() {
		t.Fatalf("DeactivatePushSubscription by endpoint failed: %v", err)
	}
	subsFinal, _ := grpcClient.GetPushSubscriptions(customerCtx, &notificationpb.GetPushSubscriptionsRequest{UserId: customerID, ActiveOnly: true})
	if subsFinal.GetTotal() != 1 || subsFinal.GetSubscriptions()[0].GetEndpoint() != firefox.endpoint {
		t.Fatalf("expected only Firefox remaining active, got %+v", subsFinal.GetSubscriptions())
	}
	t.Logf("[DEMO 5/6] Verified notification history pagination/filtering, read management (4->3->0 unread) & unsubscribe")

	// Step 6: Security & Observability Audit
	logs := logBuf.String()
	if strings.Contains(logs, privVAPID) {
		t.Fatalf("security violation: VAPID private key appeared in service logs")
	}
	if !strings.Contains(logs, "corr-demo-del-2") || !strings.Contains(logs, "corr-demo-order-1") {
		t.Errorf("expected correlation IDs in structured JSON logs")
	}
	t.Logf("[DEMO 6/6] Verified structured JSON logging with correlation IDs and zero credential leaks")
}
