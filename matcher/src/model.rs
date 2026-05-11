use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fmt;
use uuid::Uuid;

pub enum Source {
    Merchant,
    Gateway,
    Bank,
}

impl fmt::Display for Source {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Source::Merchant => write!(f, "merchant"),
            Source::Gateway => write!(f, "gateway"),
            Source::Bank => write!(f, "bank"),
        }
    }
}

impl Source {
    pub fn all() -> [Source; 3] {
        [Source::Merchant, Source::Gateway, Source::Bank]
    }
}

pub struct PaymentEvent {
    pub event_id: Uuid,
    pub transaction_id: Uuid,
    pub source: Source,
    pub amount_minor: i64,
    pub fee_minor: i64,
    pub currency: String,
    pub timestamp_ms: i64,
    pub tx_type: String,
    #[serde(default)]
    pub merchant_id: Option<String>,
}


pub struct StreamMessage {
    pub event: PaymentEvent,
    pub internal_event_id: Uuid,
    #[serde(default)]
    pub correlation_id: Option<Uuid>,
    pub ingested_at_ms: i64,
}

pub struct PartialTransaction {
    pub transaction_id: Uuid,
    pub events: HashMap<Source, PaymentEvent>,
    pub first_seen_at_ms: i64,
    pub deadline_ms: i64,
}

impl PartialTransaction {
    pub fn new(transaction_id: Uuid, first_seen_at_ms: i64, timeout_ms: u64) -> Self {
        Self {
            transaction_id,
            events: HashMap::with_capacity(3),
            first_seen_at_ms,
            deadline_ms: first_seen_at_ms + timeout_ms as i64,
        }
    }

    pub fn is_complete(&self, expected: usize) -> bool {
        self.events.len() >= expected
    }

    pub fn sources_seen(&self) -> Vec<Source> {
        let mut v: Vec<Source> = self.events.keys().copied().collect();
        v.sort_by_key(|s| s.to_string());
        v
    }

    pub fn missing_sources(&self) -> Vec<Source> {
        Source::all()
            .iter()
            .filter(|s| !self.events.contains_key(s))
            .copied()
            .collect()
    }
}

pub enum IncidentKind {
    AmountMismatch,
    FeeMismatch,
    CurrencyMismatch,
    MissingSource,
    Duplicate,
    SourceOffline,
}

impl fmt::Display for IncidentKind {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            IncidentKind::AmountMismatch => write!(f, "amount_mismatch"),
            IncidentKind::FeeMismatch => write!(f, "fee_mismatch"),
            IncidentKind::CurrencyMismatch => write!(f, "currency_mismatch"),
            IncidentKind::MissingSource => write!(f, "missing_source"),
            IncidentKind::Duplicate => write!(f, "duplicate"),
            IncidentKind::SourceOffline => write!(f, "source_offline"),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Incident {
    pub transaction_id: Uuid,
    pub incident_type: IncidentKind,
    pub severity: i16,
    pub description: String,
}

/// Результат проверки инвариантов: либо matched, либо набор инцидентов.
#[derive(Debug, Clone)]
pub enum ReconciliationResult {
    Matched,
    Mismatched(Vec<Incident>),
}

/// Детали по каждому источнику для записи в reconciliation_details.
#[derive(Debug, Clone)]
pub struct ReconciliationDetail {
    pub source: Source,
    pub amount_expected: Option<i64>,
    pub amount_actual: Option<i64>,
    pub fee_expected: Option<i64>,
    pub fee_actual: Option<i64>,
    pub is_matched: bool,
    pub mismatch_reason: Option<String>,
}

pub fn check_invariants(partial: &PartialTransaction) -> ReconciliationResult {
    let merchant = match partial.events.get(&Source::Merchant) {
        Some(e) => e,
        None => return ReconciliationResult::Mismatched(vec![]),
    };
    let gateway = match partial.events.get(&Source::Gateway) {
        Some(e) => e,
        None => return ReconciliationResult::Mismatched(vec![]),
    };
    let bank = match partial.events.get(&Source::Bank) {
        Some(e) => e,
        None => return ReconciliationResult::Mismatched(vec![]),
    };

    let mut incidents = Vec::new();

    // I3: валюта одинакова
    if !(merchant.currency == gateway.currency && gateway.currency == bank.currency) {
        incidents.push(Incident {
            transaction_id: partial.transaction_id,
            incident_type: IncidentKind::CurrencyMismatch,
            severity: 1,
            description: format!(
                "Валюты не совпадают: merchant={}, gateway={}, bank={}",
                merchant.currency, gateway.currency, bank.currency
            ),
        });
        // Если валюты разные, дальше проверять суммы бессмысленно.
        return ReconciliationResult::Mismatched(incidents);
    }

    // I1: gateway.amount == merchant.amount
    if gateway.amount_minor != merchant.amount_minor {
        incidents.push(Incident {
            transaction_id: partial.transaction_id,
            incident_type: IncidentKind::AmountMismatch,
            severity: 1,
            description: format!(
                "Сумма расходится: merchant={}, gateway={} (разница {})",
                merchant.amount_minor,
                gateway.amount_minor,
                gateway.amount_minor - merchant.amount_minor
            ),
        });
    }

    // I2: bank.amount == gateway.amount - gateway.fee
    let expected_bank_amount = gateway.amount_minor - gateway.fee_minor;
    if bank.amount_minor != expected_bank_amount {
        incidents.push(Incident {
            transaction_id: partial.transaction_id,
            incident_type: IncidentKind::FeeMismatch,
            severity: 1,
            description: format!(
                "Комиссия шлюза не сходится: gateway.amount={}, gateway.fee={}, ожидается bank.amount={}, фактически bank.amount={}",
                gateway.amount_minor, gateway.fee_minor, expected_bank_amount, bank.amount_minor
            ),
        });
    }

    if incidents.is_empty() {
        ReconciliationResult::Matched
    } else {
        ReconciliationResult::Mismatched(incidents)
    }
}

pub fn build_details(partial: &PartialTransaction) -> Vec<ReconciliationDetail> {
    let merchant = partial.events.get(&Source::Merchant);
    let gateway = partial.events.get(&Source::Gateway);
    let bank = partial.events.get(&Source::Bank);

    let mut details = Vec::new();

    let currency_mismatch = merchant
        .and_then(|m| gateway.map(|g| m.currency != g.currency))
        .unwrap_or(false)
        || gateway
            .and_then(|g| bank.map(|b| g.currency != b.currency))
            .unwrap_or(false)
        || merchant
            .and_then(|m| bank.map(|b| m.currency != b.currency))
            .unwrap_or(false);

    let amount_mismatch = merchant
        .and_then(|m| gateway.map(|g| g.amount_minor != m.amount_minor))
        .unwrap_or(false);

    let fee_mismatch = gateway
        .and_then(|g| bank.map(|b| b.amount_minor != g.amount_minor - g.fee_minor))
        .unwrap_or(false);

    if let Some(m) = merchant {
        details.push(ReconciliationDetail {
            source: Source::Merchant,
            amount_expected: gateway.map(|g| g.amount_minor),
            amount_actual: Some(m.amount_minor),
            fee_expected: Some(0),
            fee_actual: Some(m.fee_minor),
            is_matched: gateway.map(|g| g.amount_minor == m.amount_minor).unwrap_or(false),
            mismatch_reason: if currency_mismatch {
                Some("currency mismatch".to_string())
            } else if amount_mismatch {
                Some("merchant amount mismatch".to_string())
            } else {
                None
            },
        });
    }

    if let Some(g) = gateway {
        details.push(ReconciliationDetail {
            source: Source::Gateway,
            amount_expected: merchant.map(|m| m.amount_minor),
            amount_actual: Some(g.amount_minor),
            fee_expected: None,
            fee_actual: Some(g.fee_minor),
            is_matched: merchant.map(|m| m.amount_minor == g.amount_minor).unwrap_or(false),
            mismatch_reason: if currency_mismatch {
                Some("currency mismatch".to_string())
            } else if amount_mismatch || fee_mismatch {
                Some("gateway consistency mismatch".to_string())
            } else {
                None
            },
        });
    }

    if let Some(b) = bank {
        let expected = gateway.map(|g| g.amount_minor - g.fee_minor);
        details.push(ReconciliationDetail {
            source: Source::Bank,
            amount_expected: expected,
            amount_actual: Some(b.amount_minor),
            fee_expected: None,
            fee_actual: Some(b.fee_minor),
            is_matched: expected.map(|e| e == b.amount_minor).unwrap_or(false),
            mismatch_reason: if currency_mismatch {
                Some("currency mismatch".to_string())
            } else if fee_mismatch {
                Some("bank amount mismatch".to_string())
            } else {
                None
            },
        });
    }

    details
}

/// Событие для публикации в Redis Pub/Sub.
pub enum PubsubEvent {
    TransactionReceived {
        transaction_id: Uuid,
        source: Source,
        ts_ms: i64,
    },
    TransactionProgress {
        transaction_id: Uuid,
        sources_seen: Vec<Source>,
        ts_ms: i64,
    },
    TransactionMatched {
        transaction_id: Uuid,
        ts_ms: i64,
    },
    IncidentCreated {
        incident_id: i64,
        transaction_id: Uuid,
        incident_type: String,
        severity: i16,
        description: String,
        ts_ms: i64,
    },
    IncidentUpdated {
        incident_id: i64,
        new_status: Option<String>,
        new_severity: i16,
        ts_ms: i64,
    },
    SourceStatusChanged {
        source: Source,
        is_online: bool,
        ts_ms: i64,
    },
}

#[cfg(test)]
mod tests {
    use super::*;

    fn make_event(source: Source, amount: i64, fee: i64) -> PaymentEvent {
        PaymentEvent {
            event_id: Uuid::new_v4(),
            transaction_id: Uuid::new_v4(),
            source,
            amount_minor: amount,
            fee_minor: fee,
            currency: "UZS".to_string(),
            timestamp_ms: 1735689600000,
            tx_type: "card".to_string(),
            merchant_id: None,
        }
    }

    #[test]
    fn invariants_pass_on_correct_flow() {
        // N=5_000_000 UZS, gateway комиссия 25_000, bank комиссия 10_000
        let mut p = PartialTransaction::new(Uuid::new_v4(), 0, 2000);
        p.events.insert(Source::Merchant, make_event(Source::Merchant, 5_000_000, 0));
        p.events.insert(Source::Gateway, make_event(Source::Gateway, 5_000_000, 25_000));
        p.events.insert(Source::Bank, make_event(Source::Bank, 4_975_000, 10_000));

        match check_invariants(&p) {
            ReconciliationResult::Matched => {}
            ReconciliationResult::Mismatched(i) => panic!("expected matched, got incidents: {:?}", i),
        }
    }

    #[test]
    fn invariants_detect_amount_mismatch() {
        let mut p = PartialTransaction::new(Uuid::new_v4(), 0, 2000);
        p.events.insert(Source::Merchant, make_event(Source::Merchant, 5_000_000, 0));
        p.events.insert(Source::Gateway, make_event(Source::Gateway, 5_100_000, 25_000)); // !!! wrong
        p.events.insert(Source::Bank, make_event(Source::Bank, 5_075_000, 10_000));

        match check_invariants(&p) {
            ReconciliationResult::Mismatched(incs) => {
                assert!(incs.iter().any(|i| i.incident_type == IncidentKind::AmountMismatch));
            }
            ReconciliationResult::Matched => panic!("should detect amount mismatch"),
        }
    }

    #[test]
    fn invariants_detect_fee_mismatch() {
        let mut p = PartialTransaction::new(Uuid::new_v4(), 0, 2000);
        p.events.insert(Source::Merchant, make_event(Source::Merchant, 5_000_000, 0));
        p.events.insert(Source::Gateway, make_event(Source::Gateway, 5_000_000, 25_000));
        // bank.amount должен быть 4_975_000, а пришло 4_900_000
        p.events.insert(Source::Bank, make_event(Source::Bank, 4_900_000, 10_000));

        match check_invariants(&p) {
            ReconciliationResult::Mismatched(incs) => {
                assert!(incs.iter().any(|i| i.incident_type == IncidentKind::FeeMismatch));
            }
            ReconciliationResult::Matched => panic!("should detect fee mismatch"),
        }
    }

    #[test]
    fn invariants_detect_currency_mismatch() {
        let mut p = PartialTransaction::new(Uuid::new_v4(), 0, 2000);
        let mut g = make_event(Source::Gateway, 5_000_000, 25_000);
        g.currency = "USD".to_string();
        p.events.insert(Source::Merchant, make_event(Source::Merchant, 5_000_000, 0));
        p.events.insert(Source::Gateway, g);
        p.events.insert(Source::Bank, make_event(Source::Bank, 4_975_000, 10_000));

        match check_invariants(&p) {
            ReconciliationResult::Mismatched(incs) => {
                assert!(incs.iter().any(|i| i.incident_type == IncidentKind::CurrencyMismatch));
            }
            ReconciliationResult::Matched => panic!("should detect currency mismatch"),
        }
    }

    #[test]
    fn missing_sources_reports_correctly() {
        let mut p = PartialTransaction::new(Uuid::new_v4(), 0, 2000);
        p.events.insert(Source::Merchant, make_event(Source::Merchant, 5_000_000, 0));
        let missing = p.missing_sources();
        assert_eq!(missing.len(), 2);
        assert!(missing.contains(&Source::Gateway));
        assert!(missing.contains(&Source::Bank));
    }

    #[test]
    fn build_details_populates_correctly() {
        let mut p = PartialTransaction::new(Uuid::new_v4(), 0, 2000);
        p.events.insert(Source::Merchant, make_event(Source::Merchant, 5_000_000, 0));
        p.events.insert(Source::Gateway, make_event(Source::Gateway, 5_000_000, 25_000));

        let details = build_details(&p);
        assert_eq!(details.len(), 2);

        let merchant_detail = details.iter().find(|d| d.source == Source::Merchant).unwrap();
        assert_eq!(merchant_detail.amount_actual, Some(5_000_000));
        assert_eq!(merchant_detail.amount_expected, Some(5_000_000));
        assert!(merchant_detail.is_matched);
        assert!(merchant_detail.mismatch_reason.is_none());

        let gateway_detail = details.iter().find(|d| d.source == Source::Gateway).unwrap();
        assert_eq!(gateway_detail.amount_actual, Some(5_000_000));
        assert_eq!(gateway_detail.amount_expected, Some(5_000_000));
        assert!(gateway_detail.is_matched);
        assert!(gateway_detail.mismatch_reason.is_none());
    }

    #[test]
    fn build_details_includes_mismatch_reason() {
        let mut p = PartialTransaction::new(Uuid::new_v4(), 0, 2000);
        p.events.insert(Source::Merchant, make_event(Source::Merchant, 5_000_000, 0));
        p.events.insert(Source::Gateway, make_event(Source::Gateway, 5_100_000, 25_000));
        p.events.insert(Source::Bank, make_event(Source::Bank, 5_075_000, 10_000));

        let details = build_details(&p);
        let merchant_detail = details.iter().find(|d| d.source == Source::Merchant).unwrap();
        assert_eq!(merchant_detail.mismatch_reason.as_deref(), Some("merchant amount mismatch"));

        let gateway_detail = details.iter().find(|d| d.source == Source::Gateway).unwrap();
        assert_eq!(gateway_detail.mismatch_reason.as_deref(), Some("gateway consistency mismatch"));
    }

    #[test]
    fn pubsub_event_source_status_changed_serializes() {
        let event = PubsubEvent::SourceStatusChanged {
            source: Source::Bank,
            is_online: false,
            ts_ms: 1_735_689_600_000,
        };

        let json = serde_json::to_string(&event).unwrap();
        assert!(json.contains("\"type\":\"source_status_changed\""));
        assert!(json.contains("\"source\":\"bank\""));
        assert!(json.contains("\"is_online\":false"));
    }
}
