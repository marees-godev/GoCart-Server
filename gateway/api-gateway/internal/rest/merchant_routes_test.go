package rest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/rest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	testAdminKey = "test-secret-adminkey-456"
)

type mockMerchantClient struct {
	merchantpb.MerchantServiceClient
	mu        sync.Mutex
	merchants map[string]*merchantpb.MerchantResponseData
}

func newMockMerchantClient() *mockMerchantClient {
	return &mockMerchantClient{
		merchants: make(map[string]*merchantpb.MerchantResponseData),
	}
}

func (m *mockMerchantClient) GetMerchant(ctx context.Context, req *merchantpb.GetMerchantRequest, opts ...grpc.CallOption) (*merchantpb.GetMerchantResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	merch, exists := m.merchants[req.Id]
	if !exists {
		return nil, status.Error(codes.NotFound, "merchant not found")
	}
	return &merchantpb.GetMerchantResponse{Merchant: merch}, nil
}

func (m *mockMerchantClient) ListMerchants(ctx context.Context, req *merchantpb.ListMerchantsRequest, opts ...grpc.CallOption) (*merchantpb.ListMerchantsResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var list []*merchantpb.MerchantResponseData
	for _, merch := range m.merchants {
		if req.Status == nil || merch.Status == *req.Status {
			list = append(list, merch)
		}
	}
	return &merchantpb.ListMerchantsResponse{
		Merchants: list,
		Total:     int32(len(list)),
	}, nil
}

func (m *mockMerchantClient) UpdateMerchant(ctx context.Context, req *merchantpb.UpdateMerchantRequest, opts ...grpc.CallOption) (*merchantpb.UpdateMerchantResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	merch, exists := m.merchants[req.Id]
	if !exists {
		return nil, status.Error(codes.NotFound, "merchant not found")
	}

	if req.BusinessName != "" {
		merch.BusinessName = req.BusinessName
	}
	if req.FirstName != "" {
		merch.FirstName = req.FirstName
	}
	if req.LastName != "" {
		merch.LastName = req.LastName
	}
	if req.BusinessPhone != "" {
		merch.BusinessPhone = req.BusinessPhone
	}
	if req.PanCardNumber != "" {
		merch.PanCardNumber = req.PanCardNumber
	}
	merch.UpdatedAt = timestamppb.Now()
	return &merchantpb.UpdateMerchantResponse{Merchant: merch}, nil
}

func (m *mockMerchantClient) UpdateMerchantStatus(ctx context.Context, req *merchantpb.UpdateMerchantStatusRequest, opts ...grpc.CallOption) (*merchantpb.UpdateMerchantStatusResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	merch, exists := m.merchants[req.Id]
	if !exists {
		return nil, status.Error(codes.NotFound, "merchant not found")
	}

	allowedMap := map[string][]string{
		"PENDING":   {"APPROVED", "REJECTED"},
		"APPROVED":  {"SUSPENDED"},
		"REJECTED":  {},
		"SUSPENDED": {},
	}

	fromStatus := merch.Status.String()
	toStatus := req.Status.String()
	targets := allowedMap[fromStatus]
	allowed := false
	for _, t := range targets {
		if t == toStatus {
			allowed = true
			break
		}
	}

	if !allowed {
		formatTargets := "[]"
		if len(targets) > 0 {
			var formatted []string
			for _, t := range targets {
				formatted = append(formatted, fmt.Sprintf("'%s'", t))
			}
			formatTargets = "[" + strings.Join(formatted, ", ") + "]"
		}
		errMsg := fmt.Sprintf("Cannot transition merchant from '%s' to '%s'. Allowed targets: %s.", fromStatus, toStatus, formatTargets)
		return nil, status.Error(codes.FailedPrecondition, errMsg)
	}

	prevStatus := merch.Status
	merch.Status = req.Status
	merch.RejectionReason = req.RejectionReason
	merch.UpdatedAt = timestamppb.Now()

	return &merchantpb.UpdateMerchantStatusResponse{
		Merchant:       merch,
		PreviousStatus: prevStatus,
	}, nil
}

func setupRestTestApp(client *mockMerchantClient) *fiber.App {
	app := fiber.New()
	cfg := &config.Config{
		AdminAPIKey: testAdminKey,
	}
	rest.RegisterMerchantRoutes(app, client, cfg)
	return app
}

func TestAdminKeySecurityGuard(t *testing.T) {
	client := newMockMerchantClient()
	app := setupRestTestApp(client)

	merchantID := uuid.New().String()
	client.merchants[merchantID] = &merchantpb.MerchantResponseData{
		Id:           merchantID,
		BusinessName: "Acme Store",
		Status:       merchantpb.MerchantStatus_PENDING,
	}

	payload := `{"status": "APPROVED", "reason": "KYC passed"}`

	// 1. Missing adminkey header -> 401 Unauthorized
	req1 := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/admin/merchants/%s/status", merchantID), bytes.NewReader([]byte(payload)))
	req1.Header.Set("Content-Type", "application/json")
	resp1, err := app.Test(req1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp1.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401 for missing adminkey, got %d", resp1.StatusCode)
	}

	// 2. Blank adminkey header -> 401 Unauthorized
	req2 := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/admin/merchants/%s/status", merchantID), bytes.NewReader([]byte(payload)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("adminkey", "")
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401 for blank adminkey, got %d", resp2.StatusCode)
	}

	// 3. Invalid adminkey header -> 401 Unauthorized
	req3 := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/admin/merchants/%s/status", merchantID), bytes.NewReader([]byte(payload)))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("adminkey", "wrong-secret-key")
	resp3, err := app.Test(req3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp3.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401 for invalid adminkey, got %d", resp3.StatusCode)
	}

	// 4. Valid adminkey header -> 200 OK
	req4 := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/admin/merchants/%s/status", merchantID), bytes.NewReader([]byte(payload)))
	req4.Header.Set("Content-Type", "application/json")
	req4.Header.Set("adminkey", testAdminKey)
	resp4, err := app.Test(req4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp4.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 for valid adminkey, got %d", resp4.StatusCode)
	}
}

func TestUpdateMerchantStatus_Success(t *testing.T) {
	client := newMockMerchantClient()
	app := setupRestTestApp(client)

	merchantID := "merch_98765"
	client.merchants[merchantID] = &merchantpb.MerchantResponseData{
		Id:           merchantID,
		BusinessName: "Acme Retail Ltd",
		Status:       merchantpb.MerchantStatus_PENDING,
		CreatedAt:    timestamppb.New(time.Now()),
		UpdatedAt:    timestamppb.New(time.Now()),
	}

	body := `{"status": "APPROVED", "reason": "Merchant KYC verified and approved by compliance team"}`
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/admin/merchants/%s/status", merchantID), bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("adminkey", testAdminKey)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var res map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res["merchantId"] != merchantID {
		t.Errorf("expected merchantId %s, got %v", merchantID, res["merchantId"])
	}
	if res["name"] != "Acme Retail Ltd" {
		t.Errorf("expected name 'Acme Retail Ltd', got %v", res["name"])
	}
	if res["status"] != "APPROVED" {
		t.Errorf("expected status APPROVED, got %v", res["status"])
	}
	if res["previousStatus"] != "PENDING" {
		t.Errorf("expected previousStatus PENDING, got %v", res["previousStatus"])
	}
	if res["updatedAt"] == nil || res["updatedAt"] == "" {
		t.Errorf("expected updatedAt timestamp, got %v", res["updatedAt"])
	}

	// Next valid transition: APPROVED -> SUSPENDED
	suspendBody := `{"status": "SUSPENDED", "reason": "Administrative suspension"}`
	reqSuspend := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/admin/merchants/%s/status", merchantID), bytes.NewReader([]byte(suspendBody)))
	reqSuspend.Header.Set("Content-Type", "application/json")
	reqSuspend.Header.Set("adminkey", testAdminKey)

	respSuspend, err := app.Test(reqSuspend)
	if err != nil {
		t.Fatalf("unexpected error on suspend: %v", err)
	}
	if respSuspend.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 on suspend, got %d", respSuspend.StatusCode)
	}

	var resSuspend map[string]interface{}
	_ = json.NewDecoder(respSuspend.Body).Decode(&resSuspend)
	if resSuspend["status"] != "SUSPENDED" {
		t.Errorf("expected status SUSPENDED, got %v", resSuspend["status"])
	}
	if resSuspend["previousStatus"] != "APPROVED" {
		t.Errorf("expected previousStatus APPROVED, got %v", resSuspend["previousStatus"])
	}
}

func TestUpdateMerchantStatus_InvalidTransitions_Return422(t *testing.T) {
	client := newMockMerchantClient()
	app := setupRestTestApp(client)

	// 1. PENDING -> SUSPENDED (Cannot suspend before approval) -> 422
	m1ID := "m-pending-1"
	client.merchants[m1ID] = &merchantpb.MerchantResponseData{
		Id:           m1ID,
		BusinessName: "Pending Merchant",
		Status:       merchantpb.MerchantStatus_PENDING,
	}

	body1 := `{"status": "SUSPENDED", "reason": "test"}`
	req1 := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/admin/merchants/%s/status", m1ID), bytes.NewReader([]byte(body1)))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("adminkey", testAdminKey)

	resp1, err := app.Test(req1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp1.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected status 422 for PENDING -> SUSPENDED, got %d", resp1.StatusCode)
	}

	var res1 map[string]interface{}
	_ = json.NewDecoder(resp1.Body).Decode(&res1)
	if res1["error"] != "INVALID_STATE_TRANSITION" {
		t.Errorf("expected error code INVALID_STATE_TRANSITION, got %v", res1["error"])
	}
	expectedMsg1 := "Cannot transition merchant from 'PENDING' to 'SUSPENDED'. Allowed targets: ['APPROVED', 'REJECTED']."
	if res1["message"] != expectedMsg1 {
		t.Errorf("expected message:\n%q\ngot:\n%q", expectedMsg1, res1["message"])
	}

	// 2. REJECTED -> APPROVED (Terminal state; cannot transition out) -> 422
	m2ID := "m-rejected-2"
	client.merchants[m2ID] = &merchantpb.MerchantResponseData{
		Id:           m2ID,
		BusinessName: "Rejected Merchant",
		Status:       merchantpb.MerchantStatus_REJECTED,
	}

	body2 := `{"status": "APPROVED", "reason": "re-approval attempt"}`
	req2 := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/admin/merchants/%s/status", m2ID), bytes.NewReader([]byte(body2)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("adminkey", testAdminKey)

	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp2.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected status 422 for REJECTED -> APPROVED, got %d", resp2.StatusCode)
	}
	var res2 map[string]interface{}
	_ = json.NewDecoder(resp2.Body).Decode(&res2)
	expectedMsg2 := "Cannot transition merchant from 'REJECTED' to 'APPROVED'. Allowed targets: []."
	if res2["message"] != expectedMsg2 {
		t.Errorf("expected message:\n%q\ngot:\n%q", expectedMsg2, res2["message"])
	}

	// 3. Same-state transition: APPROVED -> APPROVED -> 422
	m3ID := "m-approved-3"
	client.merchants[m3ID] = &merchantpb.MerchantResponseData{
		Id:           m3ID,
		BusinessName: "Approved Merchant",
		Status:       merchantpb.MerchantStatus_APPROVED,
	}

	body3 := `{"status": "APPROVED", "reason": "duplicate approve"}`
	req3 := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/admin/merchants/%s/status", m3ID), bytes.NewReader([]byte(body3)))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("adminkey", testAdminKey)

	resp3, err := app.Test(req3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp3.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("expected status 422 for APPROVED -> APPROVED, got %d", resp3.StatusCode)
	}
}

func TestUpdateMerchantStatus_BadRequests(t *testing.T) {
	client := newMockMerchantClient()
	app := setupRestTestApp(client)

	merchantID := "merch-1"
	client.merchants[merchantID] = &merchantpb.MerchantResponseData{
		Id:     merchantID,
		Status: merchantpb.MerchantStatus_PENDING,
	}

	// 1. Missing status field
	req1 := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/admin/merchants/%s/status", merchantID), bytes.NewReader([]byte(`{"reason": "no status"}`)))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("adminkey", testAdminKey)
	resp1, _ := app.Test(req1)
	if resp1.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for missing status, got %d", resp1.StatusCode)
	}

	// 2. Unknown enum value
	req2 := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/admin/merchants/%s/status", merchantID), bytes.NewReader([]byte(`{"status": "UNKNOWN_STATUS"}`)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("adminkey", testAdminKey)
	resp2, _ := app.Test(req2)
	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for unknown enum value, got %d", resp2.StatusCode)
	}

	// 3. Invalid JSON
	req3 := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/admin/merchants/%s/status", merchantID), bytes.NewReader([]byte(`{not-json}`)))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("adminkey", testAdminKey)
	resp3, _ := app.Test(req3)
	if resp3.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid JSON, got %d", resp3.StatusCode)
	}
}

func TestUpdateMerchantStatus_NotFound(t *testing.T) {
	client := newMockMerchantClient()
	app := setupRestTestApp(client)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/merchants/non-existent-id/status", bytes.NewReader([]byte(`{"status": "APPROVED"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("adminkey", testAdminKey)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404 for non-existent merchant, got %d", resp.StatusCode)
	}
}

func TestMerchantQueryIntegration_IncludesStatus(t *testing.T) {
	client := newMockMerchantClient()
	app := setupRestTestApp(client)

	merchantID := "merch_lookup_1"
	client.merchants[merchantID] = &merchantpb.MerchantResponseData{
		Id:            merchantID,
		BusinessName:  "Lookup Store",
		BusinessEmail: "lookup@test.com",
		BusinessPhone: "+1234567890",
		PanCardNumber: "TAX-12345",
		Status:        merchantpb.MerchantStatus_APPROVED,
		CreatedAt:     timestamppb.New(time.Now()),
		UpdatedAt:     timestamppb.New(time.Now()),
	}

	// 1. GET /api/v1/merchants/{id}
	reqGet := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/merchants/%s", merchantID), nil)
	respGet, err := app.Test(reqGet)
	if err != nil {
		t.Fatalf("unexpected error on get: %v", err)
	}
	if respGet.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", respGet.StatusCode)
	}

	var resGet map[string]interface{}
	_ = json.NewDecoder(respGet.Body).Decode(&resGet)
	if resGet["status"] != "APPROVED" {
		t.Errorf("expected status 'APPROVED' in merchant lookup response, got %v", resGet["status"])
	}
	if resGet["merchantId"] != merchantID {
		t.Errorf("expected merchantId %s, got %v", merchantID, resGet["merchantId"])
	}

	// 2. GET /api/v1/merchants (List all merchant details)
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/merchants?limit=10", nil)
	respList, err := app.Test(reqList)
	if err != nil {
		t.Fatalf("unexpected error on list: %v", err)
	}
	if respList.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", respList.StatusCode)
	}

	var resList struct {
		Merchants []map[string]interface{} `json:"merchants"`
		Total     int                      `json:"total"`
	}
	_ = json.NewDecoder(respList.Body).Decode(&resList)
	if resList.Total != 1 {
		t.Errorf("expected total 1, got %d", resList.Total)
	}
	if len(resList.Merchants) != 1 {
		t.Fatalf("expected 1 merchant in list, got %d", len(resList.Merchants))
	}
	if resList.Merchants[0]["status"] != "APPROVED" {
		t.Errorf("expected merchant in list to have status 'APPROVED', got %v", resList.Merchants[0]["status"])
	}
}

func TestGenericProfileUpdate_DisallowsStatusModification(t *testing.T) {
	client := newMockMerchantClient()
	app := setupRestTestApp(client)

	merchantID := "merch_put_1"
	client.merchants[merchantID] = &merchantpb.MerchantResponseData{
		Id:           merchantID,
		BusinessName: "Original Name",
		Status:       merchantpb.MerchantStatus_PENDING,
	}

	// Attempting to update status via PUT /api/v1/merchants/{id} should be rejected
	body := `{"businessName": "New Name", "status": "APPROVED"}`
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/merchants/%s", merchantID), bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request when attempting to modify status via generic profile endpoint, got %d", resp.StatusCode)
	}

	// Status remains PENDING
	if client.merchants[merchantID].Status != merchantpb.MerchantStatus_PENDING {
		t.Errorf("status was illegally modified to %s", client.merchants[merchantID].Status)
	}
}
