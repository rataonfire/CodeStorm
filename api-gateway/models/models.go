package models

import (
	"encoding/json"
	"time"
)

type TransactionSummary struct {
	TransactionID    string    `json:"transaction_id"`
	OverallStatus    string    `json:"overall_status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	MerchantID       *string   `json:"merchant_id,omitempty"`
	TxType           string    `json:"tx_type"`
	ProcessingTimeMS float64   `json:"processing_time_ms"`
}

type ReconciliationDetail struct {
	Source         string  `json:"source"`
	AmountExpected *int64  `json:"amount_expected,omitempty"`
	AmountActual   int64   `json:"amount_actual"`
	FeeExpected    *int64  `json:"fee_expected,omitempty"`
	FeeActual      int64   `json:"fee_actual"`
	IsMatched      bool    `json:"is_matched"`
	MismatchReason *string `json:"mismatch_reason,omitempty"`
}

type TransactionDetails struct {
	Summary TransactionSummary     `json:"summary"`
	Details []ReconciliationDetail `json:"details"`
}

type Incident struct {
	ID                      int64           `json:"id"`
	TransactionID           string          `json:"transaction_id"`
	IncidentType            string          `json:"incident_type"`
	Severity                int             `json:"severity"`
	Description             string          `json:"description"`
	AffectedSources         []string        `json:"affected_sources"`
	Status                  string          `json:"status"`
	CreatedAt               time.Time       `json:"created_at"`
	AcknowledgedAt          *time.Time      `json:"acknowledged_at,omitempty"`
	ResolvedAt              *time.Time      `json:"resolved_at,omitempty"`
	AutoCorrectionProposed  json.RawMessage `json:"auto_correction_proposed,omitempty"`
}

type MismatchPerMinute struct {
	Minute        time.Time `json:"minute"`
	MismatchCount int       `json:"mismatch_count"`
}