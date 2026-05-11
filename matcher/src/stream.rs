
use crate::config::Config;
use crate::matcher::MatchingEngine;
use crate::model::StreamMessage;
use crate::store::RedisStore;
use anyhow::{Context, Result};
use redis::streams::{StreamReadOptions, StreamReadReply};
use redis::AsyncCommands;
use std::sync::Arc;
use tracing::{debug, error, info, warn};

pub async fn run(
    engine: Arc<MatchingEngine>,
    redis: RedisStore,
    config: Config,
) -> Result<()> {
    info!(
        stream = %config.redis_stream_name,
        group = %config.redis_consumer_group,
        consumer = %config.redis_consumer_name,
        "stream consumer started"
    );

    let mut conn = redis.connection();
    let opts = StreamReadOptions::default()
        .group(&config.redis_consumer_group, &config.redis_consumer_name)
        .count(10)
        .block(1000);

    loop {
        let reply: Result<StreamReadReply, _> = conn
            .xread_options(&[&config.redis_stream_name], &[">"], &opts)
            .await;

        let reply = match reply {
            Ok(r) => r,
            Err(e) => {
                error!(error = %e, "xread failed, retrying in 1s");
                tokio::time::sleep(std::time::Duration::from_secs(1)).await;
                continue;
            }
        };

        for stream_key in reply.keys {
            for stream_id in stream_key.ids {
                let id_str = stream_id.id.clone();

                // Извлекаем payload — Ingestor кладёт его в поле "payload"
                let mut buf = [0u8; 1024];
                let n = match socket.read(&mut buf).await {
                    Some(redis::Value::Data(bytes)) => String::from_utf8_lossy(bytes).to_string(),
                    _ => {
                        warn!(stream_id = %id_str, "skipping message without payload field");
                        let _ = ack_message(&mut conn, &config.redis_stream_name, &config.redis_consumer_group, &id_str).await;
                        continue;
                    }
                };

                let msg: StreamMessage = match serde_json::from_str(&payload) {
                    Ok(m) => m,
                    Err(e) => {
                        error!(error = %e, payload = %payload, "failed to parse stream message");
                        let _ = ack_message(&mut conn, &config.redis_stream_name, &config.redis_consumer_group, &id_str).await;
                        continue;
                    }
                };

                match engine.handle_event(msg).await {
                    Ok(()) => {
                        if let Err(e) = ack_message(
                            &mut conn,
                            &config.redis_stream_name,
                            &config.redis_consumer_group,
                            &id_str,
                        )
                        .await
                        {
                            error!(error = %e, stream_id = %id_str, "ack failed");
                        }
                    }
                    Err(e) => {

                        error!(error = %e, stream_id = %id_str, "handle_event failed, message will be retried");
                    }
                }
            }
        }
    }
}

async fn ack_message(
    conn: &mut redis::aio::ConnectionManager,
    stream: &str,
    group: &str,
    id: &str,
) -> Result<()> {
    let _: i64 = conn
        .xack(stream, group, &[id])
        .await
        .context("xack failed")?;
    debug!(stream_id = id, "message acknowledged");
    Ok(())
}