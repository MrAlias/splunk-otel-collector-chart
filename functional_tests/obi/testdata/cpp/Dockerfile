FROM alpine:3.18 AS builder

RUN apk add --no-cache g++ make cmake curl

WORKDIR /tmp

# Download nlohmann/json header-only library
RUN mkdir -p /usr/include/nlohmann && \
    curl -fsSL -o /usr/include/nlohmann/json.hpp https://github.com/nlohmann/json/releases/download/v3.11.2/json.hpp

WORKDIR /app

COPY main.cpp .

# Build the application
RUN g++ -std=c++17 -O2 -o server main.cpp -I/usr/include

# Final stage
FROM alpine:3.18

RUN apk add --no-cache libstdc++ libgcc

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/server .

EXPOSE 8080

CMD ["./server"]
