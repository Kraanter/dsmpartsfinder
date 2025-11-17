# ──────────────────────────────────────────────────────────────
# 1️⃣ FRONTEND BUILD STAGE — build Vue app with Bun
# ──────────────────────────────────────────────────────────────
FROM node:21-alpine3.19 AS frontend-builder

WORKDIR /app
COPY frontend/ ./frontend/
WORKDIR /app/frontend

# Install deps and build
RUN yarn
RUN yarn run build

# ──────────────────────────────────────────────────────────────
# 2️⃣ BACKEND BUILD STAGE — build static Go binary with embedded frontend
# ──────────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS backend-builder

WORKDIR /app

# Copy backend source
COPY api/ ./api/

# Copy frontend build artifacts into Go embed dir
COPY --from=frontend-builder /app/frontend/dist ./api/frontend/

# Build static Go binary for Linux amd64
WORKDIR /app/api
RUN go mod download 
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
      -ldflags="-s -w -extldflags '-static'" \
      -o /server ./cmd/server

# ──────────────────────────────────────────────────────────────
# 3️⃣ FINAL STAGE — minimal scratch image
# ──────────────────────────────────────────────────────────────
FROM scratch

# Copy binary
COPY --from=backend-builder /server /server

# Expose port (adjust if needed)
EXPOSE 8080

# Run server
ENTRYPOINT ["/server"]
