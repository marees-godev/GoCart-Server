package resolvers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	notificationpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/notification"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

func (r *Resolver) getNotificationClient() (notificationpb.NotificationServiceClient, error) {
	if r == nil {
		return nil, appErrors.ServiceUnavailable("notification service unavailable")
	}
	if r.Clients != nil && r.Clients.NotificationClient != nil {
		return r.Clients.NotificationClient, nil
	}
	if r.ClientMgr != nil && r.ClientMgr.NotificationClient != nil {
		return r.ClientMgr.NotificationClient, nil
	}
	return nil, appErrors.ServiceUnavailable("notification service unavailable")
}

func (r *queryResolver) getNotificationClient() (notificationpb.NotificationServiceClient, error) {
	if r == nil || r.Resolver == nil {
		return nil, appErrors.ServiceUnavailable("notification service unavailable")
	}
	return r.Resolver.getNotificationClient()
}

func (r *mutationResolver) getNotificationClient() (notificationpb.NotificationServiceClient, error) {
	if r == nil || r.Resolver == nil {
		return nil, appErrors.ServiceUnavailable("notification service unavailable")
	}
	return r.Resolver.getNotificationClient()
}

func stringValueOrEmpty(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

func (r *Resolver) notifyOrderCreated(ctx context.Context, order *model.Order) {
	if r == nil || order == nil || strings.TrimSpace(order.UserID) == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := r.getNotificationClient()
	if err != nil || client == nil {
		return
	}

	totalStr := fmt.Sprintf("%.2f", order.TotalAmount)
	currency := stringValueOrEmpty(order.Currency)
	eventID := "order-created-" + order.ID

	req := &notificationpb.SendNotificationRequest{
		UserId:    order.UserID,
		Channel:   "PUSH",
		EventType: "OrderCreated",
		Recipient: order.UserID,
		Subject:   fmt.Sprintf("Order Placed - #%s", order.ID),
		Content:   fmt.Sprintf("Your order #%s (%s %s) has been placed.", order.ID, totalStr, currency),
		TemplateData: map[string]string{
			"order_id":     order.ID,
			"user_id":      order.UserID,
			"total_amount": totalStr,
			"currency":     currency,
			"event_id":     eventID,
		},
		CorrelationId: order.ID,
	}

	go func() {
		nCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := client.SendNotification(nCtx, req); err != nil {
			slog.Error("Failed to trigger background notification", "error", err, "event_type", req.EventType)
		}
	}()
}

func (r *Resolver) notifyOrderCancelled(ctx context.Context, order *model.Order, reason string) {
	if r == nil || order == nil || strings.TrimSpace(order.UserID) == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := r.getNotificationClient()
	if err != nil || client == nil {
		return
	}

	body := fmt.Sprintf("Your order #%s has been cancelled.", order.ID)
	if strings.TrimSpace(reason) != "" {
		body = fmt.Sprintf("Your order #%s has been cancelled. Reason: %s.", order.ID, strings.TrimSpace(reason))
	}
	eventID := "order-cancelled-" + order.ID

	req := &notificationpb.SendNotificationRequest{
		UserId:    order.UserID,
		Channel:   "PUSH",
		EventType: "OrderCancelled",
		Recipient: order.UserID,
		Subject:   fmt.Sprintf("Order Cancelled - #%s", order.ID),
		Content:   body,
		TemplateData: map[string]string{
			"order_id": order.ID,
			"user_id":  order.UserID,
			"reason":   reason,
			"event_id": eventID,
		},
		CorrelationId: order.ID,
	}

	go func() {
		nCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := client.SendNotification(nCtx, req); err != nil {
			slog.Error("Failed to trigger background notification", "error", err, "event_type", req.EventType)
		}
	}()
}

func (r *Resolver) notifyOrderStatusUpdated(ctx context.Context, order *model.Order, status string) {
	if r == nil || order == nil || strings.TrimSpace(order.UserID) == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := r.getNotificationClient()
	if err != nil || client == nil {
		return
	}

	var eventType, subj, body string
	stUpper := strings.ToUpper(strings.TrimSpace(status))
	switch stUpper {
	case "CONFIRMED", "PAID":
		eventType = "OrderConfirmed"
		subj = fmt.Sprintf("Order Confirmed - #%s", order.ID)
		body = fmt.Sprintf("Your order #%s is confirmed and being prepared for shipment.", order.ID)
	case "SHIPPED", "OUT_FOR_DELIVERY":
		eventType = "OrderShipped"
		subj = fmt.Sprintf("Order Shipped - #%s", order.ID)
		body = fmt.Sprintf("Your order #%s has shipped.", order.ID)
	case "DELIVERED":
		eventType = "OrderDelivered"
		subj = fmt.Sprintf("Order Delivered - #%s", order.ID)
		body = fmt.Sprintf("Your order #%s has been delivered successfully. Thank you for shopping with GoCart!", order.ID)
	case "CANCELLED":
		eventType = "OrderCancelled"
		subj = fmt.Sprintf("Order Cancelled - #%s", order.ID)
		body = fmt.Sprintf("Your order #%s has been cancelled.", order.ID)
	default:
		eventType = "OrderStatusUpdated"
		subj = fmt.Sprintf("Order Update - #%s", order.ID)
		body = fmt.Sprintf("Your order #%s status has been updated to %s.", order.ID, status)
	}
	eventID := fmt.Sprintf("order-status-%s-%s", order.ID, strings.ToLower(stUpper))

	req := &notificationpb.SendNotificationRequest{
		UserId:    order.UserID,
		Channel:   "PUSH",
		EventType: eventType,
		Recipient: order.UserID,
		Subject:   subj,
		Content:   body,
		TemplateData: map[string]string{
			"order_id": order.ID,
			"user_id":  order.UserID,
			"status":   status,
			"event_id": eventID,
		},
		CorrelationId: order.ID,
	}

	go func() {
		nCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := client.SendNotification(nCtx, req); err != nil {
			slog.Error("Failed to trigger background notification", "error", err, "event_type", req.EventType)
		}
	}()
}

func (r *Resolver) notifyDeliveryCreated(ctx context.Context, delivery *model.Delivery, targetUserID string) {
	if r == nil || delivery == nil || strings.TrimSpace(targetUserID) == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := r.getNotificationClient()
	if err != nil || client == nil {
		return
	}

	eventID := "delivery-created-" + delivery.ID
	partner := delivery.DeliveryPartner
	tracking := stringValueOrEmpty(delivery.TrackingNumber)

	req := &notificationpb.SendNotificationRequest{
		UserId:    targetUserID,
		Channel:   "PUSH",
		EventType: "DeliveryCreated",
		Recipient: targetUserID,
		Subject:   fmt.Sprintf("Delivery Created - Order #%s", delivery.OrderID),
		Content:   fmt.Sprintf("Delivery for order #%s has been created via %s. Tracking: %s", delivery.OrderID, partner, tracking),
		TemplateData: map[string]string{
			"order_id":         delivery.OrderID,
			"delivery_id":      delivery.ID,
			"user_id":          targetUserID,
			"delivery_partner": partner,
			"tracking_number":  tracking,
			"status":           string(delivery.Status),
			"event_id":         eventID,
		},
		CorrelationId: delivery.OrderID,
	}

	go func() {
		nCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := client.SendNotification(nCtx, req); err != nil {
			slog.Error("Failed to trigger background notification", "error", err, "event_type", req.EventType)
		}
	}()
}

func (r *Resolver) notifyDeliveryStatusUpdated(ctx context.Context, delivery *model.Delivery, targetUserID string, status string) {
	if r == nil || delivery == nil || strings.TrimSpace(targetUserID) == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := r.getNotificationClient()
	if err != nil || client == nil {
		return
	}

	eventID := fmt.Sprintf("delivery-status-%s-%s", delivery.ID, strings.ToLower(status))
	partner := delivery.DeliveryPartner
	tracking := stringValueOrEmpty(delivery.TrackingNumber)

	req := &notificationpb.SendNotificationRequest{
		UserId:    targetUserID,
		Channel:   "PUSH",
		EventType: "DeliveryStatusUpdated",
		Recipient: targetUserID,
		Subject:   fmt.Sprintf("Delivery Update - Order #%s", delivery.OrderID),
		Content:   fmt.Sprintf("Delivery status for order #%s (Tracking: %s) updated to %s.", delivery.OrderID, tracking, status),
		TemplateData: map[string]string{
			"order_id":         delivery.OrderID,
			"delivery_id":      delivery.ID,
			"user_id":          targetUserID,
			"delivery_partner": partner,
			"tracking_number":  tracking,
			"new_status":       status,
			"event_id":         eventID,
		},
		CorrelationId: delivery.OrderID,
	}

	go func() {
		nCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := client.SendNotification(nCtx, req); err != nil {
			slog.Error("Failed to trigger background notification", "error", err, "event_type", req.EventType)
		}
	}()
}
