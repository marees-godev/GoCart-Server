package grpc_test

import (
	"context"
	"testing"

	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestUserGRPCServer_CreateUser(t *testing.T) {
	server, _, _ := setupGRPCTestServer()

	req := &userpb.CreateUserRequest{
		Id:        "usr_12345",
		Email:     "john.doe@example.com",
		FirstName: "John",
		LastName:  "Doe",
	}

	resp, err := server.CreateUser(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if resp.User.Id != "usr_12345" {
		t.Errorf("expected user id 'usr_12345', got '%s'", resp.User.Id)
	}

	// Verify protobuf marshaling works without panic or abnormal buffer allocation
	data, err := proto.Marshal(resp)
	if err != nil {
		t.Fatalf("proto.Marshal failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("marshaled data is empty")
	}

	var unmarshaled userpb.CreateUserResponse
	if err := proto.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("proto.Unmarshal failed: %v", err)
	}

	if unmarshaled.User.Email != "john.doe@example.com" {
		t.Errorf("expected email 'john.doe@example.com', got '%s'", unmarshaled.User.Email)
	}
}

func TestUserGRPCServer_DeactivateUser(t *testing.T) {
	server, _, _ := setupGRPCTestServer()

	reason := "User requested break"
	req := &userpb.DeactivateUserRequest{
		Id:     "user-1",
		Reason: &reason,
	}

	resp, err := server.DeactivateUser(context.Background(), req)
	if err != nil {
		t.Fatalf("DeactivateUser failed: %v", err)
	}

	if !resp.Success || resp.Status != "deactivated" {
		t.Errorf("expected success with deactivated status, got %+v", resp)
	}
}

func TestUserGRPCServer_DeleteUser(t *testing.T) {
	server, _, _ := setupGRPCTestServer()

	reason := "GDPR request"
	req := &userpb.DeleteUserRequest{
		Id:     "user-1",
		Reason: &reason,
	}

	resp, err := server.DeleteUser(context.Background(), req)
	if err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	if !resp.Success || resp.Status != "deleted" {
		t.Errorf("expected success with deleted status, got %+v", resp)
	}
}

func TestUserGRPCServer_InvalidUserID_ReturnsInvalidArgument(t *testing.T) {
	server, _, _ := setupGRPCTestServer()
	ctx := context.Background()

	invalidIDs := []string{"", "invalid id with spaces", "null", "undefined", "bad@id!"}

	for _, id := range invalidIDs {
		_, err := server.GetUser(ctx, &userpb.GetUserRequest{Id: id})
		if err == nil {
			t.Errorf("expected error for invalid id %q in GetUser, got nil", id)
		} else if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument code for id %q in GetUser, got %v", id, status.Code(err))
		}

		_, err = server.UpdateUser(ctx, &userpb.UpdateUserRequest{Id: id})
		if err == nil {
			t.Errorf("expected error for invalid id %q in UpdateUser, got nil", id)
		} else if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument code for id %q in UpdateUser, got %v", id, status.Code(err))
		}

		_, err = server.DeactivateUser(ctx, &userpb.DeactivateUserRequest{Id: id})
		if err == nil {
			t.Errorf("expected error for invalid id %q in DeactivateUser, got nil", id)
		} else if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument code for id %q in DeactivateUser, got %v", id, status.Code(err))
		}

		_, err = server.DeleteUser(ctx, &userpb.DeleteUserRequest{Id: id})
		if err == nil {
			t.Errorf("expected error for invalid id %q in DeleteUser, got nil", id)
		} else if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument code for id %q in DeleteUser, got %v", id, status.Code(err))
		}

		_, err = server.ReactivateUser(ctx, &userpb.ReactivateUserRequest{Id: id})
		if err == nil {
			t.Errorf("expected error for invalid id %q in ReactivateUser, got nil", id)
		} else if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument code for id %q in ReactivateUser, got %v", id, status.Code(err))
		}

		_, err = server.CreateUserAddress(ctx, &userpb.CreateUserAddressRequest{UserId: id})
		if err == nil {
			t.Errorf("expected error for invalid id %q in CreateUserAddress, got nil", id)
		} else if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument code for id %q in CreateUserAddress, got %v", id, status.Code(err))
		}
	}
}

