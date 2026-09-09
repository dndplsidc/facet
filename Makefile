build:
	go build -o facet .

build-test:
	@mkdir -p bin
	go build -o bin/facet-test .

test:
	go test ./... -v

test-cover:
	go test ./... -cover

# Requires skills@1.5.25 installed outside the checkout; see e2e/skills_cli_test.go.
test-skills-integration:
	go test -tags skillsintegration ./e2e -run TestRealSkillsLifecycle -v -count=1

clean:
	rm -f facet e2e/facet-linux e2e/facet
	rm -rf bin/
	-docker rmi facet-e2e 2>/dev/null

# --- Cross-compile for Docker ---
build-linux:
	GOOS=linux GOARCH=amd64 go build -o e2e/facet-linux .

build-linux-arm:
	GOOS=linux GOARCH=arm64 go build -o e2e/facet-linux .

# --- E2E: Docker (Linux, real packages) ---
e2e: build-linux
	docker build -t facet-e2e -f e2e/Dockerfile.ubuntu e2e/
	docker run --rm -e FACET_E2E_REAL_PACKAGES=1 facet-e2e
	rm -f e2e/facet-linux

e2e-suite: build-linux
	docker build -t facet-e2e -f e2e/Dockerfile.ubuntu e2e/
	docker run --rm -e FACET_E2E_REAL_PACKAGES=1 facet-e2e /opt/e2e/suites/$(SUITE).sh
	rm -f e2e/facet-linux

e2e-shell: build-linux
	docker build -t facet-e2e -f e2e/Dockerfile.ubuntu e2e/
	docker run --rm -it --entrypoint /bin/bash facet-e2e

# --- E2E: Native (macOS/Linux, mocked packages, isolated HOME) ---
# Safe: never touches your real config files or PATH.
e2e-local: build
	@echo "Running E2E locally (HOME will be sandboxed, packages mocked)"
	@PATH="$$PWD:$$PATH" bash e2e/harness.sh

e2e-local-suite: build
	@PATH="$$PWD:$$PATH" bash e2e/harness.sh e2e/suites/$(SUITE).sh

# --- Pre-commit: run everything CI will run ---
pre-commit: test e2e
	@echo "All tests passed (unit + E2E native + E2E linux)"

# --- CI convenience ---
ci: test e2e
	@echo "All tests passed"

.PHONY: build build-test build-linux build-linux-arm test test-cover test-skills-integration clean e2e e2e-suite e2e-shell e2e-local e2e-local-suite pre-commit ci
