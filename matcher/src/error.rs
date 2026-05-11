use thiserror::Error;

#[derive(Error, Debug)]
pub enum ReconcilerError {
    #[error("redis error: {0}")]
    Redis(#[from] redis::RedisError),

    #[error("postgres error: {0}")]
    Postgres(#[from] sqlx::Error),

    #[error("serialization error: {0}")]
    Serde(#[from] serde_json::Error),

    #[error("uuid parse error: {0}")]
    Uuid(#[from] uuid::Error),

    #[error("invalid stream payload: {0}")]
    InvalidPayload(String),

    #[error("missing required field: {0}")]
    MissingField(&'static str),
}

pub type Result<T> = std::result::Result<T, ReconcilerError>;