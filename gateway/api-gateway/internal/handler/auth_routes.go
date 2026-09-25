package handler

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	authpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	gatewayGRPC "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
)

func RegisterAuthRoutes(app *fiber.App, grpcClients *gatewayGRPC.Clients) {
	app.Get("/api/v1/auth/verify-email", func(c *fiber.Ctx) error {
		token := c.Query("token")
		c.Set("Content-Type", "text/html; charset=utf-8")

		if token == "" {
			return c.Status(fiber.StatusBadRequest).SendString(renderEmailVerifyCard(false, "Verification token is missing from request."))
		}

		if grpcClients == nil || grpcClients.AuthClient == nil {
			return c.Status(fiber.StatusInternalServerError).SendString(renderEmailVerifyCard(false, "Authentication service is currently unavailable."))
		}

		resp, err := grpcClients.AuthClient.VerifyEmail(c.Context(), &authpb.VerifyEmailRequest{
			Token: token,
		})
		if err != nil || (resp != nil && !resp.Success) {
			msg := "Verification link is invalid or has expired."
			if resp != nil && resp.Message != "" {
				msg = resp.Message
			}
			return c.Status(fiber.StatusBadRequest).SendString(renderEmailVerifyCard(false, msg))
		}

		return c.Status(fiber.StatusOK).SendString(renderEmailVerifyCard(true, "Your email has been successfully verified! You can now access your GoCart account."))
	})
}

func renderEmailVerifyCard(success bool, message string) string {
	title := "Email Verified Successfully"
	badgeColor := "#10B981"
	icon := "✓"
	if !success {
		title = "Verification Failed"
		badgeColor = "#EF4444"
		icon = "✕"
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s - GoCart</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background-color: #0f172a; color: #f8fafc; display: flex; align-items: center; justify-content: center; min-height: 100vh; padding: 20px; }
        .card { background: #1e293b; border: 1px solid #334155; border-radius: 16px; padding: 40px; max-width: 480px; width: 100%%%%; text-align: center; box-shadow: 0 20px 25px -5px rgba(0,0,0,0.5); }
        .icon-circle { width: 72px; height: 72px; border-radius: 50%%%%; background: %s; color: white; display: flex; align-items: center; justify-content: center; font-size: 36px; font-weight: bold; margin: 0 auto 24px; }
        h1 { font-size: 24px; font-weight: 700; margin-bottom: 12px; color: #f8fafc; }
        p { font-size: 15px; color: #94a3b8; line-height: 1.6; margin-bottom: 28px; }
        .brand { font-size: 14px; font-weight: 600; color: #6366f1; letter-spacing: 0.05em; text-transform: uppercase; margin-bottom: 16px; }
    </style>
</head>
<body>
    <div class="card">
        <div class="brand">GoCart Auth</div>
        <div class="icon-circle">%s</div>
        <h1>%s</h1>
        <p>%s</p>
    </div>
</body>
</html>`, title, badgeColor, icon, title, message)
}
