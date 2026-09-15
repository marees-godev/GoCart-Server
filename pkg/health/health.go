package health

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
)

type Response struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

type Checker interface {
	Check(ctx context.Context) error
}

type CheckerFunc func(ctx context.Context) error

func (f CheckerFunc) Check(ctx context.Context) error {
	return f(ctx)
}

type Pinger interface {
	Ping(ctx context.Context) error
}

type pingerChecker struct {
	pinger Pinger
}

func (p pingerChecker) Check(ctx context.Context) error {
	return p.pinger.Ping(ctx)
}

func FromPinger(p Pinger) Checker {
	if p == nil {
		return nil
	}
	return pingerChecker{pinger: p}
}

type Handler struct {
	serviceName string
	checkers    []Checker
}

func NewHandler(serviceName string, checkers ...Checker) *Handler {
	validCheckers := make([]Checker, 0, len(checkers))
	for _, c := range checkers {
		if c != nil {
			validCheckers = append(validCheckers, c)
		}
	}
	return &Handler{
		serviceName: serviceName,
		checkers:    validCheckers,
	}
}

func (h *Handler) Register(router fiber.Router) {
	router.Get("/health", h.Health)
	router.Get("/ready", h.Ready)
}

func (h *Handler) Health(c *fiber.Ctx) error {
	return c.Status(fiber.StatusOK).JSON(Response{
		Status:  "ok",
		Service: h.serviceName,
	})
}

func (h *Handler) Ready(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.UserContext(), 3*time.Second)
	defer cancel()

	for _, checker := range h.checkers {
		if err := checker.Check(ctx); err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(Response{
				Status:  "unavailable",
				Service: h.serviceName,
			})
		}
	}

	return c.Status(fiber.StatusOK).JSON(Response{
		Status:  "ok",
		Service: h.serviceName,
	})
}
