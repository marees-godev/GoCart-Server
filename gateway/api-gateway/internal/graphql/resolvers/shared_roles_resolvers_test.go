package resolvers

import (
	"context"
	"testing"

	cartpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/cart"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	notificationpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/notification"
	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	grpcPkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockNotificationClientForTest struct {
	notificationpb.NotificationServiceClient
	lastGetReq  *notificationpb.GetNotificationsRequest
	lastSendReq *notificationpb.SendNotificationRequest
}

func (m *mockNotificationClientForTest) GetNotifications(ctx context.Context, in *notificationpb.GetNotificationsRequest, opts ...grpcPkg.CallOption) (*notificationpb.GetNotificationsResponse, error) {
	m.lastGetReq = in
	return &notificationpb.GetNotificationsResponse{
		Notifications: []*notificationpb.Notification{
			{
				Id:      "notif-1",
				UserId:  in.UserId,
				Channel: "EMAIL",
				Subject: "Order Update",
				Content: "Your order is confirmed",
				Status:  "SENT",
			},
		},
		Total: 1,
	}, nil
}

func (m *mockNotificationClientForTest) SendNotification(ctx context.Context, in *notificationpb.SendNotificationRequest, opts ...grpcPkg.CallOption) (*notificationpb.SendNotificationResponse, error) {
	m.lastSendReq = in
	return &notificationpb.SendNotificationResponse{
		NotificationId: "notif-created-1",
		Success:        true,
	}, nil
}

type mockUserClientForSharedTest struct {
	userpb.UserServiceClient
	users     map[string]*userpb.User
	addresses map[string][]*userpb.Address
}

func newMockUserClientForSharedTest() *mockUserClientForSharedTest {
	return &mockUserClientForSharedTest{
		users:     make(map[string]*userpb.User),
		addresses: make(map[string][]*userpb.Address),
	}
}

func (m *mockUserClientForSharedTest) GetUser(ctx context.Context, in *userpb.GetUserRequest, opts ...grpcPkg.CallOption) (*userpb.GetUserResponse, error) {
	if u, ok := m.users[in.Id]; ok {
		return &userpb.GetUserResponse{User: u}, nil
	}
	return nil, status.Error(codes.NotFound, "user not found")
}

func (m *mockUserClientForSharedTest) CreateUser(ctx context.Context, in *userpb.CreateUserRequest, opts ...grpcPkg.CallOption) (*userpb.CreateUserResponse, error) {
	u := &userpb.User{
		Id:        in.Id,
		Email:     in.Email,
		FirstName: in.FirstName,
		LastName:  in.LastName,
		Status:    "active",
	}
	m.users[in.Id] = u
	return &userpb.CreateUserResponse{User: u}, nil
}

func (m *mockUserClientForSharedTest) UpdateUser(ctx context.Context, in *userpb.UpdateUserRequest, opts ...grpcPkg.CallOption) (*userpb.UpdateUserResponse, error) {
	u, ok := m.users[in.Id]
	if !ok {
		return nil, status.Error(codes.NotFound, "user not found")
	}
	if in.FirstName != "" {
		u.FirstName = in.FirstName
	}
	if in.LastName != "" {
		u.LastName = in.LastName
	}
	return &userpb.UpdateUserResponse{User: u}, nil
}

func (m *mockUserClientForSharedTest) ListUserAddresses(ctx context.Context, in *userpb.ListUserAddressesRequest, opts ...grpcPkg.CallOption) (*userpb.ListUserAddressesResponse, error) {
	addrs := m.addresses[in.UserId]
	return &userpb.ListUserAddressesResponse{Addresses: addrs}, nil
}

func (m *mockUserClientForSharedTest) CreateUserAddress(ctx context.Context, in *userpb.CreateUserAddressRequest, opts ...grpcPkg.CallOption) (*userpb.CreateUserAddressResponse, error) {
	if _, ok := m.users[in.UserId]; !ok {
		return nil, status.Error(codes.NotFound, "user does not exist")
	}
	addr := &userpb.Address{
		Id:          "addr-1",
		UserId:      in.UserId,
		AddressLine: in.AddressLine,
		City:        in.City,
		State:       in.State,
		PostalCode:  in.PostalCode,
		Country:     in.Country,
	}
	m.addresses[in.UserId] = append(m.addresses[in.UserId], addr)
	return &userpb.CreateUserAddressResponse{Address: addr}, nil
}

func TestSharedRoles_AllRolesCanAccessMe(t *testing.T) {
	userClient := newMockUserClientForSharedTest()
	userClient.users["cust-1"] = &userpb.User{Id: "cust-1", Email: "cust@test.com", FirstName: "Customer", LastName: "User"}
	userClient.users["merch-1"] = &userpb.User{Id: "merch-1", Email: "merch@test.com", FirstName: "Merchant", LastName: "User"}
	userClient.users["dp-1"] = &userpb.User{Id: "dp-1", Email: "dp@test.com", FirstName: "Delivery", LastName: "Partner"}

	clients := grpc.NewClientsWithServices(userClient)
	resolver := NewResolver(clients)

	roles := []struct {
		name   string
		userID string
		role   string
	}{
		{name: "Customer", userID: "cust-1", role: "CUSTOMER"},
		{name: "Merchant", userID: "merch-1", role: "MERCHANT"},
		{name: "DeliveryPartner", userID: "dp-1", role: "DELIVERY_PARTNER"},
	}

	for _, tc := range roles {
		t.Run(tc.name+" can access /me successfully", func(t *testing.T) {
			ctx := auth.WithUser(context.Background(), &auth.UserContext{
				UserID: tc.userID,
				Role:   tc.role,
				Email:  tc.userID + "@test.com",
			})

			res, err := resolver.Query().Me(ctx)
			if err != nil {
				t.Fatalf("expected /me to succeed for %s, got err: %v", tc.role, err)
			}
			if res == nil || res.ID != tc.userID {
				t.Errorf("expected user ID %s, got %v", tc.userID, res)
			}
			if res.Role == nil || *res.Role != tc.role {
				t.Errorf("expected role %s, got %v", tc.role, res.Role)
			}
		})
	}
}

func TestSharedRoles_CartAccessAndCrossUserProtection(t *testing.T) {
	cartMock := &mockCartClient{
		getResp: &cartpb.GetCartResponse{
			Cart: &cartpb.Cart{
				Id:     "cart-1",
				UserId: "user-1",
			},
		},
		removeResp: &cartpb.RemoveCartItemResponse{
			Cart: &cartpb.Cart{
				Id:     "cart-1",
				UserId: "user-1",
			},
		},
		clearResp: &cartpb.ClearCartResponse{
			Success: true,
		},
	}

	clients := grpc.NewClientsWithServices(nil, cartMock)
	resolver := NewResolver(clients)

	roles := []string{"CUSTOMER", "MERCHANT", "DELIVERY_PARTNER"}
	for _, role := range roles {
		t.Run(role+" can view and manage their own cart", func(t *testing.T) {
			ctx := auth.WithUser(context.Background(), &auth.UserContext{
				UserID: "user-1",
				Role:   role,
			})

			cart, err := resolver.Query().Cart(ctx, nil)
			if err != nil {
				t.Fatalf("expected role %s to access cart, got: %v", role, err)
			}
			if cart.UserID != "user-1" {
				t.Errorf("expected cart for user-1, got %s", cart.UserID)
			}

			// ClearCart on own cart succeeds
			ok, err := resolver.Mutation().ClearCart(ctx, nil)
			if err != nil || !ok {
				t.Fatalf("expected ClearCart to succeed, got: %v", err)
			}
		})
	}

	t.Run("Cross-user cart access is strictly forbidden", func(t *testing.T) {
		attackerCtx := auth.WithUser(context.Background(), &auth.UserContext{
			UserID: "attacker-user",
			Role:   "CUSTOMER",
		})
		victimUserID := "victim-user"

		_, err := resolver.Query().Cart(attackerCtx, &victimUserID)
		if err == nil {
			t.Fatalf("expected forbidden error accessing another user's cart")
		}

		_, err = resolver.Mutation().RemoveFromCart(attackerCtx, &victimUserID, "prod-1")
		if err == nil {
			t.Fatalf("expected forbidden error removing from another user's cart")
		}

		_, err = resolver.Mutation().ClearCart(attackerCtx, &victimUserID)
		if err == nil {
			t.Fatalf("expected forbidden error clearing another user's cart")
		}
	})
}

func TestSharedRoles_AddressAccessAndSelfHealing(t *testing.T) {
	userClient := newMockUserClientForSharedTest()
	// userClient initially does NOT have merchant-legacy
	merchantMock := &mockMerchantClientForUserResolvers{
		getMerchantResp: &merchantpb.GetMerchantResponse{
			Merchant: &merchantpb.MerchantResponseData{
				Id:            "merchant-legacy",
				BusinessEmail: "legacy@merchant.com",
				FirstName:     "Legacy",
				LastName:      "Merchant",
			},
		},
	}

	clients := grpc.NewClientsWithServices(userClient, merchantMock)
	resolver := NewResolver(clients)

	t.Run("Legacy merchant without user record self-heals on CreateUserAddress", func(t *testing.T) {
		ctx := auth.WithUser(context.Background(), &auth.UserContext{
			UserID: "merchant-legacy",
			Role:   "MERCHANT",
			Email:  "legacy@merchant.com",
		})

		addr, err := resolver.Mutation().CreateUserAddress(ctx, model.CreateAddressInput{
			FullName:    "Legacy Merchant",
			PhoneNumber: "1234567890",
			AddressLine: "123 Merchant Way",
			City:        "Commerce City",
			State:       "CA",
			PostalCode:  "90210",
			Country:     "USA",
		})
		if err != nil {
			t.Fatalf("expected self-healing CreateUserAddress to succeed: %v", err)
		}
		if addr == nil || addr.City != "Commerce City" {
			t.Errorf("expected address created, got %v", addr)
		}

		// User profile was created in user service
		if _, ok := userClient.users["merchant-legacy"]; !ok {
			t.Errorf("expected user record to be self-healed in user service")
		}
	})

	t.Run("Cross-user address access is forbidden", func(t *testing.T) {
		ctx := auth.WithUser(context.Background(), &auth.UserContext{
			UserID: "user-a",
			Role:   "CUSTOMER",
		})

		_, err := resolver.Query().UserAddresses(ctx, "user-b")
		if err == nil {
			t.Fatalf("expected forbidden error querying another user's addresses")
		}
	})
}

func TestSharedRoles_NotificationAccessAndIsolation(t *testing.T) {
	notifMock := &mockNotificationClientForTest{}
	clients := grpc.NewClientsWithServices(nil, notifMock)
	resolver := NewResolver(clients)

	roles := []string{"CUSTOMER", "MERCHANT", "DELIVERY_PARTNER"}
	for _, role := range roles {
		t.Run(role+" can view notifications", func(t *testing.T) {
			ctx := auth.WithUser(context.Background(), &auth.UserContext{
				UserID: "notif-user-1",
				Role:   role,
			})

			uid := "notif-user-1"
			res, err := resolver.Query().Notifications(ctx, &uid, nil, nil, nil)
			if err != nil {
				t.Fatalf("expected role %s to view notifications, got: %v", role, err)
			}
			if res.Total != 1 || len(res.Notifications) != 1 {
				t.Errorf("expected 1 notification, got %d", res.Total)
			}
		})
	}

	t.Run("Cross-user notification access is forbidden", func(t *testing.T) {
		ctx := auth.WithUser(context.Background(), &auth.UserContext{
			UserID: "notif-user-1",
			Role:   "CUSTOMER",
		})

		otherUID := "other-user-2"
		_, err := resolver.Query().Notifications(ctx, &otherUID, nil, nil, nil)
		if err == nil {
			t.Fatalf("expected forbidden error reading another user's notifications")
		}
	})
}

func TestSharedRoles_OrderAccessAndIsolation(t *testing.T) {
	mockOrder := &mockOrderServiceClient{
		createResp: &orderpb.CreateOrderResponse{
			Order: &orderpb.Order{
				Id:          "order-new-1",
				UserId:      "user-caller-1",
				OrderStatus: "CREATED",
				TotalAmount: 100.0,
			},
		},
		listResp: &orderpb.ListOrdersResponse{
			Orders: []*orderpb.Order{
				{
					Id:          "order-exist-1",
					UserId:      "user-caller-1",
					OrderStatus: "CONFIRMED",
					TotalAmount: 50.0,
				},
			},
			Total: 1,
		},
	}
	mockDelivery := &mockDeliveryForOrderTest{}
	clients := grpc.NewClientsWithServices(nil, mockOrder, mockDelivery)
	resolver := NewResolver(clients)

	roles := []string{"CUSTOMER", "MERCHANT", "DELIVERY_PARTNER"}
	for _, role := range roles {
		t.Run(role+" can create order and list their purchases", func(t *testing.T) {
			ctx := auth.WithUser(context.Background(), &auth.UserContext{
				UserID: "user-caller-1",
				Role:   role,
			})

			// 1. Create order as shopper
			created, err := resolver.Mutation().CreateOrder(ctx, model.CreateOrderInput{
				DeliveryPartnerID: "dp-1",
			})
			if err != nil {
				t.Fatalf("expected role %s to create order, got: %v", role, err)
			}
			if created.ID != "order-new-1" {
				t.Errorf("expected order-new-1, got %s", created.ID)
			}

			// 2. List own orders
			orders, err := resolver.Query().ListMyOrders(ctx, nil, nil)
			if err != nil {
				t.Fatalf("expected role %s to list own orders, got: %v", role, err)
			}
			if orders.Total != 1 || len(orders.Orders) != 1 {
				t.Errorf("expected 1 order, got %d", orders.Total)
			}

			// 3. Query orders with own user ID
			userOrders, err := resolver.Query().Orders(ctx, "user-caller-1", nil, nil)
			if err != nil {
				t.Fatalf("expected role %s to query own orders, got: %v", role, err)
			}
			if userOrders.Total != 1 {
				t.Errorf("expected 1 order, got %d", userOrders.Total)
			}
		})
	}

	t.Run("Cross-user order operations are strictly forbidden", func(t *testing.T) {
		attackerCtx := auth.WithUser(context.Background(), &auth.UserContext{
			UserID: "attacker-user",
			Role:   "CUSTOMER",
		})
		victimID := "victim-user"

		// 1. Attempt to create order using another user's ID
		_, err := resolver.Mutation().CreateOrder(attackerCtx, model.CreateOrderInput{
			DeliveryPartnerID: "dp-1",
			UserID:            &victimID,
		})
		if err == nil {
			t.Fatalf("expected forbidden error when creating order on behalf of another user")
		}

		// 2. Attempt to query another user's orders
		_, err = resolver.Query().Orders(attackerCtx, victimID, nil, nil)
		if err == nil {
			t.Fatalf("expected forbidden error when viewing another user's orders")
		}
	})
}

func TestSharedRoles_RoleSpecificPermissionMatrix(t *testing.T) {
	// Function to simulate auth directive evaluation
	evalAuthDirective := func(userRole string, requires []model.Role) error {
		if userRole == "ADMIN" {
			return nil
		}
		if len(requires) > 0 {
			for _, reqRole := range requires {
				if userRole == string(reqRole) {
					return nil
				}
			}
			return appErrors.Forbidden("insufficient permissions for this operation")
		}
		return nil
	}

	tests := []struct {
		functionality    string
		requires         []model.Role
		allowCustomer    bool
		allowMerchant    bool
		allowDelivery    bool
	}{
		// Common / Shared:
		{functionality: "Profile management", requires: nil, allowCustomer: true, allowMerchant: true, allowDelivery: true},
		{functionality: "Browse products", requires: nil, allowCustomer: true, allowMerchant: true, allowDelivery: true},
		{functionality: "Shopping cart", requires: nil, allowCustomer: true, allowMerchant: true, allowDelivery: true},
		{functionality: "Place orders", requires: nil, allowCustomer: true, allowMerchant: true, allowDelivery: true},
		{functionality: "View own purchases", requires: nil, allowCustomer: true, allowMerchant: true, allowDelivery: true},
		// Role-Specific:
		{functionality: "Manage store", requires: []model.Role{model.RoleMerchant}, allowCustomer: false, allowMerchant: true, allowDelivery: false},
		{functionality: "Manage products", requires: []model.Role{model.RoleAdmin, model.RoleMerchant}, allowCustomer: false, allowMerchant: true, allowDelivery: false},
		{functionality: "Manage inventory", requires: []model.Role{model.RoleAdmin, model.RoleMerchant}, allowCustomer: false, allowMerchant: true, allowDelivery: false},
		{functionality: "Manage merchant orders", requires: []model.Role{model.RoleAdmin, model.RoleMerchant}, allowCustomer: false, allowMerchant: true, allowDelivery: false},
		{functionality: "Access assigned deliveries", requires: []model.Role{model.RoleDeliveryPartner, model.RoleAdmin}, allowCustomer: false, allowMerchant: false, allowDelivery: true},
		{functionality: "Update delivery status", requires: []model.Role{model.RoleDeliveryPartner}, allowCustomer: false, allowMerchant: false, allowDelivery: true},
	}

	for _, tc := range tests {
		t.Run(tc.functionality, func(t *testing.T) {
			// Check CUSTOMER / USER
			custErr := evalAuthDirective(string(model.RoleUser), tc.requires)
			if tc.allowCustomer && custErr != nil {
				t.Errorf("%s: expected CUSTOMER to be Allowed, got %v", tc.functionality, custErr)
			} else if !tc.allowCustomer && custErr == nil {
				t.Errorf("%s: expected CUSTOMER to be Denied, got nil error", tc.functionality)
			}

			// Check MERCHANT
			merchErr := evalAuthDirective(string(model.RoleMerchant), tc.requires)
			if tc.allowMerchant && merchErr != nil {
				t.Errorf("%s: expected MERCHANT to be Allowed, got %v", tc.functionality, merchErr)
			} else if !tc.allowMerchant && merchErr == nil {
				t.Errorf("%s: expected MERCHANT to be Denied, got nil error", tc.functionality)
			}

			// Check DELIVERY_PARTNER
			dpErr := evalAuthDirective(string(model.RoleDeliveryPartner), tc.requires)
			if tc.allowDelivery && dpErr != nil {
				t.Errorf("%s: expected DELIVERY_PARTNER to be Allowed, got %v", tc.functionality, dpErr)
			} else if !tc.allowDelivery && dpErr == nil {
				t.Errorf("%s: expected DELIVERY_PARTNER to be Denied, got nil error", tc.functionality)
			}
		})
	}
}

