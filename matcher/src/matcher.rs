use crate::config::Config;
use crate::model::{
    build_details, check_invariants, Incident, IncidentKind, PartialTransaction,
    PubsubEvent, ReconciliationResult, Source, StreamMessage,
};
use crate::publisher::EventPublisher;
use crate::store::{PostgresStore, RedisStore};
use anyhow::Result;
use chrono::Utc;
use dashmap::DashMap;
use std::sync::Arc;
use tracing::{debug, error, info, instrument, warn};
use uuid::Uuid;

pub struct MatchingEngine {
    config: Config,
    pg: PostgresStore,
    redis: RedisStore,
    publisher: Arc<EventPublisher>,
    windows: DashMap<Uuid, PartialTransaction>,
}

impl MatchingEngine {
    pub fn new(
        config: Config,
        pg: PostgresStore,
        redis: RedisStore,
        publisher: Arc<EventPublisher>,
    ) -> Self {
        Self {
            config,
            pg,
            redis,
            publisher,
            windows: DashMap::new(),
        }
    }

    /// Восстановить state из Redis при старте (для отказоустойчивости).
    pub async fn restore_from_redis(&self) -> Result<()> {
        let windows = self.redis.load_all_windows().await?;
        let count = windows.len();
        for w in windows {
            self.windows.insert(w.transaction_id, w);
        }
        info!(restored_windows = count, "state restored from redis checkpoint");
        Ok(())
    }

    /// Обработать одно сообщение из Stream.
    /// Возвращает Ok(()) если обработка успешна — стрим может XACK.
    /// Возвращает Err — сообщение НЕ ACKается, будет переотправлено.
    #[instrument(skip(self, msg), fields(
        transaction_id = %msg.event.transaction_id,
        source = %msg.event.source,
        event_id = %msg.event.event_id
    ))]
    pub async fn handle_event(&self, msg: StreamMessage) -> Result<()> {
        let event = msg.event;

        // 1. Дедупликация по event_id (защита от ретраев)
        let is_new = self
            .redis
            .try_claim_event(event.event_id, self.config.reconciler_dedup_window_ms)
            .await?;
        if !is_new {
            debug!("duplicate event_id, ignoring retry");
            return Ok(());
        }

        let tx_id = event.transaction_id;
        let now_ms = Utc::now().timestamp_millis();

        // 2. Регистрация источника в Redis (атомарная проверка дубля источника)
        let first_time_source = self.redis.try_register_source(tx_id, event.source).await?;

        if !first_time_source {
            // Этот источник уже присылал событие для этой транзакции — дубль
            warn!("duplicate source for transaction");
            let incident = Incident {
                transaction_id: tx_id,
                incident_type: IncidentKind::Duplicate,
                severity: 1,
                description: format!(
                    "Источник {} прислал второе событие для транзакции {}",
                    event.source, tx_id
                ),
            };
            self.persist_incident(&incident).await?;
            return Ok(());
        }

        // 3. Получить или создать окно
        let mut window = self
            .windows
            .entry(tx_id)
            .or_insert_with(|| {
                debug!("opening new reconciliation window");
                PartialTransaction::new(tx_id, now_ms, self.config.reconciler_timeout_ms)
            })
            .clone();

        let is_first_event = window.events.is_empty();

        // 4. Добавить событие в окно
        window.events.insert(event.source, event.clone());

        // Если это первое событие — публикуем "received" и регистрируем дедлайн
        if is_first_event {
            self.redis.add_deadline(tx_id, window.deadline_ms).await?;
            self.publisher
                .publish_event(PubsubEvent::TransactionReceived {
                    transaction_id: tx_id,
                    source: event.source,
                    ts_ms: now_ms,
                })
                .await?;
        }

        // 5. Сохранить в память и Redis checkpoint
        self.windows.insert(tx_id, window.clone());
        self.redis.save_window(&window).await?;

        // 6. Проверка комплектности
        if window.is_complete(self.config.expected_sources_count) {
            self.finalize_window(window).await?;
        } else {
            self.publisher
                .publish_event(PubsubEvent::TransactionProgress {
                    transaction_id: tx_id,
                    sources_seen: window.sources_seen(),
                    ts_ms: now_ms,
                })
                .await?;
        }

        Ok(())
    }

    /// Завершить окно: проверить инварианты, записать в БД, опубликовать результат, очистить.
    #[instrument(skip(self, window), fields(transaction_id = %window.transaction_id))]
    async fn finalize_window(&self, window: PartialTransaction) -> Result<()> {
        let tx_id = window.transaction_id;
        let now_ms = Utc::now().timestamp_millis();
        let duration_ms = now_ms - window.first_seen_at_ms;

        let merchant = window.events.get(&Source::Merchant);
        let currency = merchant
            .map(|e| e.currency.clone())
            .or_else(|| window.events.values().next().map(|e| e.currency.clone()))
            .unwrap_or_else(|| "UZS".to_string());
        let tx_type = merchant
            .map(|e| e.tx_type.clone())
            .or_else(|| window.events.values().next().map(|e| e.tx_type.clone()))
            .unwrap_or_else(|| "card".to_string());
        let merchant_id = merchant.and_then(|e| e.merchant_id.clone());

        let result = check_invariants(&window);
        let details = build_details(&window);

        let overall_status = match &result {
            ReconciliationResult::Matched => "matched",
            ReconciliationResult::Mismatched(_) => "mismatch",
        };

        self.pg
            .upsert_transaction(tx_id, overall_status, &currency, &tx_type, merchant_id.as_deref())
            .await?;
        self.pg.upsert_reconciliation_details(tx_id, &details).await?;

        match result {
            ReconciliationResult::Matched => {
                info!(duration_ms, "transaction matched");
                self.publisher
                    .publish_event(PubsubEvent::TransactionMatched {
                        transaction_id: tx_id,
                        ts_ms: now_ms,
                    })
                    .await?;
            }
            ReconciliationResult::Mismatched(incidents) => {
                info!(
                    duration_ms,
                    incidents_count = incidents.len(),
                    "transaction has mismatches"
                );
                for incident in incidents {
                    self.persist_incident(&incident).await?;
                }
            }
        }

        self.cleanup_window(tx_id).await?;
        Ok(())
    }

    /// Обработать истёкшее окно (вызывается из timer worker).
    #[instrument(skip(self), fields(transaction_id = %tx_id))]
    pub async fn handle_timeout(&self, tx_id: Uuid) -> Result<()> {
        // Забираем окно из памяти
        let window = match self.windows.remove(&tx_id) {
            Some((_, w)) => w,
            None => {
                // Уже обработано (race condition между finalize и timer)
                debug!("window already cleaned up, ignoring timeout");
                return Ok(());
            }
        };

        // Если по факту все источники пришли — финализируем как обычно
        if window.is_complete(self.config.expected_sources_count) {
            self.finalize_window(window).await?;
            return Ok(());
        }

        // Иначе — инцидент missing_source
        let missing = window.missing_sources();
        warn!(missing = ?missing, "timeout expired with incomplete window");

        let currency = window
            .events
            .values()
            .next()
            .map(|e| e.currency.clone())
            .unwrap_or_else(|| "UZS".to_string());
        let tx_type = window
            .events
            .values()
            .next()
            .map(|e| e.tx_type.clone())
            .unwrap_or_else(|| "card".to_string());
        let merchant_id = window
            .events
            .get(&Source::Merchant)
            .and_then(|e| e.merchant_id.clone());

        self.pg
            .upsert_transaction(tx_id, "degraded", &currency, &tx_type, merchant_id.as_deref())
            .await?;

        let details = build_details(&window);
        self.pg.upsert_reconciliation_details(tx_id, &details).await?;

        let missing_str: Vec<String> = missing.iter().map(|s| s.to_string()).collect();
        let incident = Incident {
            transaction_id: tx_id,
            incident_type: IncidentKind::MissingSource,
            severity: 1,
            description: format!(
                "Не получены события от источников: {} (окно {} мс)",
                missing_str.join(", "),
                self.config.reconciler_timeout_ms
            ),
        };
        self.persist_incident(&incident).await?;

        self.cleanup_window(tx_id).await?;
        Ok(())
    }

    /// Проверить и обработать все истёкшие дедлайны в Redis.
    pub async fn process_expired_windows(&self) -> Result<()> {
        let now_ms = Utc::now().timestamp_millis();
        let expired_ids = self.redis.pop_expired_deadlines(now_ms).await?;

        for tx_id in expired_ids {
            if let Err(e) = self.handle_timeout(tx_id).await {
                error!(transaction_id = %tx_id, error = %e, "failed to handle timeout");
            }
        }
        Ok(())
    }

    /// Проверить и обработать все истёкшие эскалации инцидентов.
    pub async fn process_expired_escalations(&self) -> Result<()> {
        let now_ms = Utc::now().timestamp_millis();
        let expired_incidents = self.redis.pop_expired_escalations(now_ms).await?;

        for incident_id in expired_incidents {
            info!(incident_id, "escalating incident");
            let escalated = self.pg.escalate_incident(incident_id, 2).await?;
            if escalated {
                self.publisher
                    .publish_event(PubsubEvent::IncidentUpdated {
                        incident_id,
                        new_status: None, // Status stays 'open' during escalation
                        new_severity: 2,
                        ts_ms: now_ms,
                    })
                    .await?;
                info!(incident_id, "incident severity increased to 2");
            }
        }
        Ok(())
    }

    /// Создать инцидент в БД и опубликовать событие.
    async fn persist_incident(&self, incident: &Incident) -> Result<()> {
        let id = self.pg.create_incident(incident).await?;
        let now_ms = Utc::now().timestamp_millis();

        // Добавляем в таймер эскалации (severity 1 -> 2 через 10 секунд)
        if incident.severity == 1 {
            let escalate_at = now_ms + self.config.reconciler_escalation_ms as i64;
            self.redis.add_escalation(id, escalate_at).await?;
        }

        self.publisher
            .publish_event(PubsubEvent::IncidentCreated {
                incident_id: id,
                transaction_id: incident.transaction_id,
                incident_type: incident.incident_type.to_string(),
                severity: incident.severity,
                description: incident.description.clone(),
                ts_ms: now_ms,
            })
            .await?;
        info!(incident_id = id, kind = %incident.incident_type, "incident created");
        Ok(())
    }

    /// Очистить состояние транзакции из памяти и Redis.
    async fn cleanup_window(&self, tx_id: Uuid) -> Result<()> {
        self.windows.remove(&tx_id);
        self.redis.delete_window(tx_id).await?;
        self.redis.remove_deadline(tx_id).await?;
        self.redis.cleanup_sources(tx_id).await?;
        Ok(())
    }

    /// Получить текущее количество активных окон (для метрик и /debug).
    pub fn active_windows_count(&self) -> usize {
        self.windows.len()
    }
    }

    #[cfg(test)]
    mod tests {
    use super::*;
    // Unit tests for MatchingEngine would require mocking PostgresStore and RedisStore.
    // In a real project, we would use traits and mockall crate.
    // For this MVP, core logic is tested in model.rs.
    }