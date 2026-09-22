FROM golang:1.25.3-alpine AS builder

WORKDIR /app

# Copy the entire project context
COPY . .

# Set the working directory to the specific service
WORKDIR /app/renderer

# Download dependencies and build
RUN go mod download
RUN go build -trimpath -ldflags="-s -w" -o /usr/local/bin/renderer_app .

# --- Final Stage ---
FROM alpine:3.20

RUN apk add --no-cache \
    ffmpeg \
    imagemagick \
    libpng \
    libjpeg-turbo \
    librsvg

RUN addgroup -S app && adduser -S app -G app
USER app

COPY --from=builder /usr/local/bin/renderer_app /usr/local/bin/renderer_app

HEALTHCHECK --interval=15s --timeout=3s --retries=3 CMD wget -qO- http://localhost:8081/health || exit 1
ENTRYPOINT ["renderer_app"]
