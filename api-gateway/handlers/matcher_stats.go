package handlers

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

type MatcherStats struct {
	EventsProcessed  int64       `json:"events_processed"`
	ThroughputEPS    int64       `json:"throughput_eps"`
	SuccessfulMatches int64      `json:"successful_matches"`
	FailedMatches    int64       `json:"failed_matches"`
	MatchSuccessRate int32       `json:"match_success_rate"`
	ActiveWindows    int64       `json:"active_windows"`
	Latency          LatencyStats `json:"latency"`
	Timestamp        time.Time   `json:"timestamp"`
}

type LatencyStats struct {
	P50MS  int64 `json:"p50_ms"`
	P99MS  int64 `json:"p99_ms"`
	AvgMS  int64 `json:"avg_ms"`
}

func GetMatcherStatsHandler(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Retrieve cached stats from Redis
	statsJSON, err := redisClient.Get(ctx, "matcher:stats").Result()
	if err != nil && err.Error() != "redis: nil" {
		return c.Status(500).JSON(fiber.Map{"error": "failed to get matcher stats"})
	}

	if statsJSON == "" {
		// Return default empty stats
		return c.JSON(MatcherStats{
			EventsProcessed:   0,
			ThroughputEPS:     0,
			SuccessfulMatches: 0,
			FailedMatches:     0,
			MatchSuccessRate:  0,
			ActiveWindows:     0,
			Latency: LatencyStats{
				P50MS: 0,
				P99MS: 0,
				AvgMS: 0,
			},
			Timestamp: time.Now(),
		})
	}

	var stats MatcherStats
	if err := json.Unmarshal([]byte(statsJSON), &stats); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to parse matcher stats"})
	}

	stats.Timestamp = time.Now()
	return c.JSON(stats)
}

// GetMatcherSpeedometer returns real-time latency data for visualization
func GetMatcherSpeedometer(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Get latency metrics from Redis
	latencies, err := redisClient.LRange(ctx, "metrics:latencies", 0, -1).Result()
	if err != nil && err.Error() != "redis: nil" {
		latencies = []string{}
	}

	// Parse latencies
	var latencyValues []int64
	for _, s := range latencies {
		if val, err := strconv.ParseInt(s, 10, 64); err == nil {
			latencyValues = append(latencyValues, val)
		}
	}

	// Calculate statistics
	var avgLatency, p50Latency, p99Latency int64
	if len(latencyValues) > 0 {
		// Average
		var sum int64
		for _, v := range latencyValues {
			sum += v
		}
		avgLatency = sum / int64(len(latencyValues))

		// Sort for percentiles
		sort.Slice(latencyValues, func(i, j int) bool { return latencyValues[i] < latencyValues[j] })

		// P50
		p50Latency = latencyValues[len(latencyValues)/2]

		// P99
		idx := len(latencyValues) * 99 / 100
		if idx >= len(latencyValues) {
			idx = len(latencyValues) - 1
		}
		p99Latency = latencyValues[idx]
	}

	totalEvents, _ := redisClient.Get(ctx, "metrics:events:total").Int64()
	successMatches, _ := redisClient.Get(ctx, "metrics:matches:success").Int64()
	failedMatches, _ := redisClient.Get(ctx, "metrics:matches:failed").Int64()

	successRate := int32(0)
	if successMatches+failedMatches > 0 {
		successRate = int32((successMatches * 100) / (successMatches + failedMatches))
	}

	return c.JSON(fiber.Map{
		"avg_latency_ms":      avgLatency,
		"p50_latency_ms":      p50Latency,
		"p99_latency_ms":      p99Latency,
		"success_rate":        successRate,
		"total_processed":     totalEvents,
		"successful_matches":  successMatches,
		"failed_matches":      failedMatches,
		"timestamp":           time.Now().Unix(),
	})
}
