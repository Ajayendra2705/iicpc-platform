.PHONY: help proto build test lint clean kind-up kind-down deploy-local

SERVICES := api-gateway submission-svc sandbox-runner bot-coordinator bot-worker \
            telemetry-ingester aggregator validator leaderboard-svc

help:
	@echo "Targets:"
	@echo "  proto         Generate Go code from .proto files"
	@echo "  build         Build all services"
	@echo "  test          Run unit tests across workspace"
	@echo "  lint          Run golangci-lint across workspace"
	@echo "  kind-up       Create local kind cluster"
	@echo "  kind-down     Delete local kind cluster"
	@echo "  deploy-local  Deploy services to kind"
	@echo "  clean         Remove build artifacts"

proto:
	./scripts/proto-gen.sh

build:
	@for svc in $(SERVICES); do \
		echo "Building $$svc"; \
		(cd services/$$svc && go build ./...); \
	done

test:
	go test ./...

lint:
	golangci-lint run ./...

kind-up:
	./scripts/kind-up.sh

kind-down:
	kind delete cluster --name iicpc

deploy-local:
	@echo "TODO: helm upgrade --install per service"

clean:
	@for svc in $(SERVICES); do \
		rm -rf services/$$svc/bin; \
	done
