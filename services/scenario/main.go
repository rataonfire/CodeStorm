package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
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

func main() {
	tps := getEnvInt("SCENARIO_TPS", 10)
	txTypes := strings.Split(getEnv("SCENARIO_TX_TYPES", "card,transfer"), ",")
	currency := getEnv("SCENARIO_CURRENCY", "UZS")
	amountMin := getEnvInt64("SCENARIO_AMOUNT_MIN", 10000)
	amountMax := getEnvInt64("SCENARIO_AMOUNT_MAX", 10000000)
	gatewayFeePct := getEnvFloat("SCENARIO_GATEWAY_FEE_PCT", 0.5)
	bankFeePct := getEnvFloat("SCENARIO_BANK_FEE_PCT", 0.2)

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

	log.Printf("Scenario Director started: TPS=%d, currency=%s, amount_range=%d-%d",
		tps, currency, amountMin, amountMax)

	ticker := time.NewTicker(time.Second / time.Duration(tps))
	defer ticker.Stop()

	for range ticker.C {
		tx := generateTransaction(txTypes, currency, amountMin, amountMax, gatewayFeePct, bankFeePct)
		data, err := json.Marshal(tx)
		if err != nil {
			log.Printf("Failed to marshal transaction: %v", err)
			continue
		}

		err = rdb.Publish(ctx, "scenario.tx", data).Err()
		if err != nil {
			log.Printf("Failed to publish transaction: %v", err)
		} else {
			log.Printf("Published transaction: %s amount=%d", tx.TransactionID, tx.AmountMinor)
		}
	}
}

func generateTransaction(txTypes []string, currency string, amountMin, amountMax int64, gatewayFeePct, bankFeePct float64) CanonicalTransaction {
	amount := amountMin + rand.Int63n(amountMax-amountMin+1)
	gatewayFee := int64(float64(amount) * gatewayFeePct / 100)
	bankFee := int64(float64(amount) * bankFeePct / 100)

	txType := txTypes[rand.Intn(len(txTypes))]

	return CanonicalTransaction{
		TransactionID:   uuid.New().String(),
		AmountMinor:     amount,
		Currency:        currency,
		TxType:          txType,
		MerchantID:      fmt.Sprintf("MERCHANT_%03d", rand.Intn(100)),
		GatewayFeeMinor: gatewayFee,
		BankFeeMinor:    bankFee,
		StartedAtMs:     time.Now().UnixMilli(),
	}
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

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
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
