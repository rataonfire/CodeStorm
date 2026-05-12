package handlers

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
)

type SourceHealth struct {
	Source    string `json:"source"`
	Status    string `json:"status"`
	LastSeen  string `json:"last_seen,omitempty"`
	EventCount int64  `json:"event_count,omitempty"`
}

func SourcesHealth(c *fiber.Ctx) error {
	sources := []string{"merchant", "gateway", "bank"}
	result := make(map[string]interface{})
	
	for _, source := range sources {

		lastSeenKey := "source:last_seen:" + source
		eventCountKey := "source:events:" + source
		
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		

		lastSeen, err := redisClient.Get(ctx, lastSeenKey).Result()
		if err != nil && err.Error() != "redis: nil" {
			result[source] = "unknown"
			continue
		}
		

		count, _ := redisClient.Get(ctx, eventCountKey).Int64()
		

		status := "offline"
		if lastSeen != "" {
			lastSeenTime, err := time.Parse(time.RFC3339Nano, lastSeen)
			if err == nil && time.Since(lastSeenTime) < 30*time.Second {
				status = "online"
			}
		}
		
		result[source] = fiber.Map{
			"status": status,
			"last_seen": lastSeen,
			"event_count": count,
		}
	}
	
	return c.JSON(result)
}