use crate::model::PubsubEvent;
use crate::store::RedisStore;
use anyhow::Result;
use tracing::debug;

pub struct EventPublisher {
    redis: RedisStore,
    channel: String,
}

impl EventPublisher {
    pub fn new(redis: RedisStore, channel: String) -> Self {
        Self { redis, channel }
    }

    pub async fn publish_event(&self, event: PubsubEvent) -> Result<()> {
        let json = serde_json::to_string(&event)?;
        self.redis.publish(&self.channel, &json).await?;
        debug!(event_type = ?event, "event published to pubsub");
        Ok(())
    }
}
