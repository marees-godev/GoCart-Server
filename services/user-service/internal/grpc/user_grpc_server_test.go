package grpc_test

import (
	"context"
	"testing"

	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
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
