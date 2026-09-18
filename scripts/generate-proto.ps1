# Script to compile all Protobuf contracts co-located in contracts/protobuf/<service>/
$ErrorActionPreference = "Stop"

$protoFiles = @(
    "contracts/protobuf/auth/auth.proto",
    "contracts/protobuf/cart/cart.proto",
    "contracts/protobuf/category/category.proto",
    "contracts/protobuf/delivery/delivery.proto",
    "contracts/protobuf/inventory/inventory.proto",
    "contracts/protobuf/merchant/merchant.proto",
    "contracts/protobuf/notification/notification.proto",
    "contracts/protobuf/order/order.proto",
    "contracts/protobuf/payment/payment.proto",
    "contracts/protobuf/product/product.proto",
    "contracts/protobuf/rating/rating.proto",
    "contracts/protobuf/return/return.proto",
    "contracts/protobuf/store/store.proto",
    "contracts/protobuf/user/user.proto"
)

Write-Host "Generating Protobuf code for all services..."
protoc -I. --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative $protoFiles

Write-Host "Successfully generated Protobuf and gRPC code."
