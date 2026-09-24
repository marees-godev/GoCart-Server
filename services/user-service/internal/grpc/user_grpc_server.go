package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type UserGRPCServer struct {
	userpb.UnimplementedUserServiceServer
	userService    service.UserService
	addressService service.AddressService
}

func NewUserGRPCServer(userService service.UserService, addressService service.AddressService) *UserGRPCServer {
	return &UserGRPCServer{
		userService:    userService,
		addressService: addressService,
	}
}

func toProtoAddress(a *model.Address) *userpb.Address {
	if a == nil {
		return nil
	}
	label := ""
	if a.Label != nil {
		label = *a.Label
	}
	fullName := ""
	if a.FullName != nil {
		fullName = *a.FullName
	}
	phoneNumber := ""
	if a.PhoneNumber != nil {
		phoneNumber = *a.PhoneNumber
	}
	emailAddress := ""
	if a.EmailAddress != nil {
		emailAddress = *a.EmailAddress
	}

	return &userpb.Address{
		Id:           a.ID,
		UserId:       a.UserID,
		Label:        label,
		FullName:     fullName,
		PhoneNumber:  phoneNumber,
		EmailAddress: emailAddress,
		AddressLine:  a.AddressLine,
		City:         a.City,
		State:        a.State,
		PostalCode:   a.PostalCode,
		Country:      a.Country,
		IsDefault:    a.IsDefault,
		CreatedAt:    a.CreatedAt.String(),
		UpdatedAt:    a.UpdatedAt.String(),
	}
}

func (s *UserGRPCServer) CreateUser(ctx context.Context, req *userpb.CreateUserRequest) (*userpb.CreateUserResponse, error) {
	if req == nil || req.GetId() == "" || req.GetEmail() == "" || req.GetFirstName() == "" || req.GetLastName() == "" {
		slog.WarnContext(ctx, "invalid arguments in gRPC CreateUser")
		return nil, status.Error(codes.InvalidArgument, "id, email, first_name, and last_name are required")
	}

	createReq := dto.CreateUserRequest{
		ID:        req.GetId(),
		Email:     req.GetEmail(),
		FirstName: req.GetFirstName(),
		LastName:  req.GetLastName(),
	}

	u, err := s.userService.CreateUser(ctx, createReq)
	if err != nil {
		slog.ErrorContext(ctx, "service error in gRPC CreateUser", "user_id", req.GetId(), "error", err)
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	slog.InfoContext(ctx, "gRPC CreateUser succeeded", "user_id", u.ID)
	return &userpb.CreateUserResponse{
		User: &userpb.User{
			Id:        u.ID,
			Email:     u.Email,
			FirstName: u.FirstName,
			LastName:  u.LastName,
			CreatedAt: u.CreatedAt.String(),
			Status:    u.Status,
		},
	}, nil
}

func (s *UserGRPCServer) GetUser(ctx context.Context, req *userpb.GetUserRequest) (*userpb.GetUserResponse, error) {
	if req == nil || req.Id == "" {
		slog.WarnContext(ctx, "missing user id in gRPC GetUser")
		return nil, status.Error(codes.InvalidArgument, "user id is required")
	}

	u, err := s.userService.GetUserByID(ctx, req.Id)
	if err != nil {
		slog.WarnContext(ctx, "service error in gRPC GetUser", "user_id", req.Id, "error", err)
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	phonenumber := ""
	if u.PhoneNumber != nil {
		phonenumber = *u.PhoneNumber
	}

	slog.InfoContext(ctx, "gRPC GetUser succeeded", "user_id", u.ID)
	return &userpb.GetUserResponse{
		User: &userpb.User{
			Id:        u.ID,
			Email:     u.Email,
			FirstName: u.FirstName,
			LastName:  u.LastName,
			Phone:     phonenumber,
			CreatedAt: u.CreatedAt.String(),
			Status:    u.Status,
		},
	}, nil
}

func (s *UserGRPCServer) UpdateUser(ctx context.Context, req *userpb.UpdateUserRequest) (*userpb.UpdateUserResponse, error) {
	if req == nil || req.Id == "" {
		slog.WarnContext(ctx, "missing user id in gRPC UpdateUser")
		return nil, status.Error(codes.InvalidArgument, "user id is required")
	}

	updateReq := dto.UpdateUserRequest{}
	if req.FirstName != "" {
		fn := req.FirstName
		updateReq.FirstName = &fn
	}
	if req.LastName != "" {
		ln := req.LastName
		updateReq.LastName = &ln
	}
	if req.Phone != "" {
		p := req.Phone
		updateReq.PhoneNumber = &p
	}

	u, err := s.userService.UpdateUser(ctx, req.Id, req.Id, updateReq)
	if err != nil {
		slog.WarnContext(ctx, "service error in gRPC UpdateUser", "user_id", req.Id, "error", err)
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	phone := ""
	if u.PhoneNumber != nil {
		phone = *u.PhoneNumber
	}

	slog.InfoContext(ctx, "gRPC UpdateUser succeeded", "user_id", u.ID)
	return &userpb.UpdateUserResponse{
		User: &userpb.User{
			Id:        u.ID,
			Email:     u.Email,
			FirstName: u.FirstName,
			LastName:  u.LastName,
			Phone:     phone,
			CreatedAt: u.CreatedAt.String(),
			Status:    u.Status,
		},
	}, nil
}

func (s *UserGRPCServer) DeactivateUser(ctx context.Context, req *userpb.DeactivateUserRequest) (*userpb.DeactivateUserResponse, error) {
	if req == nil || req.Id == "" {
		slog.WarnContext(ctx, "missing user id in gRPC DeactivateUser")
		return nil, status.Error(codes.InvalidArgument, "user id is required")
	}

	var reason *string
	if req.Reason != nil && *req.Reason != "" {
		reason = req.Reason
	}

	res, err := s.userService.DeactivateUser(ctx, req.Id, req.Id, dto.DeactivateUserRequest{Reason: reason})
	if err != nil {
		slog.WarnContext(ctx, "service error in gRPC DeactivateUser", "user_id", req.Id, "error", err)
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	slog.InfoContext(ctx, "gRPC DeactivateUser succeeded", "user_id", req.Id)
	return &userpb.DeactivateUserResponse{
		Success: res.Success,
		Message: res.Message,
		Status:  res.Status,
	}, nil
}

func (s *UserGRPCServer) DeleteUser(ctx context.Context, req *userpb.DeleteUserRequest) (*userpb.DeleteUserResponse, error) {
	if req == nil || req.Id == "" {
		slog.WarnContext(ctx, "missing user id in gRPC DeleteUser")
		return nil, status.Error(codes.InvalidArgument, "user id is required")
	}

	var reason *string
	if req.Reason != nil && *req.Reason != "" {
		reason = req.Reason
	}

	res, err := s.userService.DeleteUser(ctx, req.Id, req.Id, dto.DeleteUserRequest{Reason: reason})
	if err != nil {
		slog.WarnContext(ctx, "service error in gRPC DeleteUser", "user_id", req.Id, "error", err)
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	slog.InfoContext(ctx, "gRPC DeleteUser succeeded", "user_id", req.Id)
	return &userpb.DeleteUserResponse{
		Success: res.Success,
		Message: res.Message,
		Status:  res.Status,
	}, nil
}

func (s *UserGRPCServer) ReactivateUser(ctx context.Context, req *userpb.ReactivateUserRequest) (*userpb.ReactivateUserResponse, error) {
	if req == nil || req.Id == "" {
		slog.WarnContext(ctx, "missing user id in gRPC ReactivateUser")
		return nil, status.Error(codes.InvalidArgument, "user id is required")
	}

	res, err := s.userService.ReactivateUser(ctx, req.Id, req.Id)
	if err != nil {
		slog.WarnContext(ctx, "service error in gRPC ReactivateUser", "user_id", req.Id, "error", err)
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	slog.InfoContext(ctx, "gRPC ReactivateUser succeeded", "user_id", req.Id)
	return &userpb.ReactivateUserResponse{
		Success: res.Success,
		Message: res.Message,
		Status:  res.Status,
	}, nil
}

func (s *UserGRPCServer) ProcessExpiredDeactivations(ctx context.Context, req *userpb.ProcessExpiredDeactivationsRequest) (*userpb.ProcessExpiredDeactivationsResponse, error) {
	retentionDays := 30
	if req != nil && req.RetentionDays != nil && *req.RetentionDays > 0 {
		retentionDays = int(*req.RetentionDays)
	}

	retentionPeriod := time.Duration(retentionDays) * 24 * time.Hour
	count, err := s.userService.ProcessExpiredDeactivations(ctx, retentionPeriod)
	if err != nil {
		slog.ErrorContext(ctx, "service error in gRPC ProcessExpiredDeactivations", "error", err)
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	slog.InfoContext(ctx, "gRPC ProcessExpiredDeactivations succeeded", "processed_count", count)
	return &userpb.ProcessExpiredDeactivationsResponse{
		ProcessedCount: int32(count),
		Message:        fmt.Sprintf("successfully processed %d expired deactivated accounts", count),
	}, nil
}

func (s *UserGRPCServer) CreateUserAddress(ctx context.Context, req *userpb.CreateUserAddressRequest) (*userpb.CreateUserAddressResponse, error) {
	if req == nil {
		slog.WarnContext(ctx, "missing request payload in gRPC CreateUserAddress")
		return nil, status.Error(codes.InvalidArgument, "request payload is required")
	}

	var label *string
	if req.Label != "" {
		l := req.Label
		label = &l
	}
	var emailAddress *string
	if req.EmailAddress != "" {
		e := req.EmailAddress
		emailAddress = &e
	}

	var isDefault *bool
	if req.IsDefault {
		t := true
		isDefault = &t
	}

	createReq := dto.CreateAddressRequest{
		Label:        label,
		FullName:     req.FullName,
		PhoneNumber:  req.PhoneNumber,
		EmailAddress: emailAddress,
		AddressLine:  req.AddressLine,
		City:         req.City,
		State:        req.State,
		PostalCode:   req.PostalCode,
		Country:      req.Country,
		IsDefault:    isDefault,
	}

	addr, err := s.addressService.CreateAddress(ctx, req.UserId, createReq)
	if err != nil {
		slog.ErrorContext(ctx, "service error in gRPC CreateUserAddress", "user_id", req.UserId, "error", err)
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	slog.InfoContext(ctx, "gRPC CreateUserAddress succeeded", "user_id", req.UserId, "address_id", addr.ID)
	return &userpb.CreateUserAddressResponse{
		Address: toProtoAddress(addr),
	}, nil
}

func (s *UserGRPCServer) ListUserAddresses(ctx context.Context, req *userpb.ListUserAddressesRequest) (*userpb.ListUserAddressesResponse, error) {
	if req == nil || req.UserId == "" {
		slog.WarnContext(ctx, "missing user id in gRPC ListUserAddresses")
		return nil, status.Error(codes.InvalidArgument, "user id is required")
	}

	addresses, err := s.addressService.ListAddresses(ctx, req.UserId)
	if err != nil {
		slog.ErrorContext(ctx, "service error in gRPC ListUserAddresses", "user_id", req.UserId, "error", err)
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	protoAddresses := make([]*userpb.Address, len(addresses))
	for i, a := range addresses {
		protoAddresses[i] = toProtoAddress(a)
	}

	slog.InfoContext(ctx, "gRPC ListUserAddresses succeeded", "user_id", req.UserId, "count", len(protoAddresses))
	return &userpb.ListUserAddressesResponse{
		Addresses: protoAddresses,
	}, nil
}

func (s *UserGRPCServer) GetUserAddress(ctx context.Context, req *userpb.GetUserAddressRequest) (*userpb.GetUserAddressResponse, error) {
	if req == nil || req.AddressId == "" {
		slog.WarnContext(ctx, "missing address id in gRPC GetUserAddress")
		return nil, status.Error(codes.InvalidArgument, "address id is required")
	}

	addr, err := s.addressService.GetAddress(ctx, req.UserId, req.AddressId)
	if err != nil {
		slog.WarnContext(ctx, "service error in gRPC GetUserAddress", "user_id", req.UserId, "address_id", req.AddressId, "error", err)
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	slog.InfoContext(ctx, "gRPC GetUserAddress succeeded", "user_id", req.UserId, "address_id", addr.ID)
	return &userpb.GetUserAddressResponse{
		Address: toProtoAddress(addr),
	}, nil
}

func (s *UserGRPCServer) UpdateUserAddress(ctx context.Context, req *userpb.UpdateUserAddressRequest) (*userpb.UpdateUserAddressResponse, error) {
	if req == nil || req.AddressId == "" {
		slog.WarnContext(ctx, "missing address id in gRPC UpdateUserAddress")
		return nil, status.Error(codes.InvalidArgument, "address id is required")
	}

	updateReq := dto.UpdateAddressRequest{
		Label:        req.Label,
		FullName:     req.FullName,
		PhoneNumber:  req.PhoneNumber,
		EmailAddress: req.EmailAddress,
		AddressLine:  req.AddressLine,
		City:         req.City,
		State:        req.State,
		PostalCode:   req.PostalCode,
		Country:      req.Country,
		IsDefault:    req.IsDefault,
	}

	addr, err := s.addressService.UpdateAddress(ctx, req.UserId, req.AddressId, updateReq)
	if err != nil {
		slog.WarnContext(ctx, "service error in gRPC UpdateUserAddress", "user_id", req.UserId, "address_id", req.AddressId, "error", err)
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	slog.InfoContext(ctx, "gRPC UpdateUserAddress succeeded", "user_id", req.UserId, "address_id", addr.ID)
	return &userpb.UpdateUserAddressResponse{
		Address: toProtoAddress(addr),
	}, nil
}

func (s *UserGRPCServer) DeleteUserAddress(ctx context.Context, req *userpb.DeleteUserAddressRequest) (*userpb.DeleteUserAddressResponse, error) {
	if req == nil || req.AddressId == "" {
		slog.WarnContext(ctx, "missing address id in gRPC DeleteUserAddress")
		return nil, status.Error(codes.InvalidArgument, "address id is required")
	}

	if err := s.addressService.DeleteAddress(ctx, req.UserId, req.AddressId); err != nil {
		slog.WarnContext(ctx, "service error in gRPC DeleteUserAddress", "user_id", req.UserId, "address_id", req.AddressId, "error", err)
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	slog.InfoContext(ctx, "gRPC DeleteUserAddress succeeded", "user_id", req.UserId, "address_id", req.AddressId)
	return &userpb.DeleteUserAddressResponse{
		Success: true,
	}, nil
}

func (s *UserGRPCServer) SetDefaultUserAddress(ctx context.Context, req *userpb.SetDefaultUserAddressRequest) (*userpb.SetDefaultUserAddressResponse, error) {
	if req == nil || req.AddressId == "" {
		slog.WarnContext(ctx, "missing address id in gRPC SetDefaultUserAddress")
		return nil, status.Error(codes.InvalidArgument, "address id is required")
	}

	addr, err := s.addressService.SetDefaultAddress(ctx, req.UserId, req.AddressId)
	if err != nil {
		slog.WarnContext(ctx, "service error in gRPC SetDefaultUserAddress", "user_id", req.UserId, "address_id", req.AddressId, "error", err)
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	slog.InfoContext(ctx, "gRPC SetDefaultUserAddress succeeded", "user_id", req.UserId, "address_id", addr.ID)
	return &userpb.SetDefaultUserAddressResponse{
		Address: toProtoAddress(addr),
	}, nil
}
