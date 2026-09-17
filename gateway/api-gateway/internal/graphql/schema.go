package graphql

import (
	"github.com/graphql-go/graphql"
	mutation "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/mutations"
	resolver "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/resolvers"
)

func NewSchema(res *resolver.Resolver, mut ...*mutation.MutationResolver) (graphql.Schema, error) {
	var mutResolver *mutation.MutationResolver
	if len(mut) > 0 && mut[0] != nil {
		mutResolver = mut[0]
	} else if res != nil {
		mutResolver = mutation.NewMutationResolver(res.Clients)
	} else {
		mutResolver = mutation.NewMutationResolver(nil)
	}

	// User Type
	userType := graphql.NewObject(graphql.ObjectConfig{
		Name: "User",
		Fields: graphql.Fields{
			"id":        &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"email":     &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"firstName": &graphql.Field{Type: graphql.String},
			"lastName":  &graphql.Field{Type: graphql.String},
			"role":      &graphql.Field{Type: graphql.String},
			"createdAt": &graphql.Field{Type: graphql.String},
		},
	})

	// Product Type
	productType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Product",
		Fields: graphql.Fields{
			"id":            &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"name":          &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"description":   &graphql.Field{Type: graphql.String},
			"price":         &graphql.Field{Type: graphql.NewNonNull(graphql.Float)},
			"categoryId":    &graphql.Field{Type: graphql.String},
			"stockQuantity": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"createdAt":     &graphql.Field{Type: graphql.String},
		},
	})

	// CartItem Type
	cartItemType := graphql.NewObject(graphql.ObjectConfig{
		Name: "CartItem",
		Fields: graphql.Fields{
			"id":        &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"productId": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"quantity":  &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"unitPrice": &graphql.Field{Type: graphql.NewNonNull(graphql.Float)},
		},
	})

	// Cart Type
	cartType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Cart",
		Fields: graphql.Fields{
			"id":          &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"userId":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"items":       &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(cartItemType)))},
			"totalAmount": &graphql.Field{Type: graphql.NewNonNull(graphql.Float)},
		},
	})

	// OrderItem Type
	orderItemType := graphql.NewObject(graphql.ObjectConfig{
		Name: "OrderItem",
		Fields: graphql.Fields{
			"id":        &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"productId": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"quantity":  &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
			"price":     &graphql.Field{Type: graphql.NewNonNull(graphql.Float)},
		},
	})

	// Order Type
	orderType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Order",
		Fields: graphql.Fields{
			"id":          &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"userId":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"status":      &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"items":       &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(orderItemType)))},
			"totalAmount": &graphql.Field{Type: graphql.NewNonNull(graphql.Float)},
			"createdAt":   &graphql.Field{Type: graphql.String},
		},
	})

	// AuthPayload Type
	authPayloadType := graphql.NewObject(graphql.ObjectConfig{
		Name: "AuthPayload",
		Fields: graphql.Fields{
			"token": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"user":  &graphql.Field{Type: graphql.NewNonNull(userType)},
		},
	})

	// Input Types
	loginInput := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "LoginInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"email":    &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"password": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		},
	})

	registerInput := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "RegisterInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"email":     &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"password":  &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"firstName": &graphql.InputObjectFieldConfig{Type: graphql.String},
			"lastName":  &graphql.InputObjectFieldConfig{Type: graphql.String},
		},
	})

	createProductInput := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "CreateProductInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"name":          &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"description":   &graphql.InputObjectFieldConfig{Type: graphql.String},
			"price":         &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.Float)},
			"categoryId":    &graphql.InputObjectFieldConfig{Type: graphql.String},
			"stockQuantity": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.Int)},
		},
	})

	addToCartInput := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "AddToCartInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"userId":    &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"productId": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"quantity":  &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.Int)},
		},
	})

	createOrderInput := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "CreateOrderInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"userId":          &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"cartId":          &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.ID)},
			"shippingAddress": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		},
	})

	// Root Query
	rootQuery := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"health": &graphql.Field{
				Type:        graphql.String,
				Description: "Health check",
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if res == nil {
						return "OK", nil
					}
					return res.Health(p.Context)
				},
			},
			"version": &graphql.Field{
				Type:        graphql.String,
				Description: "Get API version",
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if res == nil {
						return "1.0.0", nil
					}
					return res.GetVersion(p.Context)
				},
			},
			"me": &graphql.Field{
				Type:        userType,
				Description: "Get logged in user details",
				Resolve:     res.Me,
			},
			"user": &graphql.Field{
				Type:        userType,
				Description: "Get user by ID",
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)},
				},
				Resolve: res.User,
			},
			"product": &graphql.Field{
				Type:        productType,
				Description: "Get product by ID",
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)},
				},
				Resolve: res.Product,
			},
			"products": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(productType))),
				Description: "List products with pagination",
				Args: graphql.FieldConfigArgument{
					"limit":  &graphql.ArgumentConfig{Type: graphql.Int},
					"offset": &graphql.ArgumentConfig{Type: graphql.Int},
				},
				Resolve: res.Products,
			},
			"cart": &graphql.Field{
				Type:        cartType,
				Description: "Get cart for user",
				Args: graphql.FieldConfigArgument{
					"userId": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)},
				},
				Resolve: res.Cart,
			},
			"order": &graphql.Field{
				Type:        orderType,
				Description: "Get order by ID",
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)},
				},
				Resolve: res.Order,
			},
		},
	})

	// Root Mutation
	rootMutation := graphql.NewObject(graphql.ObjectConfig{
		Name: "Mutation",
		Fields: graphql.Fields{
			"login": &graphql.Field{
				Type:        graphql.NewNonNull(authPayloadType),
				Description: "User login",
				Args: graphql.FieldConfigArgument{
					"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(loginInput)},
				},
				Resolve: mutResolver.Login,
			},
			"register": &graphql.Field{
				Type:        graphql.NewNonNull(authPayloadType),
				Description: "User registration",
				Args: graphql.FieldConfigArgument{
					"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(registerInput)},
				},
				Resolve: mutResolver.Register,
			},
			"createProduct": &graphql.Field{
				Type:        graphql.NewNonNull(productType),
				Description: "Create a new product",
				Args: graphql.FieldConfigArgument{
					"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(createProductInput)},
				},
				Resolve: mutResolver.CreateProduct,
			},
			"addToCart": &graphql.Field{
				Type:        graphql.NewNonNull(cartType),
				Description: "Add item to user cart",
				Args: graphql.FieldConfigArgument{
					"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(addToCartInput)},
				},
				Resolve: mutResolver.AddToCart,
			},
			"createOrder": &graphql.Field{
				Type:        graphql.NewNonNull(orderType),
				Description: "Create order from cart",
				Args: graphql.FieldConfigArgument{
					"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(createOrderInput)},
				},
				Resolve: mutResolver.CreateOrder,
			},
		},
	})

	return graphql.NewSchema(graphql.SchemaConfig{
		Query:    rootQuery,
		Mutation: rootMutation,
	})
}
