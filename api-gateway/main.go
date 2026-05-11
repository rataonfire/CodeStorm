package main

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"

	"yourproject/api/db"
	"yourproject/api/handlers"
)

func main() {
	// Подключение к БД
	if err := db.InitDB(); err != nil {
		log.Fatal(err)
	}
	defer db.Pool.Close()

	app := fiber.New()
	app.Use(cors.New())

	// Маршруты
	api := app.Group("/api")
	api.Get("/transactions", handlers.GetTransactions)
	api.Get("/transactions/:tx_id", handlers.GetTransactionDetails)
	api.Get("/incidents", handlers.GetIncidents)
	api.Post("/incidents/:id/ack", handlers.AcknowledgeIncident)
	api.Get("/metrics/mismatch-per-minute", handlers.MismatchPerMinute)

	// WebSocket пока не реализован, можно добавить позже
	// handlers.WebSocketUpgrade(app)

	log.Fatal(app.Listen(":8000"))
}