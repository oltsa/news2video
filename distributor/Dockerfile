FROM golang:1.25.3-alpine AS builder

WORKDIR /app

# Copy the entire project context
COPY . .

# Set the working directory to the specific service
WORKDIR /app/distributor

# Download dependencies and build
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /distributor_app .

# --- Final Stage ---
FROM alpine:latest
COPY --from=builder /distributor_app /distributor_app
CMD ["/distributor_app"]
