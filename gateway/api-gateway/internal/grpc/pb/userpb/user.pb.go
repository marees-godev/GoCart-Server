package userpb

import (
	"context"

	"google.golang.org/grpc"
)

type User struct {
	Id        string `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	CreatedAt string `json:"created_at"`
}

type Address struct {
	Id           string `json:"id"`
	UserId       string `json:"user_id"`
	Label        string `json:"label,omitempty"`
	FullName     string `json:"full_name,omitempty"`
	PhoneNumber  string `json:"phone_number,omitempty"`
	EmailAddress string `json:"email_address,omitempty"`
	AddressLine  string `json:"address_line"`
	City         string `json:"city"`
	State        string `json:"state"`
	PostalCode   string `json:"postal_code"`
	Country      string `json:"country"`
	IsDefault    bool   `json:"is_default"`
	CreatedAt    string `json:"created_at,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

type GetUserRequest struct {
	Id string `json:"id"`
}

type GetUserResponse struct {
	User *User `json:"user"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RegisterRequest struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  *User  `json:"user"`
}

type CreateUserAddressRequest struct {
	UserId       string `json:"user_id"`
	Label        string `json:"label,omitempty"`
	FullName     string `json:"full_name"`
	PhoneNumber  string `json:"phone_number"`
	EmailAddress string `json:"email_address,omitempty"`
	AddressLine  string `json:"address_line"`
	City         string `json:"city"`
	State        string `json:"state"`
	PostalCode   string `json:"postal_code"`
	Country      string `json:"country"`
	IsDefault    bool   `json:"is_default"`
}

type CreateUserAddressResponse struct {
	Address *Address `json:"address"`
}

type ListUserAddressesRequest struct {
	UserId string `json:"user_id"`
}

type ListUserAddressesResponse struct {
	Addresses []*Address `json:"addresses"`
}

type GetUserAddressRequest struct {
	UserId    string `json:"user_id"`
	AddressId string `json:"address_id"`
}

type GetUserAddressResponse struct {
	Address *Address `json:"address"`
}

type UpdateUserAddressRequest struct {
	UserId       string  `json:"user_id"`
	AddressId    string  `json:"address_id"`
	Label        *string `json:"label,omitempty"`
	FullName     *string `json:"full_name,omitempty"`
	PhoneNumber  *string `json:"phone_number,omitempty"`
	EmailAddress *string `json:"email_address,omitempty"`
	AddressLine  *string `json:"address_line,omitempty"`
	City         *string `json:"city,omitempty"`
	State        *string `json:"state,omitempty"`
	PostalCode   *string `json:"postal_code,omitempty"`
	Country      *string `json:"country,omitempty"`
	IsDefault    *bool   `json:"is_default,omitempty"`
}

type UpdateUserAddressResponse struct {
	Address *Address `json:"address"`
}

type DeleteUserAddressRequest struct {
	UserId    string `json:"user_id"`
	AddressId string `json:"address_id"`
}

type DeleteUserAddressResponse struct {
	Success bool `json:"success"`
}

type SetDefaultUserAddressRequest struct {
	UserId    string `json:"user_id"`
	AddressId string `json:"address_id"`
}

type SetDefaultUserAddressResponse struct {
	Address *Address `json:"address"`
}

type UserServiceClient interface {
	GetUser(ctx context.Context, in *GetUserRequest, opts ...grpc.CallOption) (*GetUserResponse, error)
	Login(ctx context.Context, in *LoginRequest, opts ...grpc.CallOption) (*AuthResponse, error)
	Register(ctx context.Context, in *RegisterRequest, opts ...grpc.CallOption) (*AuthResponse, error)
	CreateUserAddress(ctx context.Context, in *CreateUserAddressRequest, opts ...grpc.CallOption) (*CreateUserAddressResponse, error)
	ListUserAddresses(ctx context.Context, in *ListUserAddressesRequest, opts ...grpc.CallOption) (*ListUserAddressesResponse, error)
	GetUserAddress(ctx context.Context, in *GetUserAddressRequest, opts ...grpc.CallOption) (*GetUserAddressResponse, error)
	UpdateUserAddress(ctx context.Context, in *UpdateUserAddressRequest, opts ...grpc.CallOption) (*UpdateUserAddressResponse, error)
	DeleteUserAddress(ctx context.Context, in *DeleteUserAddressRequest, opts ...grpc.CallOption) (*DeleteUserAddressResponse, error)
	SetDefaultUserAddress(ctx context.Context, in *SetDefaultUserAddressRequest, opts ...grpc.CallOption) (*SetDefaultUserAddressResponse, error)
}

type userServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewUserServiceClient(cc grpc.ClientConnInterface) UserServiceClient {
	return &userServiceClient{cc: cc}
}

func (c *userServiceClient) GetUser(ctx context.Context, in *GetUserRequest, opts ...grpc.CallOption) (*GetUserResponse, error) {
	out := new(GetUserResponse)
	err := c.cc.Invoke(ctx, "/gocart.user.v1.UserService/GetUser", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *userServiceClient) Login(ctx context.Context, in *LoginRequest, opts ...grpc.CallOption) (*AuthResponse, error) {
	out := new(AuthResponse)
	err := c.cc.Invoke(ctx, "/gocart.user.v1.UserService/Login", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *userServiceClient) Register(ctx context.Context, in *RegisterRequest, opts ...grpc.CallOption) (*AuthResponse, error) {
	out := new(AuthResponse)
	err := c.cc.Invoke(ctx, "/gocart.user.v1.UserService/Register", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *userServiceClient) CreateUserAddress(ctx context.Context, in *CreateUserAddressRequest, opts ...grpc.CallOption) (*CreateUserAddressResponse, error) {
	out := new(CreateUserAddressResponse)
	err := c.cc.Invoke(ctx, "/gocart.user.v1.UserService/CreateUserAddress", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *userServiceClient) ListUserAddresses(ctx context.Context, in *ListUserAddressesRequest, opts ...grpc.CallOption) (*ListUserAddressesResponse, error) {
	out := new(ListUserAddressesResponse)
	err := c.cc.Invoke(ctx, "/gocart.user.v1.UserService/ListUserAddresses", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *userServiceClient) GetUserAddress(ctx context.Context, in *GetUserAddressRequest, opts ...grpc.CallOption) (*GetUserAddressResponse, error) {
	out := new(GetUserAddressResponse)
	err := c.cc.Invoke(ctx, "/gocart.user.v1.UserService/GetUserAddress", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *userServiceClient) UpdateUserAddress(ctx context.Context, in *UpdateUserAddressRequest, opts ...grpc.CallOption) (*UpdateUserAddressResponse, error) {
	out := new(UpdateUserAddressResponse)
	err := c.cc.Invoke(ctx, "/gocart.user.v1.UserService/UpdateUserAddress", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *userServiceClient) DeleteUserAddress(ctx context.Context, in *DeleteUserAddressRequest, opts ...grpc.CallOption) (*DeleteUserAddressResponse, error) {
	out := new(DeleteUserAddressResponse)
	err := c.cc.Invoke(ctx, "/gocart.user.v1.UserService/DeleteUserAddress", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *userServiceClient) SetDefaultUserAddress(ctx context.Context, in *SetDefaultUserAddressRequest, opts ...grpc.CallOption) (*SetDefaultUserAddressResponse, error) {
	out := new(SetDefaultUserAddressResponse)
	err := c.cc.Invoke(ctx, "/gocart.user.v1.UserService/SetDefaultUserAddress", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

