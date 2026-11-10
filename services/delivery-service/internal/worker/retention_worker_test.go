package worker_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/service/shipping_fee"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/worker"
)

type mockDeliveryServiceForWorker struct {
	processedCount atomic.Int32
}

func (m *mockDeliveryServiceForWorker) HandleOrderConfirmed(ctx context.Context, payload *dto.OrderConfirmedPayload) (*model.Delivery, error) {
	return nil, nil
}
func (m *mockDeliveryServiceForWorker) GetDeliveryByOrderID(ctx context.Context, orderID string) (*model.Delivery, error) {
	return nil, nil
}
func (m *mockDeliveryServiceForWorker) GetDeliveryByTrackingNumber(ctx context.Context, trackingNumber string) (*model.Delivery, error) {
	return nil, nil
}
func (m *mockDeliveryServiceForWorker) GetDeliveryByID(ctx context.Context, id string) (*model.Delivery, error) {
	return nil, nil
}
func (m *mockDeliveryServiceForWorker) ListDeliveries(ctx context.Context, limit, offset int, status string) ([]*model.Delivery, int, error) {
	return nil, 0, nil
}
func (m *mockDeliveryServiceForWorker) UpdateStatus(ctx context.Context, id string, targetStatus model.DeliveryStatus) (*model.Delivery, error) {
	return nil, nil
}
func (m *mockDeliveryServiceForWorker) CancelDelivery(ctx context.Context, deliveryID, orderID, reason string) (*model.Delivery, error) {
	return nil, nil
}
func (m *mockDeliveryServiceForWorker) CalculateShippingFee(ctx context.Context, req *shipping_fee.CalculationRequest) (*shipping_fee.CalculationResult, error) {
	return nil, nil
}
func (m *mockDeliveryServiceForWorker) ProcessStatusUpdate(ctx context.Context, req *dto.DeliveryStatusUpdateRequest) (*model.Delivery, error) {
	return nil, nil
}
func (m *mockDeliveryServiceForWorker) EstimateDelivery(ctx context.Context, orderID string, partnerID ...string) (*dto.EstimatedDeliveryResponse, error) {
	return nil, nil
}
func (m *mockDeliveryServiceForWorker) CreateOrUpdatePartnerProfile(ctx context.Context, profile *model.DeliveryPartnerProfile) error {
	return nil
}
func (m *mockDeliveryServiceForWorker) GetPartnerProfile(ctx context.Context, partnerID string) (*model.DeliveryPartnerProfile, error) {
	return nil, nil
}
func (m *mockDeliveryServiceForWorker) UpdatePartnerProfile(ctx context.Context, partnerID string, companyName, email, contactNumber, gstin *string, callerID ...string) (*model.DeliveryPartnerProfile, error) {
	return nil, nil
}
func (m *mockDeliveryServiceForWorker) VerifyGSTIN(ctx context.Context, partnerID string, gstin string, callerID ...string) (*model.DeliveryPartnerProfile, error) {
	return nil, nil
}
func (m *mockDeliveryServiceForWorker) ActivatePartner(ctx context.Context, partnerID string, callerID ...string) (*model.DeliveryPartnerProfile, error) {
	return nil, nil
}
func (m *mockDeliveryServiceForWorker) DeactivatePartner(ctx context.Context, partnerID string, reason string, callerID ...string) (*model.DeliveryPartnerProfile, error) {
	return nil, nil
}
func (m *mockDeliveryServiceForWorker) DeletePartnerProfile(ctx context.Context, partnerID string, reason string, callerID ...string) error {
	return nil
}
func (m *mockDeliveryServiceForWorker) ApprovePartner(ctx context.Context, partnerID string) (*model.DeliveryPartnerProfile, error) {
	return nil, nil
}
func (m *mockDeliveryServiceForWorker) RejectPartner(ctx context.Context, partnerID string, reason string) (*model.DeliveryPartnerProfile, error) {
	return nil, nil
}
func (m *mockDeliveryServiceForWorker) AppealRejection(ctx context.Context, partnerID string, reason string, callerID ...string) (*model.DeliveryPartnerProfile, error) {
	return nil, nil
}
func (m *mockDeliveryServiceForWorker) ListAppealedPartners(ctx context.Context, limit, offset int) ([]*model.DeliveryPartnerProfile, int, error) {
	return nil, 0, nil
}
func (m *mockDeliveryServiceForWorker) ListApprovedPartners(ctx context.Context, limit, offset int) ([]*model.DeliveryPartnerProfile, int, error) {
	return nil, 0, nil
}
func (m *mockDeliveryServiceForWorker) ProcessExpiredDeactivations(ctx context.Context, retentionPeriod time.Duration) (int, error) {
	m.processedCount.Add(1)
	return 1, nil
}

func TestRetentionWorker_Lifecycle(t *testing.T) {
	svc := &mockDeliveryServiceForWorker{}
	w := worker.NewRetentionWorker(svc, 10*time.Millisecond, 10*time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	time.Sleep(35 * time.Millisecond)
	cancel()

	select {
	case <-w.Done():
	case <-time.After(1 * time.Second):
		t.Fatal("worker did not shut down in time")
	}

	count := svc.processedCount.Load()
	if count < 2 {
		t.Errorf("expected at least 2 processed calls, got %d", count)
	}
}
