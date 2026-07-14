# Stage 1: Compiling the application.
# golang:1.23 (module targets go 1.21; Go is backward compatible): a supported
# toolchain, and it avoids the stale 1.21.0 base whose pull is unreliable.
FROM golang:1.23 AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy everything from the current directory to the PWD(Present Working Directory) inside the container
COPY . .

# Navigate to the cmd directory where the main.go file resides
WORKDIR /app/cmd

# Build the Go app
RUN CGO_ENABLED=1 GOOS=linux go build -o idp ./main.go

# List the files (for debugging purposes)
RUN ls -al

# Stage 2: Build the minimal docker image
FROM ubuntu:latest

# CA certificates are required for outbound HTTPS (the Google "Sign in with
# Google" token exchange and userinfo calls); the minimal base ships without them.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /root/

# Copy the pre-built binary from the previous stage
COPY --from=builder /app/cmd/idp /root/
COPY --from=builder /app/docs /root/docs



# Command to run
CMD ["/root/idp"]
