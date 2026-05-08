.PHONY: help proto build test vet lint clean kind-up kind-down deploy-local ci-local

SERVICES := api-gateway submission-svc sandbox-runner bot-coordinator bot-worker \
            telemetry-ingester aggregator validator leaderboard-svc

MODULES := $(addprefix services/,$(SERVICES)) proto/gen/go

help:
	@echo "Targets:"
	@echo "  proto         Generate Go code from .proto files (requires buf)"
	@echo "  build         Build all services"
	@echo "  test          Run unit tests across workspace"
	@echo "  vet           go vet across all modules"
	@echo "  lint          Run golangci-lint across workspace"
	@echo "  ci-local      vet + build + test (mirrors CI)"
	@echo "  kind-up       Create local kind cluster"
	@echo "  kind-down     Delete local kind cluster"
	@echo "  deploy-local  Deploy services to kind"
	@echo "  clean         Remove build artifacts"

proto:
	./scripts/proto-gen.sh

build:
	@for m in $(MODULES); do \
		echo "Building $$m"; \
		(cd $$m && go build ./...) || exit 1; \
	done

test:
	@for m in $(MODULES); do \
		echo "Testing $$m"; \
		(cd $$m && go test -race -count=1 ./...) || exit 1; \
	done

vet:
	@for m in $(MODULES); do \
		(cd $$m && go vet ./...) || exit 1; \
	done

lint:
	golangci-lint run ./...

ci-local: vet build test

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
