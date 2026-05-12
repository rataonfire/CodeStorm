package main

import (
    "context"
    "encoding/json"
    "log"
    "net/http"
    "os"
    "time"

    "github.com/google/uuid"
    "github.com/redis/go-redis/v9"
    "github.com/jackc/pgx/v5/pgxpool"
)

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

var db *pgxpool.Pool
var rdb *redis.Client

func main() {
    dbURL := os.Getenv("POSTGRES_URL")
    if dbURL == "" {
        dbURL = "postgres://recon:recon@localhost:5432/recon?sslmode=disable"
    }
    
    var err error
    db, err = pgxpool.New(context.Background(), dbURL)
    if err != nil {
        log.Fatalf("Unable to connect to database: %v", err)
    }
    defer db.Close()
    log.Println("Connected to PostgreSQL")
    
    redisURL := os.Getenv("REDIS_URL")
    if redisURL == "" {
        redisURL = "redis://localhost:6379/0"
    }
    
    opt, err := redis.ParseURL(redisURL)
    if err != nil {
        log.Fatalf("Invalid Redis URL: %v", err)
    }
    
    rdb = redis.NewClient(opt)
    if err := rdb.Ping(context.Background()).Err(); err != nil {
        log.Fatalf("Unable to connect to Redis: %v", err)
    }
    log.Println("Connected to Redis")
    
    http.HandleFunc("/healthz", healthHandler)
    http.HandleFunc("/readyz", readyHandler)
    http.HandleFunc("/api/v1/events/merchant", handleEvent)
    http.HandleFunc("/api/v1/events/gateway", handleEvent)
    http.HandleFunc("/api/v1/events/bank", handleEvent)
    
    port := os.Getenv("INGESTOR_PORT")
    if port == "" {
        port = "8080"
    }
    
    log.Printf("Ingestor starting on port %s", port)
    log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}

func readyHandler(w http.ResponseWriter, r *http.Request) {
    if err := db.Ping(context.Background()); err != nil {
        w.WriteHeader(http.StatusServiceUnavailable)
        w.Write([]byte("Database not ready"))
        return
    }
    if err := rdb.Ping(context.Background()).Err(); err != nil {
        w.WriteHeader(http.StatusServiceUnavailable)
        w.Write([]byte("Redis not ready"))
        return
    }
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}

func handleEvent(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        w.WriteHeader(http.StatusMethodNotAllowed)
        return
    }
    
    var event PaymentEvent
    if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
        return
    }
    
    source := r.URL.Path[len("/api/v1/events/"):]
    event.Source = source
    
    if event.EventID == "" || event.TransactionID == "" {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(map[string]string{"error": "event_id and transaction_id are required"})
        return
    }
    
    internalID := uuid.Must(uuid.NewV7()).String()
    
    ctx := context.Background()
    _, err := db.Exec(ctx, `
        INSERT INTO events 
        (internal_event_id, event_id, transaction_id, correlation_id, source,
         amount_minor, fee_minor, currency, timestamp_ms, tx_type, merchant_id, raw_payload, received_at_ms)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
        ON CONFLICT (event_id, source) DO NOTHING
    `, internalID, event.EventID, event.TransactionID, event.TransactionID, event.Source,
       event.AmountMinor, event.FeeMinor, event.Currency, event.TimestampMs, event.TxType,
       event.MerchantID, event, time.Now().UnixMilli())
    
    if err != nil {
        log.Printf("Database error: %v", err)
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(map[string]string{"error": "database error"})
        return
    }
    
    streamData, _ := json.Marshal(map[string]interface{}{
        "event": event,
        "internal_event_id": internalID,
        "ingested_at_ms": time.Now().UnixMilli(),
    })
    
    err = rdb.XAdd(ctx, &redis.XAddArgs{
        Stream: "transactions:raw",
        Values: map[string]interface{}{"payload": string(streamData)},
    }).Err()
    
    if err != nil {
        log.Printf("Redis error: %v", err)
    }
    
    log.Printf("Accepted %s event for tx %s", source, event.TransactionID)
    
    w.WriteHeader(http.StatusAccepted)
    json.NewEncoder(w).Encode(map[string]interface{}{
        "status": "accepted",
        "internal_event_id": internalID,
        "message": "event queued for reconciliation",
    })
}