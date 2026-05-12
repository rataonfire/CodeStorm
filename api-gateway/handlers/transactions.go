package handlers

import (
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"yourproject/api/db"
	"yourproject/api/models"
)

func GetTransactions(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if limit > 200 {
		limit = 200
	}
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	status := c.Query("status")

	query := `SELECT transaction_id, overall_status, created_at, updated_at, merchant_id, tx_type
		FROM transactions WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if status != "" {
		query += fmt.Sprintf(" AND overall_status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := db.Pool.Query(c.Context(), query, args...)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	transactions := []models.TransactionSummary{}
	for rows.Next() {
		var t models.TransactionSummary
		var merchantID *string
		err := rows.Scan(&t.TransactionID, &t.OverallStatus, &t.CreatedAt, &t.UpdatedAt,
			&merchantID, &t.TxType)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		t.MerchantID = merchantID
		transactions = append(transactions, t)
	}

	// Получить общее количество (приблизительное)
	var total int
	_ = db.Pool.QueryRow(c.Context(), "SELECT COUNT(*) FROM transactions").Scan(&total)

	return c.JSON(fiber.Map{
		"items":         transactions,
		"next_cursor":   nil,
		"total_estimate": total,
	})
}

func GetTransactionDetails(c *fiber.Ctx) error {
	txID := c.Params("tx_id")

	var summary models.TransactionSummary
	var merchantID *string
	err := db.Pool.QueryRow(c.Context(),
		`SELECT transaction_id, overall_status, created_at, updated_at, merchant_id, tx_type
		FROM transactions WHERE transaction_id = $1`, txID).
		Scan(&summary.TransactionID, &summary.OverallStatus, &summary.CreatedAt, &summary.UpdatedAt,
			&merchantID, &summary.TxType)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "transaction not found"})
	}
	summary.MerchantID = merchantID

	rows, err := db.Pool.Query(c.Context(),
		`SELECT source, amount_expected, amount_actual, fee_expected, fee_actual, is_matched, mismatch_reason
		FROM reconciliation_details WHERE transaction_id = $1`, txID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	details := []models.ReconciliationDetail{}
	for rows.Next() {
		var d models.ReconciliationDetail
		var amountExpected, feeExpected *int64
		var mismatchReason *string
		err := rows.Scan(&d.Source, &amountExpected, &d.AmountActual, &feeExpected, &d.FeeActual, &d.IsMatched, &mismatchReason)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		d.AmountExpected = amountExpected
		d.FeeExpected = feeExpected
		d.MismatchReason = mismatchReason
		details = append(details, d)
	}

	resp := models.TransactionDetails{
		Summary: summary,
		Details: details,
	}
	return c.JSON(resp)
}