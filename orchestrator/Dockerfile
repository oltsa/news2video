FROM golang:1.25.3-alpine AS builder

WORKDIR /app

# Copy the entire project context
COPY . .

# Set the working directory to the specific service
WORKDIR /app/orchestrator

# Download dependencies and build
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /orchestrator_app .

# --- Final Stage ---
FROM alpine:3.20
RUN addgroup -S app && adduser -S app -G app
USER app
WORKDIR /app
COPY --from=builder /orchestrator_app /usr/local/bin/orchestrator_app

EXPOSE 8080
ENTRYPOINT ["orchestrator_app"]
