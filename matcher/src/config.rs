use serde::Deserialize;

#[derive(Debug, Clone, Deserialize)]
pub struct Config {
    #[serde(default = "default_postgres_url")]
    pub postgres_url: String,

    #[serde(default = "default_redis_url")]
    pub redis_url: String,

    #[serde(default = "default_stream_name")]
    pub redis_stream_name: String,

    #[serde(default = "default_consumer_group")]
    pub redis_consumer_group: String,

    #[serde(default = "default_consumer_name")]
    pub redis_consumer_name: String,

    #[serde(default = "default_pubsub_channel")]
    pub redis_pubsub_channel: String,

    #[serde(default = "default_reconciler_timeout_ms")]
    pub reconciler_timeout_ms: u64,

    #[serde(default = "default_reconciler_escalation_ms")]
    pub reconciler_escalation_ms: u64,

    #[serde(default = "default_dedup_window_ms")]
    pub reconciler_dedup_window_ms: u64,

    #[serde(default = "default_timer_tick_ms")]
    pub reconciler_timer_tick_ms: u64,

    #[serde(default = "default_expected_sources")]
    pub expected_sources_count: usize,
}

impl Config {
    pub fn from_env() -> anyhow::Result<Self> {
        let cfg = envy::from_env::<Config>()?;
        Ok(cfg)
    }
}

fn default_postgres_url() -> String {
    "postgres://recon:recon@postgres:5432/recon?sslmode=disable".to_string()
}
fn default_redis_url() -> String {
    "redis://redis:6379/0".to_string()
}
fn default_stream_name() -> String {
    "transactions:raw".to_string()
}
fn default_consumer_group() -> String {
    "reconciler".to_string()
}
fn default_consumer_name() -> String {
    format!("consumer-{}", uuid::Uuid::new_v4())
}
fn default_pubsub_channel() -> String {
    "reconciliation.events".to_string()
}
fn default_reconciler_timeout_ms() -> u64 {
    2000
}
fn default_reconciler_escalation_ms() -> u64 {
    10_000
}
fn default_dedup_window_ms() -> u64 {
    600_000
}
fn default_timer_tick_ms() -> u64 {
    100
}
fn default_expected_sources() -> usize {
    3
}