SHELL := /bin/sh
APP := secapi-proxy-hetzner
IMAGE ?= $(APP):local
DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/secapi_proxy?sslmode=disable
SERVICE_DIR := service
MIGRATIONS_DIR := $(SERVICE_DIR)/db/migrations
GO_ENV := GOCACHE=$(CURDIR)/.cache/go-build

.PHONY: all bootstrap fmt lint generate test test-integration test-contract phase1-smoke phase2-smoke conformance-smoke conformance-full build run migrate-up migrate-down sqlc-gen docker-build docker-run docker-push release ci-verify ci-unit ci-integration ci-contract ci-conformance ci-package

all: build

bootstrap:
	cd $(SERVICE_DIR) && $(GO_ENV) go mod tidy

fmt:
	cd $(SERVICE_DIR) && gofmt -w $$(find . -name '*.go')

lint:
	cd $(SERVICE_DIR) && $(GO_ENV) go test ./... >/dev/null

test:
	cd $(SERVICE_DIR) && $(GO_ENV) go test ./...

test-integration:
	cd $(SERVICE_DIR) && $(GO_ENV) go test ./test/integration/...

test-contract:
	cd $(SERVICE_DIR) && $(GO_ENV) go test ./test/contract/...

generate:
	cd $(SERVICE_DIR) && $(GO_ENV) go generate ./...

phase1-smoke:
	@echo "Checking Phase 1 endpoints on http://localhost:8080 ..."
	@command -v jq >/dev/null
	@curl -fsS http://localhost:8080/healthz >/dev/null
	@curl -fsS http://localhost:8080/readyz >/dev/null
	@curl -fsS http://localhost:8080/.wellknown/secapi | grep -q '"version":"v1"'
	@REGION_NAME=$$(curl -fsS http://localhost:8080/v1/regions | jq -r '.items[0].metadata.name'); \
		test -n "$$REGION_NAME"; \
		curl -fsS "http://localhost:8080/v1/regions/$$REGION_NAME" | grep -q "\"name\":\"$$REGION_NAME\""
	@SKU_NAME=$$(curl -fsS http://localhost:8080/compute/v1/tenants/dev/skus | jq -r '.items[0].metadata.name'); \
		test -n "$$SKU_NAME"; \
		curl -fsS "http://localhost:8080/compute/v1/tenants/dev/skus/$$SKU_NAME" | grep -q "\"name\":\"$$SKU_NAME\""
	@IMAGE_NAME=$$(curl -fsS http://localhost:8080/storage/v1/tenants/dev/images | jq -r '.items[0].metadata.name'); \
		test -n "$$IMAGE_NAME"; \
		curl -fsS "http://localhost:8080/storage/v1/tenants/dev/images/$$IMAGE_NAME" | grep -q "\"name\":\"$$IMAGE_NAME\""
	@echo "Phase 1 smoke checks passed."

phase2-smoke:
	@echo "Checking Phase 2 endpoints on http://localhost:8080 ..."
	@command -v jq >/dev/null
	@curl -fsS http://localhost:8080/compute/v1/tenants/dev/workspaces/default/instances | jq -e '.items and .metadata' >/dev/null
	@curl -fsS http://localhost:8080/storage/v1/tenants/dev/workspaces/default/block-storages | jq -e '.items and .metadata' >/dev/null
	@curl -sS -X PUT http://localhost:8080/compute/v1/tenants/dev/workspaces/default/instances/invalid -H 'content-type: application/json' -d '{}' | jq -e '.status == 400' >/dev/null
	@curl -sS -X PUT http://localhost:8080/storage/v1/tenants/dev/workspaces/default/block-storages/invalid -H 'content-type: application/json' -d '{}' | jq -e '.status == 400' >/dev/null
	@echo "Phase 2 smoke checks passed."

conformance-smoke:
	@echo "conformance smoke placeholder: wire eu-sovereign-cloud/conformance in next phase"

conformance-full:
	@echo "conformance full placeholder: wire eu-sovereign-cloud/conformance in next phase"

build:
	cd $(SERVICE_DIR) && $(GO_ENV) go build ./cmd/secapi-proxy-hetzner

run:
	cd $(SERVICE_DIR) && $(GO_ENV) go run ./cmd/secapi-proxy-hetzner

migrate-up:
	migrate -database "$(DATABASE_URL)" -path "$(MIGRATIONS_DIR)" up

migrate-down:
	migrate -database "$(DATABASE_URL)" -path "$(MIGRATIONS_DIR)" down 1

sqlc-gen:
	cd $(SERVICE_DIR) && sqlc generate

docker-build:
	docker build -f $(SERVICE_DIR)/Dockerfile -t $(IMAGE) $(SERVICE_DIR)

docker-run:
	docker run --rm -p 8080:8080 --env SECA_DATABASE_URL="$(DATABASE_URL)" $(IMAGE)

docker-push:
	docker push $(IMAGE)

release: fmt lint test build docker-build
	@echo "release bundle built"

ci-verify: fmt lint
ci-unit: test
ci-integration: test-integration
ci-contract: test-contract
ci-conformance: conformance-smoke conformance-full
ci-package: build docker-build
