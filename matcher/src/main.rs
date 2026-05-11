mod config;
mod model;
mod store;
mod matcher;
mod stream;
mod timer;
mod publisher;
mod error;

use anyhow::Context;
use std::sync::Arc;
use tracing::info;
use tracing_subscriber::EnvFilter;

use crate::config::Config;
use crate::matcher::MatchingEngine;
use crate::publisher::EventPublisher;
use crate::store::{PostgresStore, RedisStore};

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    init_logging();

    let config = Config::from_env().context("failed to load config")?;
    info!(?config, "reconciler starting");

    let pg = PostgresStore::connect(&config.postgres_url)
        .await
        .context("postgres connect")?;
    info!("postgres connected");

    sqlx::migrate!("./migrations")
        .run(pg.pool())
        .await
        .context("failed to run migrations")?;
    info!("migrations applied");

    let redis = RedisStore::connect(&config.redis_url)
        .await
        .context("redis connect")?;
    info!("redis connected");

    redis
        .ensure_consumer_group(&config.redis_stream_name, &config.redis_consumer_group)
        .await
        .context("ensure consumer group")?;

    let publisher = Arc::new(EventPublisher::new(
        redis.clone(),
        config.redis_pubsub_channel.clone(),
    ));

    let engine = Arc::new(MatchingEngine::new(
        config.clone(),
        pg.clone(),
        redis.clone(),
        publisher.clone(),
    ));

    engine
        .restore_from_redis()
        .await
        .context("restore state from redis")?;
    info!("state restored from redis");

    let stream_task = {
        let engine = engine.clone();
        let cfg = config.clone();
        let redis = redis.clone();
        tokio::spawn(async move {
            if let Err(e) = stream::run(engine, redis, cfg).await {
                tracing::error!(error = %e, "stream consumer failed");
            }
        })
    };

    let timer_task = {
        let engine = engine.clone();
        let cfg = config.clone();
        tokio::spawn(async move {
            if let Err(e) = timer::run(engine, cfg).await {
                tracing::error!(error = %e, "timer worker failed");
            }
        })
    };

    // Health-endpoint для docker healthcheck и readiness.
    let health_task = {
        let pg = pg.clone();
        let redis = redis.clone();
        tokio::spawn(async move {
            if let Err(e) = run_health_endpoint(pg, redis).await {
                tracing::error!(error = %e, "health endpoint failed");
            }
        })
    };

    info!("reconciler ready, all workers started");

    tokio::select! {
        _ = tokio::signal::ctrl_c() => {
            info!("shutdown signal received");
        }
        _ = stream_task => {
            tracing::error!("stream task exited unexpectedly");
        }
        _ = timer_task => {
            tracing::error!("timer task exited unexpectedly");
        }
        _ = health_task => {
            tracing::error!("health task exited unexpectedly");
        }
    }

    info!("reconciler shut down");
    Ok(())
}

fn init_logging() {
    let env_filter = EnvFilter::try_from_env("LOG_LEVEL")
        .or_else(|_| EnvFilter::try_new("info"))
        .unwrap();

    tracing_subscriber::fmt()
        .json()
        .with_env_filter(env_filter)
        .with_current_span(false)
        .with_span_list(false)
        .init();
}

async fn run_health_endpoint(pg: PostgresStore, redis: RedisStore) -> anyhow::Result<()> {
    use tokio::io::{AsyncReadExt, AsyncWriteExt};
    use tokio::net::TcpListener;

    let addr = std::env::var("HEALTH_ADDR").unwrap_or_else(|_| "0.0.0.0:8081".to_string());
    let listener = TcpListener::bind(&addr).await?;
    info!(%addr, "health endpoint listening");

    loop {
        let (mut socket, _) = listener.accept().await?;
        let pg = pg.clone();
        let redis = redis.clone();

        tokio::spawn(async move {
            let mut buf = [0u8; 1024];
            let n = match socket.read(&mut buf).await {
                Ok(n) if n > 0 => n,
                _ => return,
            };

            let request = String::from_utf8_lossy(&buf[..n]);
            let path = request
                .lines()
                .next()
                .and_then(|line| line.split_whitespace().nth(1))
                .unwrap_or("/");

            let (status_line, body) = match path {
                "/healthz" => ("HTTP/1.1 200 OK", b"{\"status\":\"ok\"}".to_vec()),
                "/readyz" => {
                    let pg_ready = sqlx::query_scalar::<_, i32>("SELECT 1").fetch_one(pg.pool()).await.is_ok();
                    let mut conn = redis.connection();
                    let redis_ready: Result<String, _> = redis::cmd("PING").query_async(&mut conn).await;
                    let redis_ready = redis_ready.is_ok();

                    if pg_ready && redis_ready {
                        ("HTTP/1.1 200 OK", b"{\"status\":\"ready\"}".to_vec())
                    } else {
                        ("HTTP/1.1 503 Service Unavailable", b"{\"status\":\"unready\"}".to_vec())
                    }
                }
                _ => ("HTTP/1.1 404 Not Found", b"{\"error\":\"not_found\"}".to_vec()),
            };

            let response = format!(
                "{}\r\nContent-Type: application/json\r\nContent-Length: {}\r\n\r\n{}",
                status_line,
                body.len(),
                std::str::from_utf8(&body).unwrap()
            );
            let _ = socket.write_all(response.as_bytes()).await;
        });
    }
}