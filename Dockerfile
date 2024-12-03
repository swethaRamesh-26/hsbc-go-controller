# Use a minimal base image for Go applications
FROM golang:1.20 as builder

# Set the working directory
WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod tidy

# Copy the source code
COPY . .

# Build the application
RUN go build -o namespace-labeler .

# Use a smaller base image for the final container
FROM debian:bullseye-slim

# Set the working directory in the container
WORKDIR /root/

# Copy the binary from the builder stage
COPY --from=builder /app/namespace-labeler .

# Add a non-root user
RUN useradd -u 10001 appuser

# Use the non-root user
USER appuser

# Command to run the application
ENTRYPOINT ["./namespace-labeler"]
