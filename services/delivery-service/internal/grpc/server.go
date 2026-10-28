package grpc

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	deliverypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/delivery"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/service"
)

type DeliveryGRPCServer struct {
	deliverypb.UnimplementedDeliveryServiceServer
	deliveryService service.DeliveryService
	logger          *slog.Logger
}

func NewDeliveryGRPCServer(svc service.DeliveryService, log ...*slog.Logger) *DeliveryGRPCServer {
	var l *slog.Logger
	if len(log) > 0 && log[0] != nil {
		l = log[0]
	} else {
		l = slog.Default()
	}
	return &DeliveryGRPCServer{
		deliveryService: svc,
		logger:          l,
	}
}

func (s *DeliveryGRPCServer) CreateDelivery(ctx context.Context, req *deliverypb.CreateDeliveryRequest) (*deliverypb.CreateDeliveryResponse, error) {
	if req == nil {
		s.logger.Warn("CreateDelivery: request is nil")
		return nil, appErrors.BadRequest("request is required").ToGRPC()
	}

	var addr model.ShippingAddress
	if req.ShippingAddress != "" {
		addr = model.ParseShippingAddress(req.ShippingAddress)
	}

	if addr.ContactNumber == "" && req.ContactNumber != "" {
		addr.ContactNumber = req.ContactNumber
	}

	payload := &dto.OrderConfirmedPayload{
		OrderID:         req.OrderId,
		DeliveryPartner: req.DeliveryPartner,
		ShippingAddress: &addr,
	}

	s.logger.Info("CreateDelivery: creating delivery", slog.String("order_id", req.OrderId), slog.String("delivery_partner", req.DeliveryPartner))
	delivery, err := s.deliveryService.HandleOrderConfirmed(ctx, payload)
	if err != nil {
		s.logger.Error("CreateDelivery failed", slog.String("order_id", req.OrderId), slog.Any("error", err))
		return nil, appErrors.ToGRPC(err)
	}

	return &deliverypb.CreateDeliveryResponse{
		Delivery: toProtoDelivery(delivery),
	}, nil
}

func (s *DeliveryGRPCServer) GetDelivery(ctx context.Context, req *deliverypb.GetDeliveryRequest) (*deliverypb.GetDeliveryResponse, error) {
	if req == nil || strings.TrimSpace(req.Id) == "" {
		s.logger.Warn("GetDelivery: id is required")
		return nil, appErrors.BadRequest("id is required").ToGRPC()
	}

	delivery, err := s.deliveryService.GetDeliveryByID(ctx, strings.TrimSpace(req.Id))
	if err != nil {
		appErr := appErrors.AsAppError(err)
		if appErr != nil && appErr.HTTPStatus < 500 {
			s.logger.Warn("GetDelivery client error", slog.String("id", req.Id), slog.Any("error", err))
		} else {
			s.logger.Error("GetDelivery failed", slog.String("id", req.Id), slog.Any("error", err))
		}
		return nil, appErrors.ToGRPC(err)
	}

	return &deliverypb.GetDeliveryResponse{
		Delivery: toProtoDelivery(delivery),
	}, nil
}

func (s *DeliveryGRPCServer) GetDeliveryByTracking(ctx context.Context, req *deliverypb.GetDeliveryByTrackingRequest) (*deliverypb.GetDeliveryResponse, error) {
	if req == nil || strings.TrimSpace(req.TrackingNumber) == "" {
		s.logger.Warn("GetDeliveryByTracking: tracking_number is required")
		return nil, appErrors.BadRequest("tracking_number is required").ToGRPC()
	}

	delivery, err := s.deliveryService.GetDeliveryByTrackingNumber(ctx, strings.TrimSpace(req.TrackingNumber))
	if err != nil {
		appErr := appErrors.AsAppError(err)
		if appErr != nil && appErr.HTTPStatus < 500 {
			s.logger.Warn("GetDeliveryByTracking client error", slog.String("tracking_number", req.TrackingNumber), slog.Any("error", err))
		} else {
			s.logger.Error("GetDeliveryByTracking failed", slog.String("tracking_number", req.TrackingNumber), slog.Any("error", err))
		}
		return nil, appErrors.ToGRPC(err)
	}

	return &deliverypb.GetDeliveryResponse{
		Delivery: toProtoDelivery(delivery),
	}, nil
}

func (s *DeliveryGRPCServer) GetDeliveryByOrder(ctx context.Context, req *deliverypb.GetDeliveryByOrderRequest) (*deliverypb.GetDeliveryResponse, error) {
	if req == nil || strings.TrimSpace(req.OrderId) == "" {
		s.logger.Warn("GetDeliveryByOrder: order_id is required")
		return nil, appErrors.BadRequest("order_id is required").ToGRPC()
	}

	delivery, err := s.deliveryService.GetDeliveryByOrderID(ctx, strings.TrimSpace(req.OrderId))
	if err != nil {
		appErr := appErrors.AsAppError(err)
		if appErr != nil && appErr.HTTPStatus < 500 {
			s.logger.Warn("GetDeliveryByOrder client error", slog.String("order_id", req.OrderId), slog.Any("error", err))
		} else {
			s.logger.Error("GetDeliveryByOrder failed", slog.String("order_id", req.OrderId), slog.Any("error", err))
		}
		return nil, appErrors.ToGRPC(err)
	}

	return &deliverypb.GetDeliveryResponse{
		Delivery: toProtoDelivery(delivery),
	}, nil
}

func (s *DeliveryGRPCServer) UpdateDeliveryStatus(ctx context.Context, req *deliverypb.UpdateDeliveryStatusRequest) (*deliverypb.UpdateDeliveryStatusResponse, error) {
	if req == nil || strings.TrimSpace(req.Id) == "" {
		s.logger.Warn("UpdateDeliveryStatus: id is required")
		return nil, appErrors.BadRequest("id is required").ToGRPC()
	}

	s.logger.Info("UpdateDeliveryStatus: updating status", slog.String("id", req.Id), slog.String("target_status", req.Status))
	delivery, err := s.deliveryService.ProcessStatusUpdate(ctx, &dto.DeliveryStatusUpdateRequest{
		DeliveryID: strings.TrimSpace(req.Id),
		Status:     model.DeliveryStatus(req.Status),
		Location:   strings.TrimSpace(req.Location),
	})
	if err != nil {
		appErr := appErrors.AsAppError(err)
		if appErr != nil && appErr.HTTPStatus < 500 {
			s.logger.Warn("UpdateDeliveryStatus client error", slog.String("id", req.Id), slog.Any("error", err))
		} else {
			s.logger.Error("UpdateDeliveryStatus failed", slog.String("id", req.Id), slog.Any("error", err))
		}
		return nil, appErrors.ToGRPC(err)
	}

	return &deliverypb.UpdateDeliveryStatusResponse{
		Delivery: toProtoDelivery(delivery),
	}, nil
}

func (s *DeliveryGRPCServer) CreatePartnerProfile(ctx context.Context, req *deliverypb.CreatePartnerProfileRequest) (*deliverypb.PartnerProfileResponse, error) {
	if req == nil || strings.TrimSpace(req.PartnerId) == "" {
		s.logger.Warn("CreatePartnerProfile: partner_id is required")
		return nil, appErrors.BadRequest("partner_id is required").ToGRPC()
	}

	pUUID, err := uuid.Parse(strings.TrimSpace(req.PartnerId))
	if err != nil {
		s.logger.Warn("CreatePartnerProfile: invalid partner_id", slog.String("partner_id", req.PartnerId))
		return nil, appErrors.BadRequest("invalid partner_id format").ToGRPC()
	}

	profile := &model.DeliveryPartnerProfile{
		ID:            uuid.New(),
		PartnerID:     pUUID,
		CompanyName:   strings.TrimSpace(req.CompanyName),
		Email:         strings.TrimSpace(req.Email),
		ContactNumber: strings.TrimSpace(req.ContactNumber),
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	if err := s.deliveryService.CreateOrUpdatePartnerProfile(ctx, profile); err != nil {
		s.logger.Error("CreatePartnerProfile failed", slog.String("partner_id", req.PartnerId), slog.Any("error", err))
		return nil, appErrors.ToGRPC(err)
	}

	saved, err := s.deliveryService.GetPartnerProfile(ctx, req.PartnerId)
	if err == nil && saved != nil {
		profile = saved
	}

	return &deliverypb.PartnerProfileResponse{
		Profile: toProtoPartnerProfile(profile),
	}, nil
}

func (s *DeliveryGRPCServer) GetPartnerProfile(ctx context.Context, req *deliverypb.GetPartnerProfileRequest) (*deliverypb.PartnerProfileResponse, error) {
	if req == nil || strings.TrimSpace(req.PartnerId) == "" {
		s.logger.Warn("GetPartnerProfile: partner_id is required")
		return nil, appErrors.BadRequest("partner_id is required").ToGRPC()
	}

	profile, err := s.deliveryService.GetPartnerProfile(ctx, strings.TrimSpace(req.PartnerId))
	if err != nil {
		s.logger.Warn("GetPartnerProfile failed", slog.String("partner_id", req.PartnerId), slog.Any("error", err))
		return nil, appErrors.ToGRPC(err)
	}

	return &deliverypb.PartnerProfileResponse{
		Profile: toProtoPartnerProfile(profile),
	}, nil
}

func (s *DeliveryGRPCServer) UpdatePartnerProfile(ctx context.Context, req *deliverypb.UpdatePartnerProfileRequest) (*deliverypb.PartnerProfileResponse, error) {
	if req == nil || strings.TrimSpace(req.PartnerId) == "" {
		s.logger.Warn("UpdatePartnerProfile: partner_id is required")
		return nil, appErrors.BadRequest("partner_id is required").ToGRPC()
	}

	profile, err := s.deliveryService.UpdatePartnerProfile(ctx, strings.TrimSpace(req.PartnerId), req.CompanyName, req.Email, req.ContactNumber)
	if err != nil {
		s.logger.Error("UpdatePartnerProfile failed", slog.String("partner_id", req.PartnerId), slog.Any("error", err))
		return nil, appErrors.ToGRPC(err)
	}

	return &deliverypb.PartnerProfileResponse{
		Profile: toProtoPartnerProfile(profile),
	}, nil
}



func toProtoDelivery(d *model.Delivery) *deliverypb.Delivery {
	if d == nil {
		return nil
	}
	shippingAddr := d.ShippingAddress.Street
	if shippingAddr == "" || d.ShippingAddress.City != "" || d.ShippingAddress.State != "" {
		addrBytes, err := json.Marshal(d.ShippingAddress)
		if err == nil {
			shippingAddr = string(addrBytes)
		}
	}
	res := &deliverypb.Delivery{
		Id:              d.ID,
		OrderId:         d.OrderID,
		TrackingNumber:  d.TrackingNumber,
		DeliveryPartner: d.DeliveryPartner,
		Status:          string(d.Status),
		ShippingAddress: shippingAddr,
		ContactNumber:   d.ShippingAddress.ContactNumber,
		CreatedAt:       d.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       d.UpdatedAt.Format(time.RFC3339),
	}
	if d.EstimatedDelivery != nil {
		res.EstimatedDeliveryAt = d.EstimatedDelivery.Format(time.RFC3339)
	}
	if d.ActualDelivery != nil {
		res.DeliveredAt = d.ActualDelivery.Format(time.RFC3339)
	}
	return res
}

func toProtoPartnerProfile(p *model.DeliveryPartnerProfile) *deliverypb.DeliveryPartnerProfile {
	if p == nil {
		return nil
	}
	return &deliverypb.DeliveryPartnerProfile{
		Id:            p.ID.String(),
		PartnerId:     p.PartnerID.String(),
		CompanyName:   p.CompanyName,
		Email:         p.Email,
		ContactNumber: p.ContactNumber,
		IsActive:      p.IsActive,
		CreatedAt:     p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     p.UpdatedAt.Format(time.RFC3339),
	}
}


