# Build stage
FROM golang:1.25.0-alpine AS builder

# Set working directory
WORKDIR /app

# Copy the entire go-sentinel project
COPY . .

# Set working directory to the vec-observer example
WORKDIR /app/examples/vec-observer

# Download dependencies
RUN go mod download

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Set working directory
WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/examples/vec-observer/main .

# Expose metrics port
EXPOSE 8080

# Run the application
CMD ["./main"]
