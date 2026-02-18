SHELL := /bin/sh
APP := secapi-proxy-hetzner
IMAGE ?= $(APP):local
DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/secapi_proxy?sslmode=disable
SERVICE_DIR := service
MIGRATIONS_DIR := $(SERVICE_DIR)/db/migrations
GO_ENV := GOCACHE=$(CURDIR)/.cache/go-build
CONFORMANCE_DIR ?= resources/conformance
CONFORMANCE_REPO ?= https://github.com/eu-sovereign-cloud/conformance
CONFORMANCE_PROVIDER_REGION_V1 ?= http://localhost:8080
CONFORMANCE_PROVIDER_AUTHORIZATION_V1 ?= http://localhost:8080
CONFORMANCE_CLIENT_AUTH_TOKEN ?= dev-token
CONFORMANCE_CLIENT_TENANT ?= dev
CONFORMANCE_CLIENT_REGION ?= nbg1
CONFORMANCE_SCENARIOS_USERS ?= user1@example.com,user2@example.com
CONFORMANCE_SCENARIOS_CIDR ?= 10.10.0.0/16
CONFORMANCE_SCENARIOS_PUBLIC_IPS ?= 203.0.113.0/28
CONFORMANCE_RESULTS_PATH ?= $(CURDIR)/.artifacts/conformance/results
CONFORMANCE_SMOKE_FILTER ?= Region.V1.List
CONFORMANCE_FILTER ?=

.PHONY: all bootstrap fmt lint generate test test-integration test-contract phase1-smoke phase2-smoke conformance-bootstrap conformance-run conformance-smoke conformance-full conformance-region conformance-auth conformance-workspace conformance-compute conformance-storage conformance-network conformance-foundation conformance-foundation-core build run migrate-up migrate-down sqlc-gen docker-build docker-run docker-push release ci-verify ci-unit ci-integration ci-contract ci-conformance ci-package

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

conformance-run:
	@$(MAKE) conformance-bootstrap
	@echo "Running conformance filter '$(CONFORMANCE_FILTER)' against $(CONFORMANCE_PROVIDER_REGION_V1) ..."
	@mkdir -p "$(CONFORMANCE_RESULTS_PATH)"
	cd $(CONFORMANCE_DIR) && $(GO_ENV) go test -count=1 -v ./cmd/conformance -args run \
		--provider.region.v1="$(CONFORMANCE_PROVIDER_REGION_V1)" \
		--provider.authorization.v1="$(CONFORMANCE_PROVIDER_AUTHORIZATION_V1)" \
		--client.auth.token="$(CONFORMANCE_CLIENT_AUTH_TOKEN)" \
		--client.tenant="$(CONFORMANCE_CLIENT_TENANT)" \
		--client.region="$(CONFORMANCE_CLIENT_REGION)" \
		--scenarios.users="$(CONFORMANCE_SCENARIOS_USERS)" \
		--scenarios.cidr="$(CONFORMANCE_SCENARIOS_CIDR)" \
		--scenarios.public.ips="$(CONFORMANCE_SCENARIOS_PUBLIC_IPS)" \
		--scenarios.filter="$(CONFORMANCE_FILTER)" \
		--retry.base.delay=0 \
		--retry.base.interval=1 \
		--retry.max.attempts=3 \
		--report.results.path="$(CONFORMANCE_RESULTS_PATH)"

conformance-smoke:
	@$(MAKE) conformance-run CONFORMANCE_FILTER="$(CONFORMANCE_SMOKE_FILTER)"

conformance-full:
	@$(MAKE) conformance-run CONFORMANCE_FILTER=".*"

conformance-region:
	@$(MAKE) conformance-run CONFORMANCE_FILTER="Region\\.V1\\..*"

conformance-auth:
	@$(MAKE) conformance-run CONFORMANCE_FILTER="Authorization\\.V1\\..*"

conformance-workspace:
	@$(MAKE) conformance-run CONFORMANCE_FILTER="Workspace\\.V1\\..*"

conformance-compute:
	@$(MAKE) conformance-run CONFORMANCE_FILTER="Compute\\.V1\\..*"

conformance-storage:
	@$(MAKE) conformance-run CONFORMANCE_FILTER="Storage\\.V1\\..*"

conformance-network:
	@$(MAKE) conformance-run CONFORMANCE_FILTER="Network\\.V1\\..*"

conformance-foundation:
	@$(MAKE) conformance-run CONFORMANCE_FILTER="Foundation\\.V1\\..*"

conformance-foundation-core:
	@$(MAKE) conformance-region
	@$(MAKE) conformance-workspace
	@$(MAKE) conformance-compute
	@$(MAKE) conformance-storage
	@$(MAKE) conformance-network

conformance-bootstrap:
	@if [ ! -d "$(CONFORMANCE_DIR)/.git" ]; then \
		echo "Cloning $(CONFORMANCE_REPO) into $(CONFORMANCE_DIR)"; \
		git clone "$(CONFORMANCE_REPO)" "$(CONFORMANCE_DIR)"; \
	else \
		echo "Conformance repo already present at $(CONFORMANCE_DIR)"; \
	fi

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
