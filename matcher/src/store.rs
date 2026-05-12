use crate::error::Result;
use crate::model::{Incident, PartialTransaction, ReconciliationDetail, Source};
use redis::aio::ConnectionManager;
use redis::AsyncCommands;
use sqlx::postgres::{PgPool, PgPoolOptions};
use std::time::Duration;
use uuid::Uuid;

#[derive(Clone)]
pub struct PostgresStore {
    pool: PgPool,
}

impl PostgresStore {
    pub async fn connect(url: &str) -> Result<Self> {
        let pool = PgPoolOptions::new()
            .max_connections(10)
            .acquire_timeout(Duration::from_secs(5))
            .connect(url)
            .await?;
        Ok(Self { pool })
    }

    pub fn pool(&self) -> &PgPool {
        &self.pool
    }

    pub async fn upsert_transaction(
        &self,
        transaction_id: Uuid,
        overall_status: &str,
        currency: &str,
        tx_type: &str,
        merchant_id: Option<&str>,
    ) -> Result<()> {
        sqlx::query(
            r#"
            INSERT INTO transactions (transaction_id, overall_status, currency, tx_type, merchant_id, created_at, updated_at)
            VALUES ($1, $2::overall_status, $3, $4, $5, NOW(), NOW())
            ON CONFLICT (transaction_id) DO UPDATE SET
                overall_status = EXCLUDED.overall_status,
                updated_at = NOW()
            "#,
        )
        .bind(transaction_id)
        .bind(overall_status)
        .bind(currency)
        .bind(tx_type)
        .bind(merchant_id)
        .execute(&self.pool)
        .await?;
        Ok(())
    }

    pub async fn upsert_reconciliation_details(
        &self,
        transaction_id: Uuid,
        details: &[ReconciliationDetail],
    ) -> Result<()> {
        for d in details {
            sqlx::query(
                r#"
                INSERT INTO reconciliation_details (
                    transaction_id, source, amount_expected, amount_actual,
                    fee_expected, fee_actual, is_matched, mismatch_reason, received_at
                )
                VALUES ($1, $2::source_kind, $3, $4, $5, $6, $7, $8, NOW())
                ON CONFLICT (transaction_id, source) DO UPDATE SET
                    amount_expected = EXCLUDED.amount_expected,
                    amount_actual = EXCLUDED.amount_actual,
                    fee_expected = EXCLUDED.fee_expected,
                    fee_actual = EXCLUDED.fee_actual,
                    is_matched = EXCLUDED.is_matched,
                    mismatch_reason = EXCLUDED.mismatch_reason,
                    received_at = NOW()
                "#,
            )
            .bind(transaction_id)
            .bind(d.source.to_string())
            .bind(d.amount_expected)
            .bind(d.amount_actual)
            .bind(d.fee_expected)
            .bind(d.fee_actual)
            .bind(d.is_matched)
            .bind(&d.mismatch_reason)
            .execute(&self.pool)
            .await?;
        }
        Ok(())
    }

    pub async fn create_incident(&self, incident: &Incident) -> Result<i64> {
        let rec: (i64,) = sqlx::query_as(
            r#"
            INSERT INTO incidents (transaction_id, incident_type, severity, description, status, created_at)
            VALUES ($1, $2::incident_kind, $3, $4, 'open', NOW())
            RETURNING id
            "#,
        )
        .bind(incident.transaction_id)
        .bind(incident.incident_type.to_string())
        .bind(incident.severity)
        .bind(&incident.description)
        .fetch_one(&self.pool)
        .await?;
        Ok(rec.0)
    }

    pub async fn escalate_incident(&self, incident_id: i64, new_severity: i16) -> Result<bool> {
        let res = sqlx::query(
            r#"
            UPDATE incidents
            SET severity = $2, escalated_at = NOW()
            WHERE id = $1 AND status = 'open' AND severity < $2
            "#,
        )
        .bind(incident_id)
        .bind(new_severity)
        .execute(&self.pool)
        .await?;
        Ok(res.rows_affected() > 0)
    }
}

#[derive(Clone)]
pub struct RedisStore {
    conn: ConnectionManager,
}

impl RedisStore {
    pub async fn connect(url: &str) -> Result<Self> {
        let client = redis::Client::open(url)?;
        let conn = ConnectionManager::new(client).await?;
        Ok(Self { conn })
    }

    pub fn connection(&self) -> ConnectionManager {
        self.conn.clone()
    }

    pub async fn ensure_consumer_group(&self, stream: &str, group: &str) -> Result<()> {
        let mut c = self.conn.clone();
        let result: redis::RedisResult<String> = redis::cmd("XGROUP")
            .arg("CREATE")
            .arg(stream)
            .arg(group)
            .arg("$")
            .arg("MKSTREAM")
            .query_async(&mut c)
            .await;

        match result {
            Ok(_) => {
                tracing::info!(stream = stream, group = group, "consumer group created");
                Ok(())
            }
            Err(e) if e.to_string().contains("BUSYGROUP") => {
                tracing::debug!(stream = stream, group = group, "consumer group already exists");
                Ok(())
            }
            Err(e) => Err(e.into()),
        }
    }

    pub async fn try_claim_event(&self, event_id: Uuid, ttl_ms: u64) -> Result<bool> {
        let mut c = self.conn.clone();
        let key = format!("dedup:event:{}", event_id);
        let result: Option<String> = redis::cmd("SET")
            .arg(&key)
            .arg("1")
            .arg("NX")
            .arg("PX")
            .arg(ttl_ms)
            .query_async(&mut c)
            .await?;
        Ok(result.is_some())
    }

    pub async fn save_window(&self, partial: &PartialTransaction) -> Result<()> {
        let mut c = self.conn.clone();
        let key = format!("window:{}", partial.transaction_id);
        let json = serde_json::to_string(partial)?;
        let _: () = redis::cmd("SET")
            .arg(&key)
            .arg(json)
            .arg("PX")
            .arg(30_000_u64)
            .query_async(&mut c)
            .await?;
        Ok(())
    }

    pub async fn load_window(&self, transaction_id: Uuid) -> Result<Option<PartialTransaction>> {
        let mut c = self.conn.clone();
        let key = format!("window:{}", transaction_id);
        let json: Option<String> = c.get(&key).await?;
        match json {
            Some(s) => Ok(Some(serde_json::from_str(&s)?)),
            None => Ok(None),
        }
    }

    pub async fn delete_window(&self, transaction_id: Uuid) -> Result<()> {
        let mut c = self.conn.clone();
        let key = format!("window:{}", transaction_id);
        let _: () = c.del(&key).await?;
        Ok(())
    }


    pub async fn load_all_windows(&self) -> Result<Vec<PartialTransaction>> {
        let mut c = self.conn.clone();
        // Используем SCAN, чтобы не блокировать Redis на больших dataset'ах.
        let mut cursor: u64 = 0;
        let mut windows = Vec::new();
        loop {
            let (next_cursor, keys): (u64, Vec<String>) = redis::cmd("SCAN")
                .arg(cursor)
                .arg("MATCH")
                .arg("window:*")
                .arg("COUNT")
                .arg(100)
                .query_async(&mut c)
                .await?;

            for key in keys {
                if let Ok(Some(s)) = c.get::<_, Option<String>>(&key).await {
                    if let Ok(p) = serde_json::from_str::<PartialTransaction>(&s) {
                        windows.push(p);
                    }
                }
            }

            cursor = next_cursor;
            if cursor == 0 {
                break;
            }
        }
        Ok(windows)
    }

    pub async fn add_deadline(&self, transaction_id: Uuid, deadline_ms: i64) -> Result<()> {
        let mut c = self.conn.clone();
        let _: () = c.zadd("deadlines", transaction_id.to_string(), deadline_ms).await?;
        Ok(())
    }

    pub async fn pop_expired_deadlines(&self, now_ms: i64) -> Result<Vec<Uuid>> {
        let mut c = self.conn.clone();
        let members: Vec<String> = c.zrangebyscore("deadlines", "-inf", now_ms).await?;

        let mut result = Vec::with_capacity(members.len());
        for m in members {
            // Атомарный захват: ZREM возвращает 1 только если мы первые удалили
            let removed: i64 = c.zrem("deadlines", &m).await?;
            if removed == 1 {
                if let Ok(id) = Uuid::parse_str(&m) {
                    result.push(id);
                }
            }
        }
        Ok(result)
    }

    pub async fn remove_deadline(&self, transaction_id: Uuid) -> Result<()> {
        let mut c = self.conn.clone();
        let _: () = c.zrem("deadlines", transaction_id.to_string()).await?;
        Ok(())
    }

    pub async fn add_escalation(&self, incident_id: i64, escalate_at_ms: i64) -> Result<()> {
        let mut c = self.conn.clone();
        let _: () = c.zadd("incident_escalations", incident_id, escalate_at_ms).await?;
        Ok(())
    }

    pub async fn pop_expired_escalations(&self, now_ms: i64) -> Result<Vec<i64>> {
        let mut c = self.conn.clone();
        let members: Vec<i64> = c.zrangebyscore("incident_escalations", "-inf", now_ms).await?;

        let mut result = Vec::with_capacity(members.len());
        for id in members {
            let removed: i64 = c.zrem("incident_escalations", id).await?;
            if removed == 1 {
                result.push(id);
            }
        }
        Ok(result)
    }

    pub async fn remove_escalation(&self, incident_id: i64) -> Result<()> {
        let mut c = self.conn.clone();
        let _: () = c.zrem("incident_escalations", incident_id).await?;
        Ok(())
    }


    pub async fn try_register_source(
        &self,
        transaction_id: Uuid,
        source: Source,
    ) -> Result<bool> {
        let mut c = self.conn.clone();
        let key = format!("sources:{}", transaction_id);
        let added: i64 = c.sadd(&key, source.to_string()).await?;
        // TTL чтобы не накапливать
        let _: () = c.expire(&key, 60).await?;
        Ok(added == 1)
    }

    pub async fn cleanup_sources(&self, transaction_id: Uuid) -> Result<()> {
        let mut c = self.conn.clone();
        let key = format!("sources:{}", transaction_id);
        let _: () = c.del(&key).await?;
        Ok(())
    }

    pub async fn publish(&self, channel: &str, message: &str) -> Result<()> {
        let mut c = self.conn.clone();
        let _: i64 = c.publish(channel, message).await?;
        Ok(())
    }
}