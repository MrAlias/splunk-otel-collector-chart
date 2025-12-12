FROM rust:latest AS builder

WORKDIR /app

# Copy Cargo files
COPY Cargo.toml Cargo.lock* ./
COPY src ./src

# Build the application
RUN cargo build --release

# Final stage
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/target/release/chain-server ./server

EXPOSE 8080

CMD ["./server"]
