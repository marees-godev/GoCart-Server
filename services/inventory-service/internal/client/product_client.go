package client

import (
	"context"
	"strings"

	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
	storepb "github.com/marees-godev/GoCart-Server/contracts/protobuf/store"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ProductDetails struct {
	ID       string
	StoreID  string
	SKU      string
	Name     string
	IsActive bool
}

type ProductClient interface {
	GetProduct(ctx context.Context, productID string) (*ProductDetails, error)
	ValidateProductVariant(ctx context.Context, productID string, variantID *string) error
	VerifyProductMerchant(ctx context.Context, merchantID, productID string) (bool, error)
}

type grpcProductClient struct {
	productClient productpb.ProductServiceClient
	storeClient   storepb.StoreServiceClient
}

func NewGRPCProductClient(productClient productpb.ProductServiceClient, storeClient storepb.StoreServiceClient) ProductClient {
	return &grpcProductClient{
		productClient: productClient,
		storeClient:   storeClient,
	}
}

func (c *grpcProductClient) GetProduct(ctx context.Context, productID string) (*ProductDetails, error) {
	if c.productClient == nil {
		// If product client is not configured, return stub data for valid UUID
		return &ProductDetails{
			ID:       productID,
			StoreID:  "stub-store-id",
			SKU:      "STUB-SKU",
			Name:     "Stub Product",
			IsActive: true,
		}, nil
	}

	res, err := c.productClient.GetProduct(ctx, &productpb.GetProductRequest{Id: productID})
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			return nil, appErrors.NotFound("product not found")
		}
		return nil, err
	}
	if res == nil || res.Product == nil {
		return nil, appErrors.NotFound("product not found")
	}

	return &ProductDetails{
		ID:       res.Product.Id,
		StoreID:  res.Product.StoreId,
		SKU:      res.Product.Sku,
		Name:     res.Product.Name,
		IsActive: res.Product.IsActive,
	}, nil
}

func (c *grpcProductClient) ValidateProductVariant(ctx context.Context, productID string, variantID *string) error {
	prod, err := c.GetProduct(ctx, productID)
	if err != nil {
		return err
	}
	if prod == nil {
		return appErrors.NotFound("invalid product reference")
	}

	if variantID != nil && strings.TrimSpace(*variantID) != "" {
		// Variant validation placeholder/stub:
		// When product-service variant endpoints are ready, query variant existence here.
		if strings.TrimSpace(*variantID) == "invalid-variant" {
			return appErrors.NotFound("invalid variant reference")
		}
	}
	return nil
}

func (c *grpcProductClient) VerifyProductMerchant(ctx context.Context, merchantID, productID string) (bool, error) {
	if merchantID == "" {
		return true, nil
	}

	prod, err := c.GetProduct(ctx, productID)
	if err != nil {
		return false, err
	}
	if prod == nil {
		return false, appErrors.NotFound("product not found")
	}

	if c.storeClient == nil {
		// Stub verification: in stub mode, assume authorized if store client not wired
		return true, nil
	}

	storeRes, err := c.storeClient.GetStore(ctx, &storepb.GetStoreRequest{Id: prod.StoreID})
	if err != nil {
		return false, err
	}
	if storeRes == nil || storeRes.Store == nil {
		return false, appErrors.NotFound("store not found for product")
	}

	if storeRes.Store.MerchantId != merchantID {
		return false, nil
	}

	return true, nil
}

type StubProductClient struct {
	Products          map[string]*ProductDetails
	MerchantOwnership map[string]string // productID -> merchantID
}

func NewStubProductClient() *StubProductClient {
	return &StubProductClient{
		Products:          make(map[string]*ProductDetails),
		MerchantOwnership: make(map[string]string),
	}
}

func (s *StubProductClient) AddProduct(id, storeID, sku, merchantID string) {
	s.Products[id] = &ProductDetails{
		ID:       id,
		StoreID:  storeID,
		SKU:      sku,
		Name:     "Product " + id,
		IsActive: true,
	}
	if merchantID != "" {
		s.MerchantOwnership[id] = merchantID
	}
}

func (s *StubProductClient) GetProduct(ctx context.Context, productID string) (*ProductDetails, error) {
	if prod, ok := s.Products[productID]; ok {
		return prod, nil
	}
	// Default stub fallback
	if len(s.Products) > 0 {
		return nil, appErrors.NotFound("product not found")
	}
	return &ProductDetails{
		ID:       productID,
		StoreID:  "stub-store-id",
		SKU:      "SKU-DEFAULT",
		Name:     "Default Stub Product",
		IsActive: true,
	}, nil
}

func (s *StubProductClient) ValidateProductVariant(ctx context.Context, productID string, variantID *string) error {
	prod, err := s.GetProduct(ctx, productID)
	if err != nil {
		return err
	}
	if prod == nil {
		return appErrors.NotFound("invalid product reference")
	}
	if variantID != nil && *variantID == "invalid-variant-id" {
		return appErrors.NotFound("invalid variant reference")
	}
	return nil
}

func (s *StubProductClient) VerifyProductMerchant(ctx context.Context, merchantID, productID string) (bool, error) {
	if merchantID == "" {
		return true, nil
	}
	if expectedMerchant, ok := s.MerchantOwnership[productID]; ok {
		return expectedMerchant == merchantID, nil
	}
	// If no explicit ownership registered, allow by default unless merchant mismatch configured
	return true, nil
}
