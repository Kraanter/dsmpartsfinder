# ---- Build Stage ----
FROM ubuntu:22.04 AS builder

# Install build dependencies (adjust as needed)
RUN apt-get update && apt-get install -y \
    build-essential \
    make \
    && rm -rf /var/lib/apt/lists/*

# Copy project files
WORKDIR /app
COPY . .

# Run Makefile build
RUN make build

# ---- Runtime Stage ----
FROM ubuntu:22.04

# Create non-root user (optional)
RUN useradd -m appuser

WORKDIR /app

# Copy the built binary from previous stage
COPY --from=builder /app/builds/dsmpartsfinder /app/dsmpartsfinder

# Ensure binary is executable
RUN chmod +x /app/dsmpartsfinder

USER appuser

# Change this if your binary takes args or listens on ports
ENTRYPOINT ["./dsmpartsfinder"]
