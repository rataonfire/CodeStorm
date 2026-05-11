package handlers

import (
	"context"
	"log"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

var redisClient *redis.Client

func InitRedis(addr string) {
	redisClient = redis.NewClient(&redis.Options{
		Addr: addr,
	})
}

func WebSocketUpgrade(app *fiber.App) {
	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	app.Get("/ws", websocket.New(func(c *websocket.Conn) {
		// Подписываемся на Redis канал
		pubsub := redisClient.Subscribe(context.Background(), "reconciliation_events")
		defer pubsub.Close()

		ch := pubsub.Channel()
		for msg := range ch {
			// Отправляем сообщение клиенту
			if err := c.WriteMessage(websocket.TextMessage, []byte(msg.Payload)); err != nil {
				log.Println("write error:", err)
				break
			}
		}
	}))
}