package emulatorcommon

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
