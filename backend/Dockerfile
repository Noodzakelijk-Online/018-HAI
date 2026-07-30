# Stage 1: Compiling the application
FROM golang:1.25.12 AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy everything from the current directory to the PWD(Present Working Directory) inside the container
COPY . .

# Navigate to the cmd directory where the main.go file resides
WORKDIR /app/cmd

# Build the Go app
RUN CGO_ENABLED=1 GOOS=linux go build -o app ./main.go

# Stage 2: Build the minimal docker image
FROM ubuntu:24.04

# curl backs the container healthcheck, which polls /readyz. It has to live in
# the runtime image, not just the builder.
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /root/

# Copy the pre-built binary from the previous stage
COPY --from=builder /app/cmd/app /root/
COPY --from=builder /app/docs /root/docs



# Command to run
CMD ["/root/app"]
