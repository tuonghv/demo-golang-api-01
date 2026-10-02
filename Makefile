.PHONY: fmt-check vet build test check up down logs
fmt-check:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }
vet:
	go vet ./...
build:
	go build -o bin/api ./cmd/api
test:
	go test -race -count=1 ./...
check: fmt-check vet build test
up:
	docker compose up -d --build --wait
down:
	docker compose down
logs:
	docker compose logs -f api
