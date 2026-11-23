package grpc

import (
	"context"
	"testing"

	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockLocationService struct {
	getFunc func(ctx context.Context, pincode, countryCode string) (*model.Location, error)
}

func (m *mockLocationService) GetLocationByPincode(ctx context.Context, pincode string, countryCode string) (*model.Location, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, pincode, countryCode)
	}
	return nil, nil
}

func TestGetLocationByPincode_Success(t *testing.T) {
	locSvc := &mockLocationService{
		getFunc: func(ctx context.Context, pincode, countryCode string) (*model.Location, error) {
			return &model.Location{
				Pincode:  "623517",
				Country:  "India",
				State:    "Tamil Nadu",
				District: "Ramanathapuram",
				PostOffices: []model.PostOffice{
					{
						Name:           "Mandapam Camp",
						BranchType:     "Sub Post Office",
						DeliveryStatus: "Delivery",
						Circle:         "Tamilnadu",
						District:       "Ramanathapuram",
						Division:       "Ramanathapuram",
						Region:         "Madurai",
						State:          "Tamil Nadu",
						Country:        "India",
						Pincode:        "623517",
					},
				},
			}, nil
		},
	}

	server := NewUserGRPCServer(nil, nil, locSvc)
	res, err := server.GetLocationByPincode(context.Background(), &userpb.GetLocationByPincodeRequest{
		Pincode:     "623517",
		CountryCode: "IN",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Location == nil {
		t.Fatal("expected location in response, got nil")
	}
	if res.Location.Pincode != "623517" {
		t.Errorf("expected pincode 623517, got %s", res.Location.Pincode)
	}
	if len(res.Location.PostOffices) != 1 {
		t.Fatalf("expected 1 post office, got %d", len(res.Location.PostOffices))
	}
	if res.Location.PostOffices[0].Name != "Mandapam Camp" {
		t.Errorf("expected post office name 'Mandapam Camp', got %s", res.Location.PostOffices[0].Name)
	}
}

func TestGetLocationByPincode_NilRequest(t *testing.T) {
	server := NewUserGRPCServer(nil, nil, &mockLocationService{})
	_, err := server.GetLocationByPincode(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil request, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument code, got %v", err)
	}
}

func TestGetLocationByPincode_NotFound(t *testing.T) {
	locSvc := &mockLocationService{
		getFunc: func(ctx context.Context, pincode, countryCode string) (*model.Location, error) {
			return nil, appErrors.NotFound("not found")
		},
	}

	server := NewUserGRPCServer(nil, nil, locSvc)
	_, err := server.GetLocationByPincode(context.Background(), &userpb.GetLocationByPincodeRequest{
		Pincode: "999999",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.NotFound {
		t.Errorf("expected NotFound code, got %v", err)
	}
}

func TestGetLocationByPincode_Unimplemented(t *testing.T) {
	server := NewUserGRPCServer(nil, nil)
	_, err := server.GetLocationByPincode(context.Background(), &userpb.GetLocationByPincodeRequest{
		Pincode: "623517",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Unimplemented {
		t.Errorf("expected Unimplemented code, got %v", err)
	}
}
