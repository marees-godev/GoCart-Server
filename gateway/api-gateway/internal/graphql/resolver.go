package graphql

import (
	"github.com/graphql-go/graphql"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/cartpb"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/orderpb"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/productpb"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/userpb"
)

type Resolver struct {
	clients *grpc.Clients
}

func NewResolver(clients *grpc.Clients) *Resolver {
	return &Resolver{
		clients: clients,
	}
}

// ----------------------------------------------------------------------------
// Queries
// ----------------------------------------------------------------------------

func (r *Resolver) Me(p graphql.ResolveParams) (interface{}, error) {
	userId, ok := p.Context.Value("userID").(string)
	if !ok || userId == "" {
		return nil, appErrors.Unauthorized("authentication required")
	}

	res, err := r.clients.UserClient.GetUser(p.Context, &userpb.GetUserRequest{Id: userId})
	if err != nil {
		return nil, err
	}
	return mapUser(res.User), nil
}

func (r *Resolver) User(p graphql.ResolveParams) (interface{}, error) {
	id, _ := p.Args["id"].(string)
	if id == "" {
		return nil, appErrors.BadRequest("user id is required")
	}

	res, err := r.clients.UserClient.GetUser(p.Context, &userpb.GetUserRequest{Id: id})
	if err != nil {
		return nil, err
	}
	return mapUser(res.User), nil
}

func (r *Resolver) Product(p graphql.ResolveParams) (interface{}, error) {
	id, _ := p.Args["id"].(string)
	if id == "" {
		return nil, appErrors.BadRequest("product id is required")
	}

	res, err := r.clients.ProductClient.GetProduct(p.Context, &productpb.GetProductRequest{Id: id})
	if err != nil {
		return nil, err
	}
	return mapProduct(res.Product), nil
}

func (r *Resolver) Products(p graphql.ResolveParams) (interface{}, error) {
	limit := int32(10)
	if l, ok := p.Args["limit"].(int); ok && l > 0 {
		limit = int32(l)
	}
	offset := int32(0)
	if o, ok := p.Args["offset"].(int); ok && o >= 0 {
		offset = int32(o)
	}

	res, err := r.clients.ProductClient.ListProducts(p.Context, &productpb.ListProductsRequest{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, err
	}

	products := make([]map[string]interface{}, len(res.Products))
	for i, prod := range res.Products {
		products[i] = mapProduct(prod)
	}
	return products, nil
}

func (r *Resolver) Cart(p graphql.ResolveParams) (interface{}, error) {
	userId, _ := p.Args["userId"].(string)
	if userId == "" {
		return nil, appErrors.BadRequest("userId is required")
	}

	res, err := r.clients.CartClient.GetCart(p.Context, &cartpb.GetCartRequest{UserId: userId})
	if err != nil {
		return nil, err
	}
	return mapCart(res.Cart), nil
}

func (r *Resolver) Order(p graphql.ResolveParams) (interface{}, error) {
	id, _ := p.Args["id"].(string)
	if id == "" {
		return nil, appErrors.BadRequest("order id is required")
	}

	res, err := r.clients.OrderClient.GetOrder(p.Context, &orderpb.GetOrderRequest{Id: id})
	if err != nil {
		return nil, err
	}
	return mapOrder(res.Order), nil
}

// ----------------------------------------------------------------------------
// Mutations
// ----------------------------------------------------------------------------

func (r *Resolver) Login(p graphql.ResolveParams) (interface{}, error) {
	input, _ := p.Args["input"].(map[string]interface{})
	email, _ := input["email"].(string)
	password, _ := input["password"].(string)

	if email == "" || password == "" {
		return nil, appErrors.BadRequest("email and password are required")
	}

	res, err := r.clients.UserClient.Login(p.Context, &userpb.LoginRequest{
		Email:    email,
		Password: password,
	})
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"token": res.Token,
		"user":  mapUser(res.User),
	}, nil
}

func (r *Resolver) Register(p graphql.ResolveParams) (interface{}, error) {
	input, _ := p.Args["input"].(map[string]interface{})
	email, _ := input["email"].(string)
	password, _ := input["password"].(string)
	firstName, _ := input["firstName"].(string)
	lastName, _ := input["lastName"].(string)

	if email == "" || password == "" {
		return nil, appErrors.BadRequest("email and password are required")
	}

	res, err := r.clients.UserClient.Register(p.Context, &userpb.RegisterRequest{
		Email:     email,
		Password:  password,
		FirstName: firstName,
		LastName:  lastName,
	})
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"token": res.Token,
		"user":  mapUser(res.User),
	}, nil
}

func (r *Resolver) CreateProduct(p graphql.ResolveParams) (interface{}, error) {
	input, _ := p.Args["input"].(map[string]interface{})
	name, _ := input["name"].(string)
	desc, _ := input["description"].(string)
	price, _ := input["price"].(float64)
	categoryID, _ := input["categoryId"].(string)
	stockQty, _ := input["stockQuantity"].(int)

	if name == "" || price <= 0 {
		return nil, appErrors.BadRequest("valid product name and positive price are required")
	}

	res, err := r.clients.ProductClient.CreateProduct(p.Context, &productpb.CreateProductRequest{
		Name:          name,
		Description:   desc,
		Price:         price,
		CategoryId:    categoryID,
		StockQuantity: int32(stockQty),
	})
	if err != nil {
		return nil, err
	}
	return mapProduct(res.Product), nil
}

func (r *Resolver) AddToCart(p graphql.ResolveParams) (interface{}, error) {
	input, _ := p.Args["input"].(map[string]interface{})
	userID, _ := input["userId"].(string)
	productID, _ := input["productId"].(string)
	qty, _ := input["quantity"].(int)

	if userID == "" || productID == "" || qty <= 0 {
		return nil, appErrors.BadRequest("userId, productId, and positive quantity are required")
	}

	res, err := r.clients.CartClient.AddToCart(p.Context, &cartpb.AddToCartRequest{
		UserId:    userID,
		ProductId: productID,
		Quantity:  int32(qty),
	})
	if err != nil {
		return nil, err
	}
	return mapCart(res.Cart), nil
}

func (r *Resolver) CreateOrder(p graphql.ResolveParams) (interface{}, error) {
	input, _ := p.Args["input"].(map[string]interface{})
	userID, _ := input["userId"].(string)
	cartID, _ := input["cartId"].(string)
	shippingAddress, _ := input["shippingAddress"].(string)

	if userID == "" || cartID == "" {
		return nil, appErrors.BadRequest("userId and cartId are required")
	}

	res, err := r.clients.OrderClient.CreateOrder(p.Context, &orderpb.CreateOrderRequest{
		UserId:          userID,
		CartId:          cartID,
		ShippingAddress: shippingAddress,
	})
	if err != nil {
		return nil, err
	}
	return mapOrder(res.Order), nil
}

// ----------------------------------------------------------------------------
// Response Mappers
// ----------------------------------------------------------------------------

func mapUser(u *userpb.User) map[string]interface{} {
	if u == nil {
		return nil
	}
	return map[string]interface{}{
		"id":        u.Id,
		"email":     u.Email,
		"firstName": u.FirstName,
		"lastName":  u.LastName,
		"role":      u.Role,
		"createdAt": u.CreatedAt,
	}
}

func mapProduct(p *productpb.Product) map[string]interface{} {
	if p == nil {
		return nil
	}
	return map[string]interface{}{
		"id":            p.Id,
		"name":          p.Name,
		"description":   p.Description,
		"price":         p.Price,
		"categoryId":    p.CategoryId,
		"stockQuantity": p.StockQuantity,
		"createdAt":     p.CreatedAt,
	}
}

func mapCart(c *cartpb.Cart) map[string]interface{} {
	if c == nil {
		return nil
	}
	items := make([]map[string]interface{}, len(c.Items))
	for i, item := range c.Items {
		items[i] = map[string]interface{}{
			"id":        item.Id,
			"productId": item.ProductId,
			"quantity":  item.Quantity,
			"unitPrice": item.UnitPrice,
		}
	}
	return map[string]interface{}{
		"id":          c.Id,
		"userId":      c.UserId,
		"items":       items,
		"totalAmount": c.TotalAmount,
	}
}

func mapOrder(o *orderpb.Order) map[string]interface{} {
	if o == nil {
		return nil
	}
	items := make([]map[string]interface{}, len(o.Items))
	for i, item := range o.Items {
		items[i] = map[string]interface{}{
			"id":        item.Id,
			"productId": item.ProductId,
			"quantity":  item.Quantity,
			"price":     item.Price,
		}
	}
	return map[string]interface{}{
		"id":          o.Id,
		"userId":      o.UserId,
		"status":      o.Status,
		"items":       items,
		"totalAmount": o.TotalAmount,
		"createdAt":   o.CreatedAt,
	}
}
