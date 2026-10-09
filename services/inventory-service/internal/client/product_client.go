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
		return nil, appErrors.Internal(nil, "product service client unavailable")
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
		IsActive: res.Product.Status != "discontinued" && res.Product.Status != "inactive",
	}, nil
}

func (c *grpcProductClient) ValidateProductVariant(ctx context.Context, productID string, variantID *string) error {
	if c.productClient == nil {
		return appErrors.Internal(nil, "product service client unavailable")
	}

	res, err := c.productClient.GetProduct(ctx, &productpb.GetProductRequest{Id: productID})
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			return appErrors.NotFound("product not found")
		}
		return err
	}
	if res == nil || res.Product == nil {
		return appErrors.NotFound("invalid product reference")
	}

	if variantID != nil && strings.TrimSpace(*variantID) != "" {
		targetVariantID := strings.TrimSpace(*variantID)
		found := false
		for _, v := range res.Product.Variants {
			if v != nil && v.Id == targetVariantID {
				found = true
				break
			}
		}
		if !found {
			return appErrors.NotFound("invalid variant reference")
		}
	}
	return nil
}

func (c *grpcProductClient) VerifyProductMerchant(ctx context.Context, merchantID, productID string) (bool, error) {
	if merchantID == "" {
		return true, nil
	}

	if c.storeClient == nil {
		return false, appErrors.Internal(nil, "store service client unavailable")
	}

	prod, err := c.GetProduct(ctx, productID)
	if err != nil {
		return false, err
	}
	if prod == nil {
		return false, appErrors.NotFound("product not found")
	}

	storeRes, err := c.storeClient.GetStore(ctx, &storepb.GetStoreRequest{Id: prod.StoreID})
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			return false, appErrors.NotFound("store not found for product")
		}
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

