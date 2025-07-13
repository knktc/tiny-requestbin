# Use official Go image as build environment
FROM golang:1.23-alpine AS builder

# Set working directory
WORKDIR /app

# Install necessary packages
RUN apk add --no-cache git
# Copy go mod files
COPY go.mod ./

# Download dependencies (if any)
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o tiny-requestbin .

# Use minimal base image
FROM scratch

# Copy the binary from builder stage
COPY --from=builder /app/tiny-requestbin /tiny-requestbin

# Expose port
EXPOSE 8282

# Set startup command
ENTRYPOINT ["/tiny-requestbin"]
CMD ["-port", "8282", "-listen", "0.0.0.0"]
