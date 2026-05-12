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

func WebSocketUpgrade(app *fiber.App) {
	app.Use("/api/v1/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	app.Get("/api/v1/ws", websocket.New(func(c *websocket.Conn) {
		pubsub := redisClient.Subscribe(context.Background(), "reconciliation.events")
		defer pubsub.Close()

		pingCh := make(chan struct{}, 1)
		go func() {
			for {
				_, msg, err := c.ReadMessage()
				if err != nil {
					return
				}
				if string(msg) == `{"type":"ping"}` {
					pingCh <- struct{}{}
				}
			}
		}()

		ch := pubsub.Channel()
		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					return
				}
				if err := c.WriteMessage(websocket.TextMessage, []byte(msg.Payload)); err != nil {
					log.Println("ws write error:", err)
					return
				}
			case <-pingCh:
				if err := c.WriteMessage(websocket.TextMessage, []byte(`{"type":"pong"}`)); err != nil {
					return
				}
			}
		}
	}))
}
