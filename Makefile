.PHONY: all build frontend-build copy-frontend backend-build clean dev run

BINARY     := webssh
BACKEND    := ./backend
FRONTEND   := ./frontend
STATIC_DIR := $(BACKEND)/internal/static/files
OUT_DIR    := $(FRONTEND)/out

all: build

## Build the complete single binary (frontend embedded in Go backend)
build: frontend-build copy-frontend backend-build

## Build the Next.js frontend in static export mode
frontend-build:
	cd $(FRONTEND) && NEXT_STATIC_EXPORT=true npm run build

## Copy built frontend files into the Go embed directory
copy-frontend:
	rm -rf $(STATIC_DIR)/*
	cp -r $(OUT_DIR)/. $(STATIC_DIR)/
	@echo "Frontend files copied to $(STATIC_DIR)"

## Compile the Go backend with embedded frontend (-tags embed)
backend-build:
	cd $(BACKEND) && go build \
		-tags embed \
		-ldflags "-s -w" \
		-o ../$(BINARY) \
		./cmd/server/

## Remove all build artifacts
clean:
	rm -rf $(OUT_DIR)
	rm -rf $(STATIC_DIR)/*
	touch $(STATIC_DIR)/.gitkeep
	rm -f ./$(BINARY)

## Run both servers in development mode (requires MASTER_SECRET env var)
## Set BACKEND_URL=http://localhost:8080 if not already set
dev:
	@test -n "$$MASTER_SECRET" || (echo "ERROR: MASTER_SECRET env var must be set"; exit 1)
	@echo "Starting backend on :8080 ..."
	cd $(BACKEND) && go run ./cmd/server/ &
	@echo "Starting frontend dev server on :3000 ..."
	@echo "Tip: WebSocket requires NEXT_PUBLIC_WS_URL=ws://localhost:8080"
	BACKEND_URL=$${BACKEND_URL:-http://localhost:8080} cd $(FRONTEND) && npm run dev

## Run the compiled single binary (requires MASTER_SECRET)
run:
	./$(BINARY)
