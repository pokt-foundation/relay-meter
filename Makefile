build:
	CGO_ENABLED=0 GOOS=linux go build -a -o bin/collector ./cmd/collector/main.go
	CGO_ENABLED=0 GOOS=linux go build -a -o bin/apiserver ./cmd/apiserver/main.go

test_env_up:
	docker-compose -f ./docker-compose.test.yml up -d --remove-orphans --build;
	sleep 2;
test_env_down:
	docker-compose -f ./docker-compose.test.yml down --remove-orphans
run_integration_tests:
	-go test ./... -run Integration -count=1;
run_e2e_tests:
	-go test ./... -run E2E -count=1 -tags=tests;
run_functional_tests:
	-go test ./... -run Functional -count=1 -tags=tests;
run_all_tests:
	-go test ./... -count=1;

test_unit:
	go test ./...  -short
test_integration: test_env_up run_integration_tests  test_env_down
test_e2e:         test_env_up run_e2e_tests  test_env_down
test:             test_env_up test_unit run_integration_tests run_e2e_tests test_env_down

init-pre-commit:
	wget https://github.com/pre-commit/pre-commit/releases/download/v2.20.0/pre-commit-2.20.0.pyz;
	python3 pre-commit-2.20.0.pyz install;
	python3 pre-commit-2.20.0.pyz autoupdate;
	go install golang.org/x/tools/cmd/goimports@latest;
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest;
	go install -v github.com/go-critic/go-critic/cmd/gocritic@latest;
	python3 pre-commit-2.20.0.pyz run --all-files;
	rm pre-commit-2.20.0.pyz;
