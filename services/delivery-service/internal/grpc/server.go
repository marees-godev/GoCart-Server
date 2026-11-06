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
	gstinClientPkg "github.com/marees-godev/GoCart-Server/services/delivery-service/internal/client/gstin"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/service"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/service/shipping_fee"
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

	cleanShippingAddressID := strings.TrimSpace(req.ShippingAddressId)
	if cleanShippingAddressID == "" {
		s.logger.Warn("CreateDelivery: shipping_address_id is mandatory")
		return nil, appErrors.BadRequest("shipping_address_id is mandatory").ToGRPC()
	}

	var addr model.ShippingAddress
	if req.ShippingAddress != "" {
		addr = model.ParseShippingAddress(req.ShippingAddress)
	}

	if addr.ContactNumber == "" && req.ContactNumber != "" {
		addr.ContactNumber = req.ContactNumber
	}

	payload := &dto.OrderConfirmedPayload{
		OrderID:           req.OrderId,
		DeliveryPartner:   req.DeliveryPartner,
		ShippingAddress:   &addr,
		ShippingAddressID: cleanShippingAddressID,
	}

	s.logger.Info("CreateDelivery: creating delivery", slog.String("order_id", req.OrderId), slog.String("delivery_partner", req.DeliveryPartner), slog.String("shipping_address_id", cleanShippingAddressID))
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

func (s *DeliveryGRPCServer) CancelDelivery(ctx context.Context, req *deliverypb.CancelDeliveryRequest) (*deliverypb.CancelDeliveryResponse, error) {
	if req == nil {
		s.logger.Warn("CancelDelivery: request is nil")
		return nil, appErrors.BadRequest("request is required").ToGRPC()
	}

	deliveryID := strings.TrimSpace(req.DeliveryId)
	orderID := strings.TrimSpace(req.OrderId)
	if deliveryID == "" && orderID == "" {
		s.logger.Warn("CancelDelivery: missing delivery_id or order_id")
		return nil, appErrors.BadRequest("delivery_id or order_id is required").ToGRPC()
	}

	s.logger.Info("CancelDelivery: processing delivery cancellation", slog.String("delivery_id", deliveryID), slog.String("order_id", orderID), slog.String("reason", req.Reason))
	delivery, err := s.deliveryService.CancelDelivery(ctx, deliveryID, orderID, req.Reason)
	if err != nil {
		s.logger.Warn("CancelDelivery rejected or failed", slog.String("delivery_id", deliveryID), slog.String("order_id", orderID), slog.Any("error", err))
		return nil, appErrors.ToGRPC(err)
	}

	return &deliverypb.CancelDeliveryResponse{
		Delivery: toProtoDelivery(delivery),
		Success:  true,
		Message:  "delivery cancelled successfully",
	}, nil
}

func (s *DeliveryGRPCServer) CalculateShippingFee(ctx context.Context, req *deliverypb.CalculateShippingFeeRequest) (*deliverypb.CalculateShippingFeeResponse, error) {
	if req == nil {
		return nil, appErrors.BadRequest("request is required").ToGRPC()
	}

	calcItems := make([]shipping_fee.CalculationItem, 0, len(req.Items))
	for _, it := range req.Items {
		calcItems = append(calcItems, shipping_fee.CalculationItem{
			ProductID:   it.ProductId,
			Quantity:    int(it.Quantity),
			WeightGrams: it.WeightGrams,
		})
	}

	calcReq := &shipping_fee.CalculationRequest{
		WeightGrams:           req.WeightGrams,
		Zone:                  req.Zone,
		OriginCity:            req.OriginCity,
		OriginState:           req.OriginState,
		DestinationCity:       req.DestinationCity,
		DestinationState:      req.DestinationState,
		DestinationPostalCode: req.DestinationPostalCode,
		ShippingAddress:       req.ShippingAddress,
		Items:                 calcItems,
		Model:                 req.Model,
		PartnerID:             req.PartnerId,
	}

	res, err := s.deliveryService.CalculateShippingFee(ctx, calcReq)
	if err != nil {
		s.logger.Error("CalculateShippingFee failed", slog.Any("error", err))
		return nil, appErrors.ToGRPC(err)
	}

	return &deliverypb.CalculateShippingFeeResponse{
		ShippingFee:      res.ShippingFee,
		Currency:         res.Currency,
		Zone:             res.Zone,
		TotalWeightGrams: res.TotalWeightGrams,
		CalculationModel: res.CalculationModel,
		BreakdownDetails: res.BreakdownDetails,
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

	var gstinPtr *string
	if req.Gstin != nil && strings.TrimSpace(*req.Gstin) != "" {
		g := strings.ToUpper(strings.TrimSpace(*req.Gstin))
		if !gstinClientPkg.GSTINRegex.MatchString(g) {
			return nil, appErrors.BadRequest("invalid GSTIN format").ToGRPC()
		}
		gstinPtr = &g
	}

	profile := &model.DeliveryPartnerProfile{
		ID:            uuid.New(),
		PartnerID:     pUUID,
		CompanyName:   strings.TrimSpace(req.CompanyName),
		Email:         strings.TrimSpace(req.Email),
		ContactNumber: strings.TrimSpace(req.ContactNumber),
		GSTIN:         gstinPtr,
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

	profile, err := s.deliveryService.UpdatePartnerProfile(ctx, strings.TrimSpace(req.PartnerId), req.CompanyName, req.Email, req.ContactNumber, req.Gstin, req.CallerId)
	if err != nil {
		s.logger.Error("UpdatePartnerProfile failed", slog.String("partner_id", req.PartnerId), slog.Any("error", err))
		return nil, appErrors.ToGRPC(err)
	}

	return &deliverypb.PartnerProfileResponse{
		Profile: toProtoPartnerProfile(profile),
	}, nil
}

func (s *DeliveryGRPCServer) VerifyGSTIN(ctx context.Context, req *deliverypb.VerifyPartnerGSTINRequest) (*deliverypb.PartnerProfileResponse, error) {
	if req == nil {
		return nil, appErrors.BadRequest("request is required").ToGRPC()
	}
	partnerID := strings.TrimSpace(req.PartnerId)
	if partnerID == "" {
		s.logger.Warn("VerifyGSTIN: partner_id is required")
		return nil, appErrors.BadRequest("partner_id is required").ToGRPC()
	}

	profile, err := s.deliveryService.VerifyGSTIN(ctx, partnerID, strings.TrimSpace(req.Gstin), req.CallerId)
	if err != nil {
		s.logger.Error("VerifyGSTIN failed", slog.String("partner_id", req.PartnerId), slog.Any("error", err))
		return nil, appErrors.ToGRPC(err)
	}

	return &deliverypb.PartnerProfileResponse{
		Profile: toProtoPartnerProfile(profile),
	}, nil
}

func (s *DeliveryGRPCServer) ActivatePartner(ctx context.Context, req *deliverypb.ActivatePartnerRequest) (*deliverypb.PartnerProfileResponse, error) {
	if req == nil || strings.TrimSpace(req.PartnerId) == "" {
		s.logger.Warn("ActivatePartner: partner_id is required")
		return nil, appErrors.BadRequest("partner_id is required").ToGRPC()
	}

	profile, err := s.deliveryService.ActivatePartner(ctx, strings.TrimSpace(req.PartnerId), req.CallerId)
	if err != nil {
		s.logger.Error("ActivatePartner failed", slog.String("partner_id", req.PartnerId), slog.Any("error", err))
		return nil, appErrors.ToGRPC(err)
	}

	return &deliverypb.PartnerProfileResponse{
		Profile: toProtoPartnerProfile(profile),
	}, nil
}

func (s *DeliveryGRPCServer) DeactivatePartner(ctx context.Context, req *deliverypb.DeactivatePartnerRequest) (*deliverypb.PartnerProfileResponse, error) {
	if req == nil || strings.TrimSpace(req.PartnerId) == "" {
		s.logger.Warn("DeactivatePartner: partner_id is required")
		return nil, appErrors.BadRequest("partner_id is required").ToGRPC()
	}
	if strings.TrimSpace(req.Reason) == "" {
		s.logger.Warn("DeactivatePartner: reason is required")
		return nil, appErrors.BadRequest("deactivation reason is required").ToGRPC()
	}

	profile, err := s.deliveryService.DeactivatePartner(ctx, strings.TrimSpace(req.PartnerId), strings.TrimSpace(req.Reason), req.CallerId)
	if err != nil {
		s.logger.Error("DeactivatePartner failed", slog.String("partner_id", req.PartnerId), slog.Any("error", err))
		return nil, appErrors.ToGRPC(err)
	}

	return &deliverypb.PartnerProfileResponse{
		Profile: toProtoPartnerProfile(profile),
	}, nil
}

func (s *DeliveryGRPCServer) DeletePartnerProfile(ctx context.Context, req *deliverypb.DeletePartnerProfileRequest) (*deliverypb.DeletePartnerProfileResponse, error) {
	if req == nil || strings.TrimSpace(req.PartnerId) == "" {
		s.logger.Warn("DeletePartnerProfile: partner_id is required")
		return nil, appErrors.BadRequest("partner_id is required").ToGRPC()
	}

	var callerID string
	if req.CallerId != "" {
		callerID = strings.TrimSpace(req.CallerId)
	}

	reason := ""
	if req.Reason != nil {
		reason = strings.TrimSpace(*req.Reason)
	}

	err := s.deliveryService.DeletePartnerProfile(ctx, strings.TrimSpace(req.PartnerId), reason, callerID)
	if err != nil {
		s.logger.Error("DeletePartnerProfile failed", slog.String("partner_id", req.PartnerId), slog.Any("error", err))
		return nil, appErrors.ToGRPC(err)
	}

	return &deliverypb.DeletePartnerProfileResponse{
		Success: true,
		Message: "account deleted successfully",
		Status:  "deleted",
	}, nil
}

func (s *DeliveryGRPCServer) ApprovePartner(ctx context.Context, req *deliverypb.ApprovePartnerRequest) (*deliverypb.PartnerProfileResponse, error) {
	if req == nil || strings.TrimSpace(req.PartnerId) == "" {
		s.logger.Warn("ApprovePartner: partner_id is required")
		return nil, appErrors.BadRequest("partner_id is required").ToGRPC()
	}

	profile, err := s.deliveryService.ApprovePartner(ctx, strings.TrimSpace(req.PartnerId))
	if err != nil {
		s.logger.Error("ApprovePartner failed", slog.String("partner_id", req.PartnerId), slog.Any("error", err))
		return nil, appErrors.ToGRPC(err)
	}

	return &deliverypb.PartnerProfileResponse{
		Profile: toProtoPartnerProfile(profile),
	}, nil
}

func (s *DeliveryGRPCServer) RejectPartner(ctx context.Context, req *deliverypb.RejectPartnerRequest) (*deliverypb.PartnerProfileResponse, error) {
	if req == nil || strings.TrimSpace(req.PartnerId) == "" {
		s.logger.Warn("RejectPartner: partner_id is required")
		return nil, appErrors.BadRequest("partner_id is required").ToGRPC()
	}

	profile, err := s.deliveryService.RejectPartner(ctx, strings.TrimSpace(req.PartnerId), req.Reason)
	if err != nil {
		s.logger.Error("RejectPartner failed", slog.String("partner_id", req.PartnerId), slog.Any("error", err))
		return nil, appErrors.ToGRPC(err)
	}

	return &deliverypb.PartnerProfileResponse{
		Profile: toProtoPartnerProfile(profile),
	}, nil
}

func (s *DeliveryGRPCServer) AppealRejection(ctx context.Context, req *deliverypb.AppealRejectionRequest) (*deliverypb.PartnerProfileResponse, error) {
	if req == nil || strings.TrimSpace(req.PartnerId) == "" {
		s.logger.Warn("AppealRejection: partner_id is required")
		return nil, appErrors.BadRequest("partner_id is required").ToGRPC()
	}

	var callerID string
	if req.CallerId != "" {
		callerID = strings.TrimSpace(req.CallerId)
	}

	profile, err := s.deliveryService.AppealRejection(ctx, strings.TrimSpace(req.PartnerId), req.Reason, callerID)
	if err != nil {
		s.logger.Error("AppealRejection failed", slog.String("partner_id", req.PartnerId), slog.Any("error", err))
		return nil, appErrors.ToGRPC(err)
	}

	return &deliverypb.PartnerProfileResponse{
		Profile: toProtoPartnerProfile(profile),
	}, nil
}

func (s *DeliveryGRPCServer) ListAppealedPartners(ctx context.Context, req *deliverypb.ListAppealedPartnersRequest) (*deliverypb.ListAppealedPartnersResponse, error) {
	limit := 20
	offset := 0
	if req != nil {
		if req.Limit > 0 {
			limit = int(req.Limit)
		}
		if req.Offset > 0 {
			offset = int(req.Offset)
		}
	}

	profiles, total, err := s.deliveryService.ListAppealedPartners(ctx, limit, offset)
	if err != nil {
		s.logger.Error("ListAppealedPartners failed", slog.Any("error", err))
		return nil, appErrors.ToGRPC(err)
	}

	protoProfiles := make([]*deliverypb.DeliveryPartnerProfile, 0, len(profiles))
	for _, p := range profiles {
		protoProfiles = append(protoProfiles, toProtoPartnerProfile(p))
	}

	return &deliverypb.ListAppealedPartnersResponse{
		Profiles: protoProfiles,
		Total:    int32(total),
	}, nil
}

func (s *DeliveryGRPCServer) ListApprovedPartners(ctx context.Context, req *deliverypb.ListApprovedPartnersRequest) (*deliverypb.ListApprovedPartnersResponse, error) {
	limit := 20
	offset := 0
	if req != nil {
		if req.Limit > 0 {
			limit = int(req.Limit)
		}
		if req.Offset > 0 {
			offset = int(req.Offset)
		}
	}

	profiles, total, err := s.deliveryService.ListApprovedPartners(ctx, limit, offset)
	if err != nil {
		s.logger.Error("ListApprovedPartners failed", slog.Any("error", err))
		return nil, appErrors.ToGRPC(err)
	}

	protoProfiles := make([]*deliverypb.DeliveryPartnerProfile, 0, len(profiles))
	for _, p := range profiles {
		protoProfiles = append(protoProfiles, toProtoPartnerProfile(p))
	}

	return &deliverypb.ListApprovedPartnersResponse{
		Profiles: protoProfiles,
		Total:    int32(total),
	}, nil
}

func (s *DeliveryGRPCServer) ProcessExpiredDeactivations(ctx context.Context, req *deliverypb.ProcessExpiredDeactivationsRequest) (*deliverypb.ProcessExpiredDeactivationsResponse, error) {
	var retentionPeriod time.Duration
	if req != nil && req.RetentionMinutes > 0 {
		retentionPeriod = time.Duration(req.RetentionMinutes) * time.Minute
	}

	count, err := s.deliveryService.ProcessExpiredDeactivations(ctx, retentionPeriod)
	if err != nil {
		s.logger.Error("ProcessExpiredDeactivations failed", slog.Any("error", err))
		return nil, appErrors.ToGRPC(err)
	}

	return &deliverypb.ProcessExpiredDeactivationsResponse{
		ProcessedCount: int32(count),
		Message:        "Expired deactivations processed successfully",
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
		Id:                d.ID,
		OrderId:           d.OrderID,
		TrackingNumber:    d.TrackingNumber,
		DeliveryPartner:   d.DeliveryPartner,
		Status:            string(d.Status),
		ShippingAddress:   shippingAddr,
		ContactNumber:     d.ShippingAddress.ContactNumber,
		ShippingAddressId: d.ShippingAddressID,
		CreatedAt:         d.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         d.UpdatedAt.Format(time.RFC3339),
	}
	if d.EstimatedDelivery != nil {
		res.EstimatedDeliveryAt = d.EstimatedDelivery.Format(time.RFC3339)
	}
	if d.ActualDelivery != nil {
		res.DeliveredAt = d.ActualDelivery.Format(time.RFC3339)
	}
	for _, u := range d.TrackingUpdates {
		res.TrackingUpdates = append(res.TrackingUpdates, &deliverypb.DeliveryTrackingUpdate{
			Id:        u.ID,
			Status:    string(u.Status),
			Location:  u.Location,
			Notes:     u.Notes,
			CreatedAt: u.CreatedAt.Format(time.RFC3339),
		})
	}
	return res
}

func toProtoPartnerProfile(p *model.DeliveryPartnerProfile) *deliverypb.DeliveryPartnerProfile {
	if p == nil {
		return nil
	}
	status := string(p.ApprovalStatus)
	if status == "" {
		status = string(model.ApprovalStatusPending)
	}
	res := &deliverypb.DeliveryPartnerProfile{
		Id:              p.ID.String(),
		PartnerId:       p.PartnerID.String(),
		CompanyName:     p.CompanyName,
		Email:           p.Email,
		ContactNumber:   p.ContactNumber,
		IsActive:        p.IsActive,
		ApprovalStatus:  status,
		IsGstinVerified: p.IsGSTINVerified,
		CreatedAt:       p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       p.UpdatedAt.Format(time.RFC3339),
	}
	if p.GSTIN != nil {
		res.Gstin = *p.GSTIN
	}
	if p.RejectionReason != nil {
		res.RejectionReason = *p.RejectionReason
	}
	if p.AppealReason != nil {
		res.AppealReason = *p.AppealReason
	}
	if p.DeactivationReason != nil {
		res.DeactivationReason = *p.DeactivationReason
	}
	if p.DeletionReason != nil {
		res.DeletionReason = p.DeletionReason
	}
	if p.AppealedAt != nil {
		res.AppealedAt = p.AppealedAt.Format(time.RFC3339)
	}
	if p.DeactivatedAt != nil {
		res.DeactivatedAt = p.DeactivatedAt.Format(time.RFC3339)
	}
	if p.DeletedAt != nil {
		res.DeletedAt = p.DeletedAt.Format(time.RFC3339)
	}
	return res
}


