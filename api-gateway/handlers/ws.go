package handlers

import (
	"context"
	"log"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

var redisClient *redis.Client

func InitRedis(url string) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		log.Fatalf("failed to parse redis URL: %v", err)
	}
	redisClient = redis.NewClient(opts)
}

func SetupWebSocket(api fiber.Router) {
	api.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	api.Get("/ws", websocket.New(func(c *websocket.Conn) {
		defer c.Close()

		pubsub := redisClient.Subscribe(context.Background(), "reconciliation.events")
		defer pubsub.Close()
		ch := pubsub.Channel()

		c.SetPingHandler(func(appData string) error {
			return c.WriteMessage(websocket.PongMessage, []byte(appData))
		})

		go func() {
			for {
				if _, _, err := c.ReadMessage(); err != nil {
					return
				}
			}
		}()

		for msg := range ch {
			if err := c.WriteMessage(websocket.TextMessage, []byte(msg.Payload)); err != nil {
				log.Println("WS write error:", err)
				break
			}
		}
	}))
}
