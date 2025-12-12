FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy source
COPY main.go .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o server main.go

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/server .

EXPOSE 8080

CMD ["./server"]
