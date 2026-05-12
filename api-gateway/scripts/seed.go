package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/google/uuid"
)

func main() {
	connString := "postgres://postgres:postgres@localhost:5432/payment_reconciliation?sslmode=disable"
	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		log.Fatal("Failed to connect:", err)
	}
	defer pool.Close()

	// Очистка (правильный порядок: сначала dependent tables)
	_, err = pool.Exec(context.Background(), "TRUNCATE incidents, reconciliation_details, transactions, events RESTART IDENTITY CASCADE")
	if err != nil {
		log.Fatal("Truncate failed:", err)
	}

	for i := 0; i < 20; i++ {
		txID := uuid.New().String()
		status := "matched"
		if i%5 == 0 {
			status = "mismatch"
		} else if i%7 == 0 {
			status = "degraded"
		}
		merchantID := fmt.Sprintf("merchant_%d", i%3)
		txType := "card"
		if i%3 == 0 {
			txType = "transfer"
		}
		currency := "RUB"
		createdAt := time.Now().Add(-time.Duration(i) * time.Minute)
		updatedAt := createdAt.Add(time.Duration(rand.Intn(60)) * time.Second)

		_, err = pool.Exec(context.Background(),
			`INSERT INTO transactions (transaction_id, overall_status, merchant_id, tx_type, currency, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			txID, status, merchantID, txType, currency, createdAt, updatedAt)
		if err != nil {
			log.Printf("Insert tx %s failed: %v", txID, err)
			continue
		}

		// Детали (три источника)
		amountMerchant := int64(10000)
		feeMerchant := int64(0)
		amountGateway := int64(9800)
		feeGateway := int64(200)
		amountBank := int64(9550)
		feeBank := int64(250)

		if status == "mismatch" && i%2 == 0 {
			amountBank = int64(9500)
		}

		details := []struct {
			source         string
			amountExpected *int64
			amountActual   int64
			feeExpected    *int64
			feeActual      int64
			isMatched      bool
			mismatchReason *string
		}{
			{"merchant", nil, amountMerchant, nil, feeMerchant, true, nil},
			{"gateway", &amountMerchant, amountGateway, &feeMerchant, feeGateway, amountMerchant-amountGateway == feeGateway, nil},
			{"bank", &amountGateway, amountBank, &feeGateway, feeBank, amountGateway-amountBank == feeBank, nil},
		}
		for _, d := range details {
			_, err = pool.Exec(context.Background(),
				`INSERT INTO reconciliation_details
				(transaction_id, source, amount_expected, amount_actual, fee_expected, fee_actual, is_matched, mismatch_reason)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
				txID, d.source, d.amountExpected, d.amountActual, d.feeExpected, d.feeActual, d.isMatched, d.mismatchReason)
			if err != nil {
				log.Printf("Insert detail for %s/%s failed: %v", txID, d.source, err)
			}
		}

		// Инциденты
		if status == "mismatch" {
			incidentType := "amount_mismatch"
			severity := 1
			description := "Bank amount does not match expected"
			if i%10 == 0 {
				incidentType = "fee_mismatch"
				description = "Gateway fee differs from bank fee"
				severity = 2
			}
			_, err = pool.Exec(context.Background(),
				`INSERT INTO incidents (transaction_id, incident_type, severity, description, status, created_at)
				VALUES ($1, $2, $3, $4, $5, $6)`,
				txID, incidentType, severity, description, "open", createdAt)
			if err != nil {
				log.Printf("Insert incident for %s failed: %v", txID, err)
			}
		}
	}

	fmt.Println("Seed completed. Inserted 20 transactions with details and incidents.")
}