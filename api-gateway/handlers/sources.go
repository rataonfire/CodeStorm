package handlers

import "github.com/gofiber/fiber/v2"

func SourcesHealth(c *fiber.Ctx) error {
	// Заглушка – возвращаем все источники онлайн
	return c.JSON(fiber.Map{
		"merchant": "online",
		"gateway":  "online",
		"bank":     "online",
	})
}