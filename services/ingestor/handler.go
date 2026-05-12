package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type PaymentTransactionEvent struct {
	EventID       string `json:"event_id"`
	TransactionID string `json:"transaction_id"`
	Source        string `json:"source"`
	AmountMinor   int64  `json:"amount_minor"`
	FeeMinor      int64  `json:"fee_minor"`
	Currency      string `json:"currency"`
	TimestampMs   int64  `json:"timestamp_ms"`
	TxType        string `json:"tx_type"`
	MerchantID    string `json:"merchant_id,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

type StreamMessage struct {
	Event           PaymentTransactionEvent `json:"event"`
	InternalEventID string                  `json:"internal_event_id"`
	CorrelationID   string                  `json:"correlation_id"`
	IngestedAtMs    int64                   `json:"ingested_at_ms"`
}

type Handler struct {
	db  *pgxpool.Pool
	rdb *redis.Client
}

func NewHandler(db *pgxpool.Pool, rdb *redis.Client) *Handler {
	return &Handler{
		db:  db,
		rdb: rdb,
	}
}

func (h *Handler) handleEvent(c *fiber.Ctx) error {
	source := c.Params("source")
	if source == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "validation_failed",
				"message": "source is required",
			},
		})
	}

	// Parse request body
	var event PaymentTransactionEvent
	if err := c.BodyParser(&event); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "validation_failed",
				"message": "invalid JSON: " + err.Error(),
			},
		})
	}

	// Set source from URL
	event.Source = source

	// Validate event
	if err := validateEvent(&event); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "validation_failed",
				"message": err.Error(),
			},
		})
	}

	// Idempotency check
	apiKey := c.Get("Authorization")
	idempotencyKey := c.Get("Idempotency-Key")
	if idempotencyKey == "" {
		// Generate from content hash
		hash := sha256.Sum256(c.Body())
		idempotencyKey = hex.EncodeToString(hash[:])
	}

	idempKey := fmt.Sprintf("idempotency:%s:%s", apiKey, idempotencyKey)
	ctx := c.Context()

	// Try to set idempotency key
	set, err := h.rdb.SetNX(ctx, idempKey, "PROCESSING", 24*time.Hour).Result()
	if err != nil {
		log.Printf("Idempotency check failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "internal_error",
				"message": "idempotency check failed",
			},
		})
	}

	if !set {
		// Key exists, check if it's still processing or return cached response
		val, _ := h.rdb.Get(ctx, idempKey).Result()
		if val == "PROCESSING" {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": fiber.Map{
					"code":    "idempotency_in_flight",
					"message": "request with same key is being processed",
				},
			})
		}
		// Return cached response
		var cachedResp map[string]interface{}
		if err := json.Unmarshal([]byte(val), &cachedResp); err == nil {
			return c.Status(fiber.StatusAccepted).JSON(cachedResp)
		}
	}

	// Identity resolution (MVP: correlation_id = transaction_id)
	correlationID := event.TransactionID

	// Generate internal ID (UUID v7)
	internalID := uuid.Must(uuid.NewV7()).String()

	// Persist to Postgres
	rawPayload, _ := json.Marshal(event)
	receivedAtMs := time.Now().UnixMilli()

	_, err = h.db.Exec(ctx, `
        INSERT INTO events 
        (internal_event_id, event_id, transaction_id, correlation_id, source,
         amount_minor, fee_minor, currency, timestamp_ms, tx_type, merchant_id, raw_payload, received_at_ms)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
        ON CONFLICT (event_id, source) DO NOTHING
    `, internalID, event.EventID, event.TransactionID, correlationID, event.Source,
		event.AmountMinor, event.FeeMinor, event.Currency, event.TimestampMs, event.TxType,
		event.MerchantID, rawPayload, receivedAtMs)

	if err != nil {
		log.Printf("Failed to insert event: %v", err)
		h.rdb.Del(ctx, idempKey)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "internal_error",
				"message": "failed to persist event",
			},
		})
	}

	// Publish to Redis Stream
	streamMsg := StreamMessage{
		Event:           event,
		InternalEventID: internalID,
		CorrelationID:   correlationID,
		IngestedAtMs:    receivedAtMs,
	}

	msgData, err := json.Marshal(streamMsg)
	if err != nil {
		log.Printf("Failed to marshal stream message: %v", err)
		h.rdb.Del(ctx, idempKey)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "internal_error",
				"message": "failed to prepare stream message",
			},
		})
	}

	err = h.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: "transactions:raw",
		Values: map[string]interface{}{"payload": string(msgData)},
	}).Err()

	if err != nil {
		log.Printf("Failed to add to stream: %v", err)
		h.rdb.Del(ctx, idempKey)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "internal_error",
				"message": "failed to queue event",
			},
		})
	}

	// Cache response
	response := fiber.Map{
		"status":            "accepted",
		"internal_event_id": internalID,
		"message":           "event queued for reconciliation",
	}

	respData, _ := json.Marshal(response)
	h.rdb.Set(ctx, idempKey, string(respData), 24*time.Hour)

	return c.Status(fiber.StatusAccepted).JSON(response)
}

func validateEvent(event *PaymentTransactionEvent) error {
	if event.EventID == "" {
		return fmt.Errorf("event_id is required")
	}

	if _, err := uuid.Parse(event.EventID); err != nil {
		return fmt.Errorf("event_id must be valid UUID")
	}

	if event.TransactionID == "" {
		return fmt.Errorf("transaction_id is required")
	}

	if _, err := uuid.Parse(event.TransactionID); err != nil {
		return fmt.Errorf("transaction_id must be valid UUID")
	}

	if event.Source == "" {
		return fmt.Errorf("source is required")
	}

	validSources := map[string]bool{"merchant": true, "gateway": true, "bank": true}
	if !validSources[event.Source] {
		return fmt.Errorf("source must be one of: merchant, gateway, bank")
	}

	if event.AmountMinor < 0 {
		return fmt.Errorf("amount_minor must be >= 0")
	}

	if event.FeeMinor < 0 {
		return fmt.Errorf("fee_minor must be >= 0")
	}

	// Merchant must have fee 0
	if event.Source == "merchant" && event.FeeMinor != 0 {
		return fmt.Errorf("merchant fee_minor must be 0")
	}

	if event.Currency == "" {
		return fmt.Errorf("currency is required")
	}

	if len(event.Currency) != 3 {
		return fmt.Errorf("currency must be 3-letter ISO code")
	}

	if event.TimestampMs <= 0 {
		return fmt.Errorf("timestamp_ms must be positive")
	}

	// Check timestamp not in future
	now := time.Now().UnixMilli()
	if event.TimestampMs > now+60000 { // Allow 1 minute clock skew
		return fmt.Errorf("timestamp_ms is too far in the future")
	}

	if event.TxType == "" {
		return fmt.Errorf("tx_type is required")
	}

	validTxTypes := map[string]bool{"card": true, "transfer": true}
	if !validTxTypes[event.TxType] {
		return fmt.Errorf("tx_type must be one of: card, transfer")
	}

	return nil
}
