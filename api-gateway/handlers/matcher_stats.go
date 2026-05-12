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
	LogicLatency     LatencyStats `json:"logic_latency"`
	Timestamp        time.Time   `json:"timestamp"`
}

type LatencyStats struct {
	P50MS  float64 `json:"p50_ms"`
	P99MS  float64 `json:"p99_ms"`
	AvgMS  float64 `json:"avg_ms"`
}

func GetMatcherStatsHandler(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()


	statsJSON, err := redisClient.Get(ctx, "matcher:stats").Result()
	if err != nil && err.Error() != "redis: nil" {
		return c.Status(500).JSON(fiber.Map{"error": "failed to get matcher stats"})
	}

	if statsJSON == "" {

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


func GetMatcherSpeedometer(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	calcLatency := func(key string) (avg, p50, p99 float64) {
		latencies, _ := redisClient.LRange(ctx, key, 0, -1).Result()
		var values []int64
		for _, s := range latencies {
			if val, err := strconv.ParseInt(s, 10, 64); err == nil {
				values = append(values, val)
			}
		}
		if len(values) == 0 {
			return 0, 0, 0
		}
		var sum int64
		for _, v := range values {
			sum += v
		}
		avg = float64(sum) / float64(len(values)) / 1000.0
		sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
		p50 = float64(values[len(values)/2]) / 1000.0
		idx := len(values) * 99 / 100
		if idx >= len(values) {
			idx = len(values) - 1
		}
		p99 = float64(values[idx]) / 1000.0
		return
	}

	avgL, p50L, p99L := calcLatency("metrics:latencies:total")
	avgLogic, p50Logic, p99Logic := calcLatency("metrics:latencies:logic")

	totalEvents, _ := redisClient.Get(ctx, "metrics:events:total").Int64()
	successMatches, _ := redisClient.Get(ctx, "metrics:matches:success").Int64()
	failedMatches, _ := redisClient.Get(ctx, "metrics:matches:failed").Int64()

	successRate := int32(0)
	if successMatches+failedMatches > 0 {
		successRate = int32((successMatches * 100) / (successMatches + failedMatches))
	}

	return c.JSON(fiber.Map{
		"avg_latency_ms":       avgL,
		"p50_latency_ms":       p50L,
		"p99_latency_ms":       p99L,
		"avg_logic_latency_ms": avgLogic,
		"p50_logic_latency_ms": p50Logic,
		"p99_logic_latency_ms": p99Logic,
		"success_rate":         successRate,
		"total_processed":      totalEvents,
		"successful_matches":   successMatches,
		"failed_matches":       failedMatches,
		"timestamp":            time.Now().Unix(),
	})
}
