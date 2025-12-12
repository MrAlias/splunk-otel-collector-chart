FROM eclipse-temurin:21-jdk-alpine AS builder

WORKDIR /app

# Copy Java source
COPY ChainServer.java .

# Install downloader and download json library
RUN apk add --no-cache curl && \
    curl -fsSL -o json-20231013.jar https://repo1.maven.org/maven2/org/json/json/20231013/json-20231013.jar

# Compile
RUN javac -cp json-20231013.jar ChainServer.java

# Final stage
FROM eclipse-temurin:21-jre-alpine

WORKDIR /app

# Copy compiled class and library from builder
COPY --from=builder /app/ChainServer.class .
COPY --from=builder /app/json-20231013.jar .

EXPOSE 8080

CMD ["java", "-cp", ".:json-20231013.jar", "ChainServer"]
