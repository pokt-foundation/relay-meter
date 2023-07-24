SHELL := /bin/bash

build:
	CGO_ENABLED=0 GOOS=linux go build -a -o bin/collector ./cmd/collector/main.go
	CGO_ENABLED=0 GOOS=linux go build -a -o bin/apiserver ./cmd/apiserver/main.go

# These targets spin up and shut down the E2E test env in docker.
test_env_up:
	@echo "🧪 Starting up Relay Meter test environment ..."
	@docker-compose -f ./testdata/docker-compose.test.yml up -d --remove-orphans --build >/dev/null
	@echo "⏳ Waiting for services to be ready ..."
	@echo "⏳ Waiting for portal-db to be ready ..."
	@attempts=0; until pg_isready -h localhost -p 5432 -U postgres >/dev/null || [[ $$attempts -eq 5 ]]; do sleep 2; ((attempts++)); done
	@[[ $$attempts -lt 5 ]] && echo "🐘 portal-db is up ..." || (echo "❌ portal-db failed to start" && make test_env_down >/dev/null && exit 1)
	@echo "⏳ Performing health check on pocket-http-db ..."
	@attempts=0; until curl -s http://localhost:8080/healthz >/dev/null || [[ $$attempts -eq 5 ]]; do sleep 2; ((attempts++)); done
	@[[ $$attempts -lt 5 ]] && echo "🖥️  pocket-http-db is online ..." || (echo "❌ pocket-http-db failed health check" && make test_env_down >/dev/null && exit 1)
	@echo "⏳ Waiting for relay-meter-db to be ready ..."
	@attempts=0; until pg_isready -h localhost -p 5434 -U postgres >/dev/null || [[ $$attempts -eq 5 ]]; do sleep 2; ((attempts++)); done
	@[[ $$attempts -lt 5 ]] && echo "🐘 relay-meter-db is up ..." || (echo "❌ relay-meter-db failed to start" && make test_env_down >/dev/null && exit 1)
	@echo "⏳ Performing health check on relay-meter-apiserver ..."
	@attempts=0; until curl -s http://localhost:9898/ >/dev/null || [[ $$attempts -eq 5 ]]; do sleep 2; ((attempts++)); done
	@[[ $$attempts -lt 5 ]] && echo "🖥️  relay-meter-apiserver is online ..." || (echo "❌ relay-meter-apiserver failed health check" && make test_env_down >/dev/null && exit 1)
	@echo "🚀 Test environment is up!"
test_env_down:
	@echo "🧪 Shutting down Relay Meter test environment ..."
	@docker-compose -f ./testdata/docker-compose.test.yml down --remove-orphans >/dev/null
	@echo "✅ Test environment is down."

run_e2e_tests:
	-go test ./... -run E2E -count=1
run_functional_tests:
	go test ./... -run Functional -count=1
run_all_tests:
	-go test ./... -count=1

test_unit:
	go test ./...  -short
test_e2e: test_env_up run_e2e_tests test_env_down

# temp TODO: remove when migration completed
export ENABLE_WRITING=y

test: test_unit run_functional_tests test_env_up run_e2e_tests test_env_down

gen_sql:
	sqlc generate -f ./driver-autogenerated/sqlc/sqlc.yaml
test_driver: test_driver_env_up run_driver_tests test_driver_env_down
test_driver_env_up:
	docker-compose -f ./driver-autogenerated/docker-compose.test.yml up -d --remove-orphans --build;
	sleep 2;
test_driver_env_down:
	docker-compose -f ./driver-autogenerated/docker-compose.test.yml down --remove-orphans -v
run_driver_tests:
	-go test ./... -run Test_RunPGDriverSuite -count=1 -v;

init-pre-commit:
	wget https://github.com/pre-commit/pre-commit/releases/download/v2.20.0/pre-commit-2.20.0.pyz;
	python3 pre-commit-2.20.0.pyz install;
	python3 pre-commit-2.20.0.pyz autoupdate;
	go install golang.org/x/tools/cmd/goimports@v0.6.0;
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.51.0;
	go install -v github.com/go-critic/go-critic/cmd/gocritic@v0.6.5;
	python3 pre-commit-2.20.0.pyz run --all-files;
	rm pre-commit-2.20.0.pyz;
