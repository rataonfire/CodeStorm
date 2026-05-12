package handlers

import (
	"github.com/gofiber/fiber/v2"
	"yourproject/api/db"
)

func Healthz(c *fiber.Ctx) error {
	return c.SendString("OK")
}

func Readyz(c *fiber.Ctx) error {
	if err := db.Pool.Ping(c.Context()); err != nil {
		return c.Status(503).SendString("database unavailable")
	}

	return c.SendString("ready")
}