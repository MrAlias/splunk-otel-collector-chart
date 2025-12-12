FROM mcr.microsoft.com/dotnet/sdk:8.0 AS builder

WORKDIR /app

# Copy project files
COPY ChainServer.csproj .
COPY Program.cs .

# Build
RUN dotnet publish -c Release -o out

# Final stage
FROM mcr.microsoft.com/dotnet/aspnet:8.0

WORKDIR /app

# Copy published app from builder
COPY --from=builder /app/out .

EXPOSE 8080

ENTRYPOINT ["dotnet", "ChainServer.dll"]
