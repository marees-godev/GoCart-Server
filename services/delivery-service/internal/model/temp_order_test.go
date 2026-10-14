package model_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
)

func TestTempOrder_Initialization(t *testing.T) {
	orderID := uuid.NewString()
	now := time.Now().UTC()
	shippingAddr := model.ShippingAddress{
		Street:        "123 Delivery Way",
		City:          "San Francisco",
		State:         "CA",
		PostalCode:    "94105",
		Country:       "USA",
		ContactNumber: "+14155552671",
	}

	order := &model.TempOrder{
		ID:              orderID,
		OrderNumber:     "ORD-12345678",
		Status:          model.TempOrderStatusConfirmed,
		TotalAmount:     99.99,
		Currency:        "USD",
		ShippingAddress: shippingAddr,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if order.ID != orderID {
		t.Fatalf("expected order ID %s, got %s", orderID, order.ID)
	}
	if order.OrderNumber != "ORD-12345678" {
		t.Fatalf("expected order number ORD-12345678, got %s", order.OrderNumber)
	}
	if order.Status != model.TempOrderStatusConfirmed {
		t.Fatalf("expected order status %s, got %s", model.TempOrderStatusConfirmed, order.Status)
	}
	if order.TotalAmount != 99.99 {
		t.Fatalf("expected total amount 99.99, got %f", order.TotalAmount)
	}
	if order.Currency != "USD" {
		t.Fatalf("expected currency USD, got %s", order.Currency)
	}
	if order.ShippingAddress != shippingAddr {
		t.Fatalf("expected shipping address %+v, got %+v", shippingAddr, order.ShippingAddress)
	}
	if !order.CreatedAt.Equal(now) {
		t.Fatalf("expected created at %v, got %v", now, order.CreatedAt)
	}
	if !order.UpdatedAt.Equal(now) {
		t.Fatalf("expected updated at %v, got %v", now, order.UpdatedAt)
	}
}
