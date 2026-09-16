package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
)

// AdminOutboxHandler exposes operational endpoints for the outbox_events table.
// Mount these under a secured /admin prefix — they should not be publicly accessible.
type AdminOutboxHandler struct {
	store *outbox.Store
	pool  *pgxpool.Pool
}

// NewAdminOutboxHandler constructs the handler with its dependencies.
func NewAdminOutboxHandler(store *outbox.Store, pool *pgxpool.Pool) *AdminOutboxHandler {
	return &AdminOutboxHandler{store: store, pool: pool}
}

// Register mounts all admin outbox routes on the given router group.
func (h *AdminOutboxHandler) Register(r fiber.Router) {
	g := r.Group("/admin/outbox")
	g.Get("/stats", h.stats)
	g.Post("/requeue", h.requeue)
}

// stats godoc
// GET /admin/outbox/stats
// Returns event counts grouped by status (PENDING, PUBLISHED, FAILED).
func (h *AdminOutboxHandler) stats(c *fiber.Ctx) error {
	counts, err := h.store.CountByStatus(c.Context(), h.pool)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to retrieve outbox stats",
		})
	}

	result := make(map[string]int64, len(counts))
	for status, count := range counts {
		result[string(status)] = count
	}
	return c.JSON(result)
}

// requeueRequest defines the optional filters for the requeue operation.
type requeueRequest struct {
	AggregateType string `json:"aggregate_type"`
	EventType     string `json:"event_type"`
}

// requeue godoc
// POST /admin/outbox/requeue
// Resets FAILED events back to PENDING so the publisher will retry them.
// Optionally filter by aggregate_type and/or event_type.
//
// Request body (all fields optional):
//
//	{
//	  "aggregate_type": "order",   // omit to requeue all aggregate types
//	  "event_type": "OrderCreated" // omit to requeue all event types
//	}
func (h *AdminOutboxHandler) requeue(c *fiber.Ctx) error {
	var req requeueRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	count, err := h.store.RequeueFailed(c.Context(), h.pool, req.AggregateType, req.EventType)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to requeue failed events",
		})
	}

	return c.JSON(fiber.Map{
		"requeued": count,
		"filters": fiber.Map{
			"aggregate_type": req.AggregateType,
			"event_type":     req.EventType,
		},
	})
}
