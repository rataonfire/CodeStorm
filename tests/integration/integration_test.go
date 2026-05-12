// Integration tests for the payment reconciliation system.
// Run against a live stack: docker compose --profile test run --rm integration-tests
//
// Target services (configurable via env):
//   INGESTOR_URL  (default: http://localhost:8080)
//   API_GW_URL    (default: http://localhost:8090)
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
// Config helpers
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
// Request helpers
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

func sendEvent(t *testing.T, source string, ev *paymentEvent) int {
	t.Helper()
	ev.Source = source
	data, _ := json.Marshal(ev)
	url := fmt.Sprintf("%s/api/v1/events/%s", ingestorURL(), source)
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func waitFor(t *testing.T, label string, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(250 * time.Millisecond)
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
		t.Fatalf("GET %s → %d: %s", url, resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("unmarshal from %s: %v (body: %s)", url, err, body)
	}
}

func postEmpty(t *testing.T, url string) int {
	t.Helper()
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// ---------------------------------------------------------------------------
// Incident query helpers
// ---------------------------------------------------------------------------

func hasIncidentForTx(t *testing.T, txID, incidentType string) bool {
	t.Helper()
	for _, status := range []string{"open", "acknowledged", "resolved"} {
		url := fmt.Sprintf("%s/api/v1/incidents?status=%s&limit=200", apiURL(), status)
		resp, err := http.Get(url)
		if err != nil || resp.StatusCode != 200 {
			if resp != nil {
				resp.Body.Close()
			}
			continue
		}
		var result struct {
			Items []struct {
				TransactionID string `json:"transaction_id"`
				IncidentType  string `json:"incident_type"`
			} `json:"items"`
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
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
			if resp != nil {
				resp.Body.Close()
			}
			continue
		}
		var result struct {
			Items []struct {
				ID            int64  `json:"id"`
				TransactionID string `json:"transaction_id"`
			} `json:"items"`
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
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

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestServicesReady checks that healthz and readyz endpoints respond 200.
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

// TestIngestorValidation verifies that the ingestor rejects invalid payloads.
func TestIngestorValidation(t *testing.T) {
	now := fmt.Sprint(time.Now().UnixMilli())
	tests := []struct {
		name       string
		source     string
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

// TestIngestorAcceptsValidEvent sends a single valid merchant event and expects 202.
func TestIngestorAcceptsValidEvent(t *testing.T) {
	ev := makeEvent(uuid.New().String(), "merchant", 5_000_000, 0)
	if s := sendEvent(t, "merchant", ev); s != 202 {
		t.Fatalf("expected 202, got %d", s)
	}
}

// TestIdempotency sends the same event_id twice; second call must be 202 or 409.
func TestIdempotency(t *testing.T) {
	txID := uuid.New().String()
	ev := makeEvent(txID, "merchant", 5_000_000, 0)
	eventID := ev.EventID

	if s := sendEvent(t, "merchant", ev); s != 202 {
		t.Fatalf("first send expected 202, got %d", s)
	}

	ev2 := *ev
	ev2.EventID = eventID
	s2 := sendEvent(t, "merchant", &ev2)
	if s2 != 202 && s2 != 409 {
		t.Fatalf("second send expected 202 or 409, got %d", s2)
	}
}

// TestHappyPathMatched sends all 3 correct sources and expects overall_status = "matched".
// Invariants: I1 gateway.amount==merchant.amount, I2 bank.amount==gateway.amount-gateway.fee
func TestHappyPathMatched(t *testing.T) {
	txID := uuid.New().String()
	const N int64 = 5_000_000
	const Fg int64 = 25_000

	if s := sendEvent(t, "merchant", makeEvent(txID, "merchant", N, 0)); s != 202 {
		t.Fatalf("merchant: got %d", s)
	}
	if s := sendEvent(t, "gateway", makeEvent(txID, "gateway", N, Fg)); s != 202 {
		t.Fatalf("gateway: got %d", s)
	}
	if s := sendEvent(t, "bank", makeEvent(txID, "bank", N-Fg, 10_000)); s != 202 {
		t.Fatalf("bank: got %d", s)
	}

	waitFor(t, "transaction matched", 5*time.Second, func() bool {
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

// TestAmountMismatch violates I1 (gateway.amount != merchant.amount) → amount_mismatch incident.
func TestAmountMismatch(t *testing.T) {
	txID := uuid.New().String()
	const N int64 = 5_000_000
	const Fg int64 = 25_000

	sendEvent(t, "merchant", makeEvent(txID, "merchant", N, 0))
	sendEvent(t, "gateway", makeEvent(txID, "gateway", N+100_000, Fg)) // I1 violation
	sendEvent(t, "bank", makeEvent(txID, "bank", N+100_000-Fg, 10_000))

	waitFor(t, "amount_mismatch incident", 5*time.Second, func() bool {
		return hasIncidentForTx(t, txID, "amount_mismatch")
	})
}

// TestFeeMismatch violates I2 (bank.amount != gateway.amount - gateway.fee) → fee_mismatch incident.
func TestFeeMismatch(t *testing.T) {
	txID := uuid.New().String()
	const N int64 = 5_000_000
	const Fg int64 = 25_000

	sendEvent(t, "merchant", makeEvent(txID, "merchant", N, 0))
	sendEvent(t, "gateway", makeEvent(txID, "gateway", N, Fg))
	sendEvent(t, "bank", makeEvent(txID, "bank", N-Fg-100_000, 10_000)) // I2 violation

	waitFor(t, "fee_mismatch incident", 5*time.Second, func() bool {
		return hasIncidentForTx(t, txID, "fee_mismatch")
	})
}

// TestCurrencyMismatch violates I3 (currencies differ) → currency_mismatch incident.
func TestCurrencyMismatch(t *testing.T) {
	txID := uuid.New().String()
	const N int64 = 5_000_000
	const Fg int64 = 25_000

	merchant := makeEvent(txID, "merchant", N, 0)
	gateway := makeEvent(txID, "gateway", N, Fg)
	gateway.Currency = "USD" // I3 violation
	bank := makeEvent(txID, "bank", N-Fg, 10_000)

	sendEvent(t, "merchant", merchant)
	sendEvent(t, "gateway", gateway)
	sendEvent(t, "bank", bank)

	waitFor(t, "currency_mismatch incident", 5*time.Second, func() bool {
		return hasIncidentForTx(t, txID, "currency_mismatch")
	})
}

// TestMissingSource sends only 2 of 3 sources; reconciler must time out and raise missing_source.
func TestMissingSource(t *testing.T) {
	txID := uuid.New().String()

	sendEvent(t, "merchant", makeEvent(txID, "merchant", 1_000_000, 0))
	sendEvent(t, "gateway", makeEvent(txID, "gateway", 1_000_000, 5_000))
	// bank is intentionally not sent

	// RECONCILER_TIMEOUT_MS=2000ms; allow extra time for timer processing under load
	waitFor(t, "missing_source incident", 15*time.Second, func() bool {
		return hasIncidentForTx(t, txID, "missing_source")
	})
}

// TestDuplicateSourceDetected sends gateway twice with different event_ids → duplicate incident.
func TestDuplicateSourceDetected(t *testing.T) {
	txID := uuid.New().String()
	const N int64 = 2_000_000
	const Fg int64 = 10_000

	sendEvent(t, "merchant", makeEvent(txID, "merchant", N, 0))
	sendEvent(t, "gateway", makeEvent(txID, "gateway", N, Fg))
	sendEvent(t, "gateway", makeEvent(txID, "gateway", N, Fg)) // duplicate source
	sendEvent(t, "bank", makeEvent(txID, "bank", N-Fg, 5_000))

	waitFor(t, "duplicate incident", 6*time.Second, func() bool {
		return hasIncidentForTx(t, txID, "duplicate")
	})
}

// ---------------------------------------------------------------------------
// API endpoint tests
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

func TestAPIIncidentsList(t *testing.T) {
	var resp struct {
		Items []struct {
			ID           int64  `json:"id"`
			IncidentType string `json:"incident_type"`
			Status       string `json:"status"`
		} `json:"items"`
	}
	getJSON(t, apiURL()+"/api/v1/incidents?limit=10", &resp)
}

// TestAckAndResolveIncident creates a missing-source incident then acks and resolves it.
func TestAckAndResolveIncident(t *testing.T) {
	txID := uuid.New().String()
	sendEvent(t, "merchant", makeEvent(txID, "merchant", 500_000, 0))
	// only merchant sent → missing_source after timeout

	var incidentID int64
	waitFor(t, "incident created", 15*time.Second, func() bool {
		id := firstIncidentIDForTx(t, txID)
		if id > 0 {
			incidentID = id
			return true
		}
		return false
	})

	if s := postEmpty(t, fmt.Sprintf("%s/api/v1/incidents/%d/ack", apiURL(), incidentID)); s != 200 {
		t.Fatalf("ack returned %d", s)
	}

	waitFor(t, "incident acknowledged", 3*time.Second, func() bool {
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

	if s := postEmpty(t, fmt.Sprintf("%s/api/v1/incidents/%d/resolve", apiURL(), incidentID)); s != 200 {
		t.Fatalf("resolve returned %d", s)
	}
}

func TestAPIMetrics(t *testing.T) {
	resp, err := http.Get(apiURL() + "/api/v1/metrics/mismatches-per-minute")
	if err != nil {
		t.Fatalf("GET metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("metrics returned %d", resp.StatusCode)
	}
	var result []interface{}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("metrics not a JSON array: %v (body: %s)", err, body)
	}
}
