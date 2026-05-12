// Integration tests for the payment reconciliation system.
// Run with: go test -v -timeout 120s ./...
// Requires the stack to be running: docker compose up -d (then wait for readiness)
//
// Target services:
//   INGESTOR_URL   (default: http://localhost:8080)
//   API_GW_URL     (default: http://localhost:8090)
package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

func ingestorURL() string {
	if u := os.Getenv("INGESTOR_URL"); u != "" {
		return u
	}
	return "http://localhost:8080"
}

func apiURL() string {
	if u := os.Getenv("API_GW_URL"); u != "" {
		return u
	}
	return "http://localhost:8090"
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type paymentEvent struct {
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

func sendEvent(t *testing.T, source string, ev *paymentEvent) int {
	t.Helper()
	ev.Source = source
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	url := fmt.Sprintf("%s/api/v1/events/%s", ingestorURL(), source)
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func makeEvent(txID, source string, amount, fee int64) *paymentEvent {
	return &paymentEvent{
		EventID:       uuid.New().String(),
		TransactionID: txID,
		Source:        source,
		AmountMinor:   amount,
		FeeMinor:      fee,
		Currency:      "UZS",
		TimestampMs:   time.Now().UnixMilli(),
		TxType:        "card",
		MerchantID:    "MERCHANT_001",
	}
}

func waitFor(t *testing.T, label string, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", label)
}

func getJSON(t *testing.T, url string, target interface{}) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("GET %s returned %d: %s", url, resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("unmarshal response from %s: %v (body: %s)", url, err, body)
	}
}

func postJSON(t *testing.T, url string) int {
	t.Helper()
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// ---------------------------------------------------------------------------
// Service readiness
// ---------------------------------------------------------------------------

func TestServicesReady(t *testing.T) {
	for _, tc := range []struct{ name, url string }{
		{"ingestor /healthz", ingestorURL() + "/healthz"},
		{"ingestor /readyz", ingestorURL() + "/readyz"},
		{"api-gateway /healthz", apiURL() + "/healthz"},
		{"api-gateway /readyz", apiURL() + "/readyz"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(tc.url)
			if err != nil {
				t.Fatalf("GET %s: %v", tc.url, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Fatalf("expected 200, got %d", resp.StatusCode)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Ingestor validation
// ---------------------------------------------------------------------------

func TestIngestorValidation(t *testing.T) {
	now := fmt.Sprint(time.Now().UnixMilli())
	tests := []struct {
		name       string
		source     string // URL path source segment
		body       string
		wantStatus int
	}{
		{
			name:       "missing event_id",
			source:     "merchant",
			body:       `{"transaction_id":"` + uuid.New().String() + `","amount_minor":1000,"fee_minor":0,"currency":"UZS","timestamp_ms":` + now + `,"tx_type":"card"}`,
			wantStatus: 400,
		},
		{
			name:       "invalid uuid event_id",
			source:     "merchant",
			body:       `{"event_id":"not-a-uuid","transaction_id":"` + uuid.New().String() + `","amount_minor":1000,"fee_minor":0,"currency":"UZS","timestamp_ms":` + now + `,"tx_type":"card"}`,
			wantStatus: 400,
		},
		{
			// Source in URL is the authority; posting to an unrecognised source → 400
			name:       "invalid source in url",
			source:     "unknown",
			body:       `{"event_id":"` + uuid.New().String() + `","transaction_id":"` + uuid.New().String() + `","amount_minor":1000,"fee_minor":0,"currency":"UZS","timestamp_ms":` + now + `,"tx_type":"card"}`,
			wantStatus: 400,
		},
		{
			name:       "merchant with nonzero fee",
			source:     "merchant",
			body:       `{"event_id":"` + uuid.New().String() + `","transaction_id":"` + uuid.New().String() + `","amount_minor":1000,"fee_minor":50,"currency":"UZS","timestamp_ms":` + now + `,"tx_type":"card"}`,
			wantStatus: 400,
		},
		{
			name:       "invalid tx_type",
			source:     "merchant",
			body:       `{"event_id":"` + uuid.New().String() + `","transaction_id":"` + uuid.New().String() + `","amount_minor":1000,"fee_minor":0,"currency":"UZS","timestamp_ms":` + now + `,"tx_type":"wire"}`,
			wantStatus: 400,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			url := fmt.Sprintf("%s/api/v1/events/%s", ingestorURL(), tc.source)
			resp, err := http.Post(url, "application/json", bytes.NewBufferString(tc.body))
			if err != nil {
				t.Fatalf("POST: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("expected %d, got %d: %s", tc.wantStatus, resp.StatusCode, body)
			}
		})
	}
}

func TestIngestorAcceptsValidEvent(t *testing.T) {
	ev := makeEvent(uuid.New().String(), "merchant", 5_000_000, 0)
	status := sendEvent(t, "merchant", ev)
	if status != 202 {
		t.Fatalf("expected 202, got %d", status)
	}
}

// ---------------------------------------------------------------------------
// Idempotency
// ---------------------------------------------------------------------------

func TestIdempotency(t *testing.T) {
	txID := uuid.New().String()
	ev := makeEvent(txID, "merchant", 5_000_000, 0)
	eventID := ev.EventID

	// Send twice with same event_id
	status1 := sendEvent(t, "merchant", ev)
	if status1 != 202 {
		t.Fatalf("first send expected 202, got %d", status1)
	}

	ev2 := *ev
	ev2.EventID = eventID // same event_id → same content hash
	status2 := sendEvent(t, "merchant", &ev2)
	// Should return 202 (cached) or 409 (idempotency_in_flight)
	if status2 != 202 && status2 != 409 {
		t.Fatalf("second send expected 202 or 409, got %d", status2)
	}
}

// ---------------------------------------------------------------------------
// Full reconciliation flow: happy path (all 3 match)
// ---------------------------------------------------------------------------

func TestHappyPathMatched(t *testing.T) {
	txID := uuid.New().String()
	const N int64 = 5_000_000 // 50 000 UZS
	const Fg int64 = 25_000   // gateway fee
	const Fb int64 = 10_000   // bank fee

	// Send all 3 sources with correct amounts per Project_Bible §2.2
	if s := sendEvent(t, "merchant", makeEvent(txID, "merchant", N, 0)); s != 202 {
		t.Fatalf("merchant ingest: got %d", s)
	}
	if s := sendEvent(t, "gateway", makeEvent(txID, "gateway", N, Fg)); s != 202 {
		t.Fatalf("gateway ingest: got %d", s)
	}
	if s := sendEvent(t, "bank", makeEvent(txID, "bank", N-Fg, Fb)); s != 202 {
		t.Fatalf("bank ingest: got %d", s)
	}

	// Wait up to 5s for reconciler to write to DB
	waitFor(t, "transaction matched in DB", 5*time.Second, func() bool {
		var result struct {
			Summary struct {
				OverallStatus string `json:"overall_status"`
			} `json:"summary"`
		}
		resp, err := http.Get(apiURL() + "/api/v1/transactions/" + txID)
		if err != nil || resp.StatusCode != 200 {
			return false
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(body, &result); err != nil {
			return false
		}
		return result.Summary.OverallStatus == "matched"
	})
}

// ---------------------------------------------------------------------------
// Amount mismatch: I1 violation
// ---------------------------------------------------------------------------

func TestAmountMismatch(t *testing.T) {
	txID := uuid.New().String()
	const N int64 = 5_000_000
	const Fg int64 = 25_000
	const Fb int64 = 10_000

	sendEvent(t, "merchant", makeEvent(txID, "merchant", N, 0))
	sendEvent(t, "gateway", makeEvent(txID, "gateway", N+100_000, Fg)) // ← wrong amount
	sendEvent(t, "bank", makeEvent(txID, "bank", N+100_000-Fg, Fb))

	waitFor(t, "transaction with amount_mismatch incident", 5*time.Second, func() bool {
		return hasIncidentForTx(t, txID, "amount_mismatch")
	})
}

// ---------------------------------------------------------------------------
// Fee mismatch: I2 violation
// ---------------------------------------------------------------------------

func TestFeeMismatch(t *testing.T) {
	txID := uuid.New().String()
	const N int64 = 5_000_000
	const Fg int64 = 25_000
	const Fb int64 = 10_000

	sendEvent(t, "merchant", makeEvent(txID, "merchant", N, 0))
	sendEvent(t, "gateway", makeEvent(txID, "gateway", N, Fg))
	sendEvent(t, "bank", makeEvent(txID, "bank", N-Fg-100_000, Fb)) // ← bank.amount wrong

	waitFor(t, "transaction with fee_mismatch incident", 5*time.Second, func() bool {
		return hasIncidentForTx(t, txID, "fee_mismatch")
	})
}

// ---------------------------------------------------------------------------
// Currency mismatch: I3 violation
// ---------------------------------------------------------------------------

func TestCurrencyMismatch(t *testing.T) {
	txID := uuid.New().String()
	const N int64 = 5_000_000
	const Fg int64 = 25_000
	const Fb int64 = 10_000

	merchant := makeEvent(txID, "merchant", N, 0)
	gateway := makeEvent(txID, "gateway", N, Fg)
	gateway.Currency = "USD" // ← currency mismatch
	bank := makeEvent(txID, "bank", N-Fg, Fb)

	sendEvent(t, "merchant", merchant)
	sendEvent(t, "gateway", gateway)
	sendEvent(t, "bank", bank)

	waitFor(t, "transaction with currency_mismatch incident", 5*time.Second, func() bool {
		return hasIncidentForTx(t, txID, "currency_mismatch")
	})
}

// ---------------------------------------------------------------------------
// Missing source: timeout creates incident
// ---------------------------------------------------------------------------

func TestMissingSource(t *testing.T) {
	txID := uuid.New().String()
	const N int64 = 1_000_000
	const Fg int64 = 5_000

	// Only send merchant and gateway — bank is missing
	sendEvent(t, "merchant", makeEvent(txID, "merchant", N, 0))
	sendEvent(t, "gateway", makeEvent(txID, "gateway", N, Fg))

	// RECONCILER_TIMEOUT_MS is 2000 ms + processing time → wait up to 15s
	waitFor(t, "missing_source incident", 15*time.Second, func() bool {
		return hasIncidentForTx(t, txID, "missing_source")
	})
}

// ---------------------------------------------------------------------------
// Duplicate source detection
// ---------------------------------------------------------------------------

func TestDuplicateSourceDetected(t *testing.T) {
	txID := uuid.New().String()
	const N int64 = 2_000_000
	const Fg int64 = 10_000

	sendEvent(t, "merchant", makeEvent(txID, "merchant", N, 0))
	sendEvent(t, "gateway", makeEvent(txID, "gateway", N, Fg))
	// Send gateway again with different event_id → duplicate source
	sendEvent(t, "gateway", makeEvent(txID, "gateway", N, Fg))
	sendEvent(t, "bank", makeEvent(txID, "bank", N-Fg, 5_000))

	waitFor(t, "duplicate incident", 6*time.Second, func() bool {
		return hasIncidentForTx(t, txID, "duplicate")
	})
}

// ---------------------------------------------------------------------------
// API: transactions list endpoint
// ---------------------------------------------------------------------------

func TestAPITransactionsList(t *testing.T) {
	var resp struct {
		Items []struct {
			TransactionID string `json:"transaction_id"`
			OverallStatus string `json:"overall_status"`
		} `json:"items"`
		TotalEstimate int `json:"total_estimate"`
	}
	getJSON(t, apiURL()+"/api/v1/transactions?limit=10", &resp)
	// Just verify the response shape is correct — items can be empty
}

func TestAPITransactionsFilter(t *testing.T) {
	var resp map[string]interface{}
	getJSON(t, apiURL()+"/api/v1/transactions?status=matched&limit=5", &resp)
	items, ok := resp["items"]
	if !ok {
		t.Fatal("response missing 'items' key")
	}
	if _, ok := items.([]interface{}); !ok {
		t.Fatalf("'items' is not an array: %T", items)
	}
}

// ---------------------------------------------------------------------------
// API: incidents list endpoint
// ---------------------------------------------------------------------------

func TestAPIIncidentsList(t *testing.T) {
	var resp struct {
		Items []struct {
			ID           int64  `json:"id"`
			IncidentType string `json:"incident_type"`
			Severity     int    `json:"severity"`
			Status       string `json:"status"`
		} `json:"items"`
	}
	getJSON(t, apiURL()+"/api/v1/incidents?limit=10", &resp)
}

// ---------------------------------------------------------------------------
// API: ack and resolve incident
// ---------------------------------------------------------------------------

func TestAckAndResolveIncident(t *testing.T) {
	// Create a known incident via a missing-source scenario
	txID := uuid.New().String()
	const N int64 = 500_000
	sendEvent(t, "merchant", makeEvent(txID, "merchant", N, 0))
	// send only merchant, bank and gateway absent → missing_source after timeout

	var incidentID int64
	waitFor(t, "incident created for ack test", 15*time.Second, func() bool {
		id := firstIncidentIDForTx(t, txID)
		if id > 0 {
			incidentID = id
			return true
		}
		return false
	})

	// ACK
	ackStatus := postJSON(t, fmt.Sprintf("%s/api/v1/incidents/%d/ack", apiURL(), incidentID))
	if ackStatus != 200 {
		t.Fatalf("ack returned %d", ackStatus)
	}

	// Verify status changed
	waitFor(t, "incident status = acknowledged", 3*time.Second, func() bool {
		var resp struct {
			Items []struct {
				ID     int64  `json:"id"`
				Status string `json:"status"`
			} `json:"items"`
		}
		getJSON(t, fmt.Sprintf("%s/api/v1/incidents?status=acknowledged&limit=50", apiURL()), &resp)
		for _, inc := range resp.Items {
			if inc.ID == incidentID {
				return true
			}
		}
		return false
	})

	// RESOLVE
	resolveStatus := postJSON(t, fmt.Sprintf("%s/api/v1/incidents/%d/resolve", apiURL(), incidentID))
	if resolveStatus != 200 {
		t.Fatalf("resolve returned %d", resolveStatus)
	}
}

// ---------------------------------------------------------------------------
// API: metrics endpoint
// ---------------------------------------------------------------------------

func TestAPIMetrics(t *testing.T) {
	resp, err := http.Get(apiURL() + "/api/v1/metrics/mismatches-per-minute")
	if err != nil {
		t.Fatalf("GET metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("metrics endpoint returned %d", resp.StatusCode)
	}
	var result []interface{}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("metrics not a JSON array: %v (body: %s)", err, body)
	}
}

// ---------------------------------------------------------------------------
// helpers for incident lookup
// ---------------------------------------------------------------------------

func hasIncidentForTx(t *testing.T, txID, incidentType string) bool {
	t.Helper()
	// Check all statuses
	for _, status := range []string{"open", "acknowledged", "resolved"} {
		url := fmt.Sprintf("%s/api/v1/incidents?status=%s&limit=200", apiURL(), status)
		resp, err := http.Get(url)
		if err != nil || resp.StatusCode != 200 {
			continue
		}
		defer resp.Body.Close()
		var result struct {
			Items []struct {
				TransactionID string `json:"transaction_id"`
				IncidentType  string `json:"incident_type"`
			} `json:"items"`
		}
		body, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(body, &result); err != nil {
			continue
		}
		for _, inc := range result.Items {
			if inc.TransactionID == txID && inc.IncidentType == incidentType {
				return true
			}
		}
	}
	return false
}

func firstIncidentIDForTx(t *testing.T, txID string) int64 {
	t.Helper()
	for _, status := range []string{"open", "acknowledged"} {
		url := fmt.Sprintf("%s/api/v1/incidents?status=%s&limit=200", apiURL(), status)
		resp, err := http.Get(url)
		if err != nil || resp.StatusCode != 200 {
			continue
		}
		defer resp.Body.Close()
		var result struct {
			Items []struct {
				ID            int64  `json:"id"`
				TransactionID string `json:"transaction_id"`
			} `json:"items"`
		}
		body, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(body, &result); err != nil {
			continue
		}
		for _, inc := range result.Items {
			if inc.TransactionID == txID {
				return inc.ID
			}
		}
	}
	return 0
}
