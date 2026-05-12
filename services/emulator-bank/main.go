package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type CanonicalTransaction struct {
	TransactionID   string `json:"transaction_id"`
	AmountMinor     int64  `json:"amount_minor"`
	Currency        string `json:"currency"`
	TxType          string `json:"tx_type"`
	MerchantID      string `json:"merchant_id"`
	GatewayFeeMinor int64  `json:"gateway_fee_minor"`
	BankFeeMinor    int64  `json:"bank_fee_minor"`
	StartedAtMs     int64  `json:"started_at_ms"`
}

type PaymentEvent struct {
	EventID       string `json:"event_id"`
	TransactionID string `json:"transaction_id"`
	Source        string `json:"source"`
	AmountMinor   int64  `json:"amount_minor"`
	FeeMinor      int64  `json:"fee_minor"`
	Currency      string `json:"currency"`
	TimestampMs   int64  `json:"timestamp_ms"`
	TxType        string `json:"tx_type"`
	MerchantID    string `json:"merchant_id,omitempty"`
}

func main() {
	source := "bank"
	ingestorURL := getEnv("EMULATOR_INGESTOR_URL", "http://localhost:8080")
	missingPct := getEnvFloat("EMULATOR_PCT_MISSING", 0.05)
	duplicatePct := getEnvFloat("EMULATOR_PCT_DUPLICATE", 0.02)
	wrongAmountPct := getEnvFloat("EMULATOR_PCT_WRONG_AMOUNT", 0.03)
	wrongFeePct := getEnvFloat("EMULATOR_PCT_WRONG_FEE", 0.03)
	delayMinMs := getEnvInt("EMULATOR_DELAY_MS_MIN", 50)
	delayMaxMs := getEnvInt("EMULATOR_DELAY_MS_MAX", 800)

	redisURL := getEnv("REDIS_URL", "redis://localhost:6379/0")
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("Failed to parse Redis URL: %v", err)
	}

	rdb := redis.NewClient(opts)
	defer rdb.Close()

	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Redis connection failed: %v", err)
	}

	pubsub := rdb.Subscribe(ctx, "scenario.tx")
	defer pubsub.Close()

	ch := pubsub.Channel()

	log.Printf("Emulator %s started, listening for scenarios", source)

	for msg := range ch {
		var canonical CanonicalTransaction
		if err := json.Unmarshal([]byte(msg.Payload), &canonical); err != nil {
			log.Printf("Failed to unmarshal canonical tx: %v", err)
			continue
		}

		// Bank amount = N - F_g
		amountMinor := canonical.AmountMinor - canonical.GatewayFeeMinor
		if amountMinor < 0 {
			amountMinor = 0
		}

		event := &PaymentEvent{
			EventID:       uuid.New().String(),
			TransactionID: canonical.TransactionID,
			Source:        source,
			AmountMinor:   amountMinor,
			FeeMinor:      canonical.BankFeeMinor,
			Currency:      canonical.Currency,
			TimestampMs:   time.Now().UnixMilli(),
			TxType:        canonical.TxType,
			MerchantID:    canonical.MerchantID,
		}

		shouldSend, shouldDuplicate := applyNoise(event, missingPct, duplicatePct, wrongAmountPct, wrongFeePct, delayMinMs, delayMaxMs)

		if shouldSend {
			sendToIngestor(event, ingestorURL, source)
			if shouldDuplicate {
				dupEvent := *event
				dupEvent.EventID = uuid.New().String()
				log.Printf("Sending duplicate for tx %s", canonical.TransactionID)
				sendToIngestor(&dupEvent, ingestorURL, source)
			}
		} else {
			log.Printf("Skipping tx %s (missing)", canonical.TransactionID)
		}
	}
}

func sendToIngestor(event *PaymentEvent, ingestorURL, source string) {
	url := fmt.Sprintf("%s/api/v1/events/%s", ingestorURL, source)
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to marshal event: %v", err)
		return
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Failed to send to ingestor: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 202 {
		log.Printf("Ingestor returned non-202: %d", resp.StatusCode)
	} else {
		log.Printf("Sent %s event for tx %s", source, event.TransactionID)
	}
}

func applyNoise(event *PaymentEvent, missingPct, duplicatePct, wrongAmountPct, wrongFeePct float64, delayMinMs, delayMaxMs int) (shouldSend bool, shouldDuplicate bool) {
	shouldSend = true
	shouldDuplicate = false

	if rand.Float64() < missingPct {
		return false, false
	}

	if rand.Float64() < wrongAmountPct {
		event.AmountMinor = mutateAmount(event.AmountMinor)
		log.Printf("Applied amount noise: new amount=%d", event.AmountMinor)
	}

	if rand.Float64() < wrongFeePct {
		event.FeeMinor = mutateFee(event.FeeMinor)
		log.Printf("Applied fee noise: new fee=%d", event.FeeMinor)
	}

	if rand.Float64() < duplicatePct {
		shouldDuplicate = true
	}

	if delayMaxMs > 0 {
		delayMs := delayMinMs
		if delayMaxMs > delayMinMs {
			delayMs += rand.Intn(delayMaxMs - delayMinMs)
		}
		time.Sleep(time.Duration(delayMs) * time.Millisecond)
	}

	return shouldSend, shouldDuplicate
}

func mutateAmount(amount int64) int64 {
	factor := 0.9 + rand.Float64()*0.2
	return int64(float64(amount) * factor)
}

func mutateFee(fee int64) int64 {
	factor := 0.5 + rand.Float64()
	return int64(float64(fee) * factor)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
		}
	}
	return defaultValue
}
