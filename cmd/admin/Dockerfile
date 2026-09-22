FROM golang:1.25.3-alpine

WORKDIR /app

# Copy the entire project context
COPY . .

# Set the working directory to the specific service
WORKDIR /app/cmd/admin

# Download dependencies and build the CLI
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /usr/local/bin/admin-cli .

ENTRYPOINT ["/usr/local/bin/admin-cli"]
