use crate::config::Config;
use crate::matcher::MatchingEngine;
use anyhow::Result;
use std::sync::Arc;
use tokio::time::{sleep, Duration};
use tracing::{error, info};

pub async fn run(engine: Arc<MatchingEngine>, config: Config) -> Result<()> {
    info!(
        tick_ms = config.reconciler_timer_tick_ms,
        "timer worker started"
    );

    let tick_duration = Duration::from_millis(config.reconciler_timer_tick_ms);

    loop {

        if let Err(e) = engine.process_expired_windows().await {
            error!(error = %e, "failed to handle window timeouts");
        }

        if let Err(e) = engine.process_expired_escalations().await {
            error!(error = %e, "failed to handle incident escalations");
        }

        sleep(tick_duration).await;
    }
}
