package grpc

import (
	"context"

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
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &userpb.CreateUserResponse{
		User: &userpb.User{
			Id:        u.ID,
			Email:     u.Email,
			FirstName: u.FirstName,
			LastName:  u.LastName,
			CreatedAt: u.CreatedAt.String(),
		},
	}, nil
}

func (s *UserGRPCServer) GetUser(ctx context.Context, req *userpb.GetUserRequest) (*userpb.GetUserResponse, error) {
	if req == nil || req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "user id is required")
	}

	u, err := s.userService.GetUserByID(ctx, req.Id)
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	phonenumber := ""
	if u.PhoneNumber != nil {
		phonenumber = *u.PhoneNumber
	}

	return &userpb.GetUserResponse{
		User: &userpb.User{
			Id:        u.ID,
			Email:     u.Email,
			FirstName: u.FirstName,
			LastName:  u.LastName,
			Phone:     phonenumber,
			CreatedAt: u.CreatedAt.String(),
		},
	}, nil
}

func (s *UserGRPCServer) UpdateUser(ctx context.Context, req *userpb.UpdateUserRequest) (*userpb.UpdateUserResponse, error) {
	if req == nil || req.Id == "" {
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
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	phone := ""
	if u.PhoneNumber != nil {
		phone = *u.PhoneNumber
	}

	return &userpb.UpdateUserResponse{
		User: &userpb.User{
			Id:        u.ID,
			Email:     u.Email,
			FirstName: u.FirstName,
			LastName:  u.LastName,
			Phone:     phone,
			CreatedAt: u.CreatedAt.String(),
		},
	}, nil
}

func (s *UserGRPCServer) CreateUserAddress(ctx context.Context, req *userpb.CreateUserAddressRequest) (*userpb.CreateUserAddressResponse, error) {
	if req == nil {
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
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &userpb.CreateUserAddressResponse{
		Address: toProtoAddress(addr),
	}, nil
}

func (s *UserGRPCServer) ListUserAddresses(ctx context.Context, req *userpb.ListUserAddressesRequest) (*userpb.ListUserAddressesResponse, error) {
	if req == nil || req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user id is required")
	}

	addresses, err := s.addressService.ListAddresses(ctx, req.UserId)
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	protoAddresses := make([]*userpb.Address, len(addresses))
	for i, a := range addresses {
		protoAddresses[i] = toProtoAddress(a)
	}

	return &userpb.ListUserAddressesResponse{
		Addresses: protoAddresses,
	}, nil
}

func (s *UserGRPCServer) GetUserAddress(ctx context.Context, req *userpb.GetUserAddressRequest) (*userpb.GetUserAddressResponse, error) {
	if req == nil || req.AddressId == "" {
		return nil, status.Error(codes.InvalidArgument, "address id is required")
	}

	addr, err := s.addressService.GetAddress(ctx, req.UserId, req.AddressId)
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &userpb.GetUserAddressResponse{
		Address: toProtoAddress(addr),
	}, nil
}

func (s *UserGRPCServer) UpdateUserAddress(ctx context.Context, req *userpb.UpdateUserAddressRequest) (*userpb.UpdateUserAddressResponse, error) {
	if req == nil || req.AddressId == "" {
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
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &userpb.UpdateUserAddressResponse{
		Address: toProtoAddress(addr),
	}, nil
}

func (s *UserGRPCServer) DeleteUserAddress(ctx context.Context, req *userpb.DeleteUserAddressRequest) (*userpb.DeleteUserAddressResponse, error) {
	if req == nil || req.AddressId == "" {
		return nil, status.Error(codes.InvalidArgument, "address id is required")
	}

	if err := s.addressService.DeleteAddress(ctx, req.UserId, req.AddressId); err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &userpb.DeleteUserAddressResponse{
		Success: true,
	}, nil
}

func (s *UserGRPCServer) SetDefaultUserAddress(ctx context.Context, req *userpb.SetDefaultUserAddressRequest) (*userpb.SetDefaultUserAddressResponse, error) {
	if req == nil || req.AddressId == "" {
		return nil, status.Error(codes.InvalidArgument, "address id is required")
	}

	addr, err := s.addressService.SetDefaultAddress(ctx, req.UserId, req.AddressId)
	if err != nil {
		return nil, appErrors.MapAppErrorToGRPC(err)
	}

	return &userpb.SetDefaultUserAddressResponse{
		Address: toProtoAddress(addr),
	}, nil
}
