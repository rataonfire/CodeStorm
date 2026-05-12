package handlers

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"yourproject/api/db"
)

func ResolveIncident(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid incident id")
	}

	now := time.Now()
	res, err := db.Pool.Exec(c.Context(),
		`UPDATE incidents SET status = 'resolved', resolved_at = $1 
		WHERE id = $2 AND status IN ('open', 'acknowledged')`,
		now, id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if res.RowsAffected() == 0 {
		return fiber.NewError(fiber.StatusConflict, "incident already resolved or not found")
	}
	return c.JSON(fiber.Map{"status": "resolved"})
}