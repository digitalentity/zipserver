# Stage 1: Build the application
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /app

# Copy and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o zipserver ./cmd/zipserver

# Stage 2: Final minimal image
FROM alpine:latest

# Install CA certificates for OAuth2/HTTPS calls
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/zipserver .

# Create a default directory for zips
RUN mkdir publish

# Expose the default port
EXPOSE 8080

# Run the binary
ENTRYPOINT ["./zipserver"]
CMD ["-config", "config.yaml"]
