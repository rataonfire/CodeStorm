# Build stage
FROM rust:1.88-slim-bookworm AS builder

WORKDIR /app

# Install Rust build dependencies
RUN apt-get update && apt-get install -y \
    pkg-config \
    libssl-dev \
    build-essential \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy Cargo manifest only and build a dummy binary to cache dependencies.
COPY Cargo.toml ./
RUN mkdir -p src && printf 'fn main() {}' > src/main.rs
RUN cargo build --release
RUN rm -f target/release/deps/reconciler*

# Copy actual source and migrations, then build the real binary.
COPY src ./src
COPY migrations ./migrations
RUN cargo build --release --bin reconciler

# Runtime stage
FROM debian:bookworm-slim
WORKDIR /app
RUN apt-get update && apt-get install -y \
    ca-certificates \
    libssl3 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/target/release/reconciler /usr/local/bin/reconciler

ENV LOG_LEVEL=info
ENV HEALTH_ADDR=0.0.0.0:8081
EXPOSE 8081
CMD ["/usr/local/bin/reconciler"]
