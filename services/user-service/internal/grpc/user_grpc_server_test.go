package grpc_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"

	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	userGRPC "github.com/marees-godev/GoCart-Server/services/user-service/internal/grpc"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/service"
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

type mockGRPCImageProcessor struct {
	processFunc func(ctx context.Context, userID string, input service.AvatarInput) (string, error)
}

func (m *mockGRPCImageProcessor) ProcessAndUploadAvatar(ctx context.Context, userID string, input service.AvatarInput) (string, error) {
	if m.processFunc != nil {
		return m.processFunc(ctx, userID, input)
	}
	return "https://storage.example.com/avatars/" + userID + "/avatar.png", nil
}

func setupGRPCTestServerWithProcessor(imgProc service.ImageProcessor) (*userGRPC.UserGRPCServer, *mockUserRepository) {
	userRepo := newMockUserRepo()
	addressRepo := newMockAddressRepo()

	userRepo.users["user-1"] = &model.User{
		ID:        "user-1",
		Email:     "user1@example.com",
		FirstName: "Alex",
		LastName:  "Morgan",
		Status:    "active",
	}

	addressSvc := service.NewAddressService(addressRepo, userRepo)
	userSvc := service.NewUserService(userRepo, imgProc)
	grpcServer := userGRPC.NewUserGRPCServer(userSvc, addressSvc)

	return grpcServer, userRepo
}

func createTestPNGBytes() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 50, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 50; x++ {
			img.Set(x, y, color.RGBA{R: 100, G: 200, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestUserGRPCServer_UpdateUser_AvatarAttachment_Success(t *testing.T) {
	imgProc := &mockGRPCImageProcessor{}
	server, repo := setupGRPCTestServerWithProcessor(imgProc)

	pngBytes := createTestPNGBytes()
	ct := "image/png"
	fn := "avatar.png"

	req := &userpb.UpdateUserRequest{
		Id:                "user-1",
		AvatarImage:       pngBytes,
		AvatarContentType: &ct,
		AvatarFilename:    &fn,
	}

	resp, err := server.UpdateUser(context.Background(), req)
	if err != nil {
		t.Fatalf("expected UpdateUser to succeed, got %v", err)
	}

	expectedURL := "https://storage.example.com/avatars/user-1/avatar.png"
	if resp.User.AvatarUrl == nil || *resp.User.AvatarUrl != expectedURL {
		t.Errorf("expected avatar url %s, got %v", expectedURL, resp.User.AvatarUrl)
	}

	if repo.users["user-1"].AvatarURL == nil || *repo.users["user-1"].AvatarURL != expectedURL {
		t.Errorf("expected database avatar url to be %s", expectedURL)
	}
}

func TestUserGRPCServer_UpdateUser_InvalidFileType_ReturnsInvalidArgument(t *testing.T) {
	imgProc := &mockGRPCImageProcessor{
		processFunc: func(ctx context.Context, userID string, input service.AvatarInput) (string, error) {
			return "", appErrors.BadRequest("invalid file type: only JPEG, PNG, and WebP images are allowed")
		},
	}
	server, _ := setupGRPCTestServerWithProcessor(imgProc)

	req := &userpb.UpdateUserRequest{
		Id:          "user-1",
		AvatarImage: []byte("not an image file"),
	}

	_, err := server.UpdateUser(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument code, got %v", status.Code(err))
	}
}

func TestUserGRPCServer_UpdateUser_FileTooLarge_ReturnsInvalidArgument(t *testing.T) {
	imgProc := &mockGRPCImageProcessor{
		processFunc: func(ctx context.Context, userID string, input service.AvatarInput) (string, error) {
			return "", appErrors.BadRequest("file too large: avatar size must not exceed 5MB")
		},
	}
	server, _ := setupGRPCTestServerWithProcessor(imgProc)

	req := &userpb.UpdateUserRequest{
		Id:          "user-1",
		AvatarImage: make([]byte, 6*1024*1024),
	}

	_, err := server.UpdateUser(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument code, got %v", status.Code(err))
	}
}

func TestUserGRPCServer_UpdateUser_CorruptedImage_ReturnsInvalidArgument(t *testing.T) {
	imgProc := &mockGRPCImageProcessor{
		processFunc: func(ctx context.Context, userID string, input service.AvatarInput) (string, error) {
			return "", appErrors.BadRequest("invalid or corrupted image data")
		},
	}
	server, _ := setupGRPCTestServerWithProcessor(imgProc)

	req := &userpb.UpdateUserRequest{
		Id:          "user-1",
		AvatarImage: []byte{0x89, 0x50, 0x4E, 0x47, 0x00},
	}

	_, err := server.UpdateUser(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument code, got %v", status.Code(err))
	}
}

func TestUserGRPCServer_UpdateUser_StorageFailure_ReturnsInternal(t *testing.T) {
	imgProc := &mockGRPCImageProcessor{
		processFunc: func(ctx context.Context, userID string, input service.AvatarInput) (string, error) {
			return "", appErrors.Internal(nil, "failed to store avatar image")
		},
	}
	server, _ := setupGRPCTestServerWithProcessor(imgProc)

	req := &userpb.UpdateUserRequest{
		Id:          "user-1",
		AvatarImage: createTestPNGBytes(),
	}

	_, err := server.UpdateUser(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal code, got %v", status.Code(err))
	}
}

func TestUserGRPCServer_UpdateUser_UserNotFound_ReturnsNotFound(t *testing.T) {
	imgProc := &mockGRPCImageProcessor{}
	server, _ := setupGRPCTestServerWithProcessor(imgProc)

	req := &userpb.UpdateUserRequest{
		Id:          "non-existent-user",
		AvatarImage: createTestPNGBytes(),
	}

	_, err := server.UpdateUser(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound code, got %v", status.Code(err))
	}
}


