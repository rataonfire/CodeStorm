package main

import (
	"context"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"

	"yourproject/api/db"
	"yourproject/api/handlers"
)

func main() {
	if err := db.InitDB(); err != nil {
		log.Fatal(err)
	}
	defer db.Pool.Close()

	redisURL := getEnv("REDIS_URL", "redis://localhost:6379/0")
	handlers.InitRedis(redisURL)

	app := fiber.New(fiber.Config{
		ErrorHandler: handlers.CustomErrorHandler,
	})
	app.Use(cors.New())

	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.SendStatus(200)
	})
	app.Get("/readyz", func(c *fiber.Ctx) error {
		if err := db.Pool.Ping(context.Background()); err != nil {
			return c.Status(503).JSON(fiber.Map{"error": "database not ready"})
		}
		return c.SendStatus(200)
	})

	api := app.Group("/api/v1")
	api.Get("/transactions", handlers.GetTransactions)
	api.Get("/transactions/:tx_id", handlers.GetTransactionDetails)
	api.Get("/incidents", handlers.GetIncidents)
	api.Post("/incidents/:id/ack", handlers.AcknowledgeIncident)
	api.Post("/incidents/:id/resolve", handlers.ResolveIncident)
	api.Get("/metrics/mismatches-per-minute", handlers.MismatchPerMinute)
	api.Get("/sources/health", handlers.SourcesHealth)

	handlers.SetupWebSocket(api)

	port := getEnv("API_PORT", "8090")
	log.Fatal(app.Listen(":" + port))
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
