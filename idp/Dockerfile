# Stage 1: Compiling the application
FROM golang:1.25.12 AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy everything from the current directory to the PWD(Present Working Directory) inside the container
COPY . .

# Navigate to the cmd directory where the main.go file resides
WORKDIR /app/cmd

# Sarama and the IDP are pure Go. Keeping CGO disabled produces a portable
# Linux binary without a host C toolchain or runtime library dependency.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o idp ./main.go

# Stage 2: Build the runtime image
FROM ubuntu:24.04

# CA certificates are required for outbound HTTPS (the Google "Sign in with
# Google" token exchange and userinfo calls); the minimal base ships without them.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && groupadd --gid 10001 hai \
    && useradd --uid 10001 --gid hai --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin hai \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy the pre-built binary from the previous stage
COPY --from=builder --chown=10001:10001 /app/cmd/idp /app/idp
COPY --from=builder --chown=10001:10001 /app/docs /app/docs

USER 10001:10001

ENTRYPOINT ["/app/idp"]
