package tests

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	userGRPC "github.com/marees-godev/GoCart-Server/services/user-service/internal/grpc"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type inMemoryUserRepo struct {
	users map[string]*model.User
}

func newInMemoryUserRepo() *inMemoryUserRepo {
	return &inMemoryUserRepo{
		users: make(map[string]*model.User),
	}
}

func (r *inMemoryUserRepo) CreateUser(ctx context.Context, u *model.User) error {
	r.users[u.ID] = u
	return nil
}

func (r *inMemoryUserRepo) GetByID(ctx context.Context, id string) (*model.User, error) {
	u, ok := r.users[id]
	if !ok {
		return nil, appErrors.NotFound("user not found")
	}
	cp := *u
	return &cp, nil
}

func (r *inMemoryUserRepo) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	for _, u := range r.users {
		if strings.EqualFold(u.Email, email) {
			cp := *u
			return &cp, nil
		}
	}
	return nil, appErrors.NotFound("user not found")
}

func (r *inMemoryUserRepo) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	for _, u := range r.users {
		if u.Username != nil && strings.EqualFold(*u.Username, username) {
			cp := *u
			return &cp, nil
		}
	}
	return nil, appErrors.NotFound("user not found")
}

func (r *inMemoryUserRepo) UpdateUser(ctx context.Context, u *model.User) error {
	existing, ok := r.users[u.ID]
	if !ok {
		return appErrors.NotFound("user not found")
	}
	existing.Username = u.Username
	existing.Email = u.Email
	existing.FirstName = u.FirstName
	existing.LastName = u.LastName
	existing.PhoneNumber = u.PhoneNumber
	existing.AlternatePhone = u.AlternatePhone
	existing.DateOfBirth = u.DateOfBirth
	existing.Gender = u.Gender
	existing.Bio = u.Bio
	existing.AvatarURL = u.AvatarURL
	existing.UpdatedAt = time.Now()
	return nil
}

func (r *inMemoryUserRepo) DeactivateUser(ctx context.Context, userID, performedBy string, reason *string) error {
	u, ok := r.users[userID]
	if !ok {
		return appErrors.NotFound("user not found")
	}
	u.Status = "deactivated"
	return nil
}

func (r *inMemoryUserRepo) ReactivateUser(ctx context.Context, userID, performedBy string) error {
	u, ok := r.users[userID]
	if !ok {
		return appErrors.NotFound("user not found")
	}
	u.Status = "active"
	return nil
}

func (r *inMemoryUserRepo) DeleteUser(ctx context.Context, userID, performedBy string, reason *string) error {
	u, ok := r.users[userID]
	if !ok {
		return appErrors.NotFound("user not found")
	}
	u.Status = "deleted"
	return nil
}

func (r *inMemoryUserRepo) DeleteExpiredDeactivatedUser(ctx context.Context, userID string, cutoff time.Time, performedBy string, reason *string) (bool, error) {
	return false, nil
}

func (r *inMemoryUserRepo) GetExpiredDeactivatedUserIDs(ctx context.Context, cutoff time.Time, limit int) ([]string, error) {
	return nil, nil
}

func (r *inMemoryUserRepo) GetUserAuditLogs(ctx context.Context, userID string) ([]*model.UserAuditLog, error) {
	return nil, nil
}

type testUploader struct {
	failUpload   bool
	uploadedKeys []string
	uploadedData map[string][]byte
}

func newTestUploader() *testUploader {
	return &testUploader{
		uploadedKeys: make([]string, 0),
		uploadedData: make(map[string][]byte),
	}
}

func (u *testUploader) Upload(ctx context.Context, key string, body io.Reader, contentType string) (string, error) {
	data, _ := io.ReadAll(body)
	return u.UploadBytes(ctx, key, data, contentType)
}

func (u *testUploader) UploadBytes(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	if u.failUpload {
		return "", errors.New("simulated storage failure")
	}
	u.uploadedKeys = append(u.uploadedKeys, key)
	u.uploadedData[key] = data
	return "https://cdn.gocart.internal/" + key, nil
}

func (u *testUploader) Delete(ctx context.Context, key string) error {
	return nil
}

func (u *testUploader) GetPublicURL(key string) string {
	return "https://cdn.gocart.internal/" + key
}

func (u *testUploader) GenerateKey(prefix, filename string) string {
	return prefix + "/test-" + filename
}

func (u *testUploader) GetPresignedPutURL(ctx context.Context, key string, contentType string, expiry time.Duration) (string, error) {
	return "", nil
}

func createSamplePNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 30, G: 120, B: 240, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

type testIntegrationFixture struct {
	client   userpb.UserServiceClient
	repo     *inMemoryUserRepo
	uploader *testUploader
	cleanup  func()
}

func setupIntegrationServer(t *testing.T) *testIntegrationFixture {
	lis := bufconn.Listen(10 * 1024 * 1024)

	repo := newInMemoryUserRepo()
	uploader := newTestUploader()
	imgProc := service.NewImageProcessor(uploader)
	userSvc := service.NewUserService(repo, imgProc)
	grpcHandler := userGRPC.NewUserGRPCServer(userSvc, nil)

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(grpcclient.UnaryServerInterceptor()),
		grpc.MaxRecvMsgSize(10*1024*1024),
		grpc.MaxSendMsgSize(10*1024*1024),
	)
	userpb.RegisterUserServiceServer(grpcServer, grpcHandler)

	go func() {
		_ = grpcServer.Serve(lis)
	}()

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(10*1024*1024),
			grpc.MaxCallSendMsgSize(10*1024*1024),
		),
	)
	if err != nil {
		t.Fatalf("failed to dial bufnet: %v", err)
	}

	client := userpb.NewUserServiceClient(conn)

	cleanup := func() {
		_ = conn.Close()
		grpcServer.Stop()
		_ = lis.Close()
	}

	return &testIntegrationFixture{
		client:   client,
		repo:     repo,
		uploader: uploader,
		cleanup:  cleanup,
	}
}

func TestIntegration_UpdateUserAvatar_FullSuccessFlow(t *testing.T) {
	f := setupIntegrationServer(t)
	defer f.cleanup()

	ctx := context.Background()

	// Seed user
	initialAvatar := "https://example.com/initial.jpg"
	f.repo.users["usr_test_1"] = &model.User{
		ID:        "usr_test_1",
		Email:     "usr1@example.com",
		FirstName: "John",
		LastName:  "Doe",
		Status:    "active",
		AvatarURL: &initialAvatar,
	}

	pngData := createSamplePNG(200, 200)
	ct := "image/png"
	fn := "avatar_pic.png"

	req := &userpb.UpdateUserRequest{
		Id:                "usr_test_1",
		AvatarImage:       pngData,
		AvatarContentType: &ct,
		AvatarFilename:    &fn,
	}

	res, err := f.client.UpdateUser(ctx, req)
	if err != nil {
		t.Fatalf("expected UpdateUser to succeed, got %v", err)
	}

	expectedPrefix := "https://cdn.gocart.internal/avatars/usr_test_1/"
	if res.User.AvatarUrl == nil || !strings.HasPrefix(*res.User.AvatarUrl, expectedPrefix) {
		t.Errorf("expected response avatar url with prefix %s, got %v", expectedPrefix, res.User.AvatarUrl)
	}

	// Verify storage uploader received and stored the image
	if len(f.uploader.uploadedKeys) != 1 {
		t.Fatalf("expected 1 uploaded key, got %d", len(f.uploader.uploadedKeys))
	}

	// Verify repository has the new avatar URL persisted
	repoUser := f.repo.users["usr_test_1"]
	if repoUser.AvatarURL == nil || *repoUser.AvatarURL != *res.User.AvatarUrl {
		t.Errorf("expected repo avatar URL to match response URL %v, got %v", res.User.AvatarUrl, repoUser.AvatarURL)
	}
}

func TestIntegration_UpdateUserAvatar_DataURIFlow(t *testing.T) {
	f := setupIntegrationServer(t)
	defer f.cleanup()

	ctx := context.Background()

	f.repo.users["usr_test_2"] = &model.User{
		ID:        "usr_test_2",
		Email:     "usr2@example.com",
		FirstName: "Jane",
		LastName:  "Doe",
		Status:    "active",
	}

	pngData := createSamplePNG(100, 100)
	b64 := base64.StdEncoding.EncodeToString(pngData)
	dataURI := "data:image/png;base64," + b64

	req := &userpb.UpdateUserRequest{
		Id:        "usr_test_2",
		AvatarUrl: &dataURI,
	}

	res, err := f.client.UpdateUser(ctx, req)
	if err != nil {
		t.Fatalf("expected UpdateUser with data URI to succeed, got %v", err)
	}

	expectedPrefix := "https://cdn.gocart.internal/avatars/usr_test_2/"
	if res.User.AvatarUrl == nil || !strings.HasPrefix(*res.User.AvatarUrl, expectedPrefix) {
		t.Errorf("expected response avatar url with prefix %s, got %v", expectedPrefix, res.User.AvatarUrl)
	}

	repoUser := f.repo.users["usr_test_2"]
	if repoUser.AvatarURL == nil || *repoUser.AvatarURL != *res.User.AvatarUrl {
		t.Errorf("expected repo avatar to match response URL")
	}
}

func TestIntegration_UpdateUserAvatar_InvalidFileType_Rollback(t *testing.T) {
	f := setupIntegrationServer(t)
	defer f.cleanup()

	ctx := context.Background()

	initialAvatar := "https://example.com/keep_me.jpg"
	f.repo.users["usr_test_3"] = &model.User{
		ID:        "usr_test_3",
		Email:     "usr3@example.com",
		FirstName: "Sam",
		LastName:  "Smith",
		Status:    "active",
		AvatarURL: &initialAvatar,
	}

	req := &userpb.UpdateUserRequest{
		Id:          "usr_test_3",
		AvatarImage: []byte("This is plain text not an image"),
	}

	_, err := f.client.UpdateUser(ctx, req)
	if err == nil {
		t.Fatal("expected error for invalid file type, got nil")
	}

	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument code, got %v", status.Code(err))
	}

	// Verify database was NOT updated
	if *f.repo.users["usr_test_3"].AvatarURL != initialAvatar {
		t.Errorf("expected repo avatar URL to remain unchanged, got %s", *f.repo.users["usr_test_3"].AvatarURL)
	}
}

func TestIntegration_UpdateUserAvatar_CorruptedImage_Rollback(t *testing.T) {
	f := setupIntegrationServer(t)
	defer f.cleanup()

	ctx := context.Background()

	initialAvatar := "https://example.com/keep_me.jpg"
	f.repo.users["usr_test_4"] = &model.User{
		ID:        "usr_test_4",
		Email:     "usr4@example.com",
		FirstName: "Chris",
		LastName:  "Paul",
		Status:    "active",
		AvatarURL: &initialAvatar,
	}

	corruptBytes := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0xDE, 0xAD, 0xBE, 0xEF}

	req := &userpb.UpdateUserRequest{
		Id:          "usr_test_4",
		AvatarImage: corruptBytes,
	}

	_, err := f.client.UpdateUser(ctx, req)
	if err == nil {
		t.Fatal("expected error for corrupted image, got nil")
	}

	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument code, got %v", status.Code(err))
	}

	// Verify database was NOT updated
	if *f.repo.users["usr_test_4"].AvatarURL != initialAvatar {
		t.Errorf("expected repo avatar URL to remain unchanged")
	}
}

func TestIntegration_UpdateUserAvatar_FileTooLarge_Rollback(t *testing.T) {
	f := setupIntegrationServer(t)
	defer f.cleanup()

	ctx := context.Background()

	initialAvatar := "https://example.com/keep_me.jpg"
	f.repo.users["usr_test_5"] = &model.User{
		ID:        "usr_test_5",
		Email:     "usr5@example.com",
		FirstName: "Emma",
		LastName:  "Watson",
		Status:    "active",
		AvatarURL: &initialAvatar,
	}

	tooLargeBytes := make([]byte, service.MaxAvatarSizeBytes+100)

	req := &userpb.UpdateUserRequest{
		Id:          "usr_test_5",
		AvatarImage: tooLargeBytes,
	}

	_, err := f.client.UpdateUser(ctx, req)
	if err == nil {
		t.Fatal("expected error for file too large, got nil")
	}

	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument code, got %v", status.Code(err))
	}

	// Verify database was NOT updated
	if *f.repo.users["usr_test_5"].AvatarURL != initialAvatar {
		t.Errorf("expected repo avatar URL to remain unchanged")
	}
}

func TestIntegration_UpdateUserAvatar_StorageFailure_Rollback(t *testing.T) {
	f := setupIntegrationServer(t)
	defer f.cleanup()

	ctx := context.Background()

	initialAvatar := "https://example.com/keep_me.jpg"
	f.repo.users["usr_test_6"] = &model.User{
		ID:        "usr_test_6",
		Email:     "usr6@example.com",
		FirstName: "Tony",
		LastName:  "Stark",
		Status:    "active",
		AvatarURL: &initialAvatar,
	}

	// Simulate storage failure
	f.uploader.failUpload = true

	req := &userpb.UpdateUserRequest{
		Id:          "usr_test_6",
		AvatarImage: createSamplePNG(100, 100),
	}

	_, err := f.client.UpdateUser(ctx, req)
	if err == nil {
		t.Fatal("expected error for storage failure, got nil")
	}

	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal code, got %v", status.Code(err))
	}

	// Verify database was NOT updated
	if *f.repo.users["usr_test_6"].AvatarURL != initialAvatar {
		t.Errorf("expected repo avatar URL to remain unchanged on storage failure")
	}
}
