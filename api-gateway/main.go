package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/joho/godotenv"

	"yourproject/api/db"
	"yourproject/api/handlers"
)

func main() {
	godotenv.Load()
	if err := db.InitDB(); err != nil {
		log.Fatal(err)
	}
	defer db.Pool.Close()

	// Redis для WebSocket (опционально, пока можно без него)
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr != "" {
		handlers.InitRedis(redisAddr)
	}

	app := fiber.New(fiber.Config{
		ErrorHandler: handlers.CustomErrorHandler,
	})
	app.Use(cors.New())

	// Health checks
	app.Get("/healthz", handlers.Healthz)
	app.Get("/readyz", handlers.Readyz)

	// API v1
	api := app.Group("/api/v1")
	api.Get("/transactions", handlers.GetTransactions)
	api.Get("/transactions/:tx_id", handlers.GetTransactionDetails)
	api.Get("/incidents", handlers.GetIncidents)
	api.Post("/incidents/:id/ack", handlers.AcknowledgeIncident)
	api.Post("/incidents/:id/resolve", handlers.ResolveIncident)
	api.Get("/metrics/mismatches-per-minute", handlers.MismatchPerMinute)
	api.Get("/sources/health", handlers.SourcesHealth)

	// WebSocket
	handlers.SetupWebSocket(api)

	log.Fatal(app.Listen(":8000"))
}