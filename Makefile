MODULE := github.com/mikerudolph/artifacts
GOLANGCI_LINT_VERSION := v2.5.0
GOTESTCOVERAGE_VERSION := v2.17.0
MAX_FILE_LINES := 400

.PHONY: verify fmt vet lint test coverage complexity run

verify: fmt vet lint test coverage complexity

fmt:
	gofmt -w .
	go run golang.org/x/tools/cmd/goimports@v0.36.0 -w .

vet:
	go vet ./...

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run ./...

test:
	go test ./... -race -count=1

coverage:
	go test ./... -race -count=1 -coverprofile=cover.out -covermode=atomic -coverpkg=./...
	go run github.com/vladopajic/go-test-coverage/v2@$(GOTESTCOVERAGE_VERSION) --config=.testcoverage.yml

complexity:
	@fail=0; \
	for f in $$(find . -name '*.go' -not -path './.git/*'); do \
		n=$$(wc -l < "$$f"); \
		if [ "$$n" -gt $(MAX_FILE_LINES) ]; then \
			echo "FAIL $$f has $$n lines (max $(MAX_FILE_LINES))"; \
			fail=1; \
		fi; \
	done; \
	echo "package LOC:"; \
	find . -name '*.go' -not -path './.git/*' | sed 's|/[^/]*$$||' | sort -u | while read d; do \
		printf '  %s %s\n' "$$(find "$$d" -maxdepth 1 -name '*.go' | xargs wc -l 2>/dev/null | tail -n1 | awk '{print $$1}')" "$$d"; \
	done; \
	exit $$fail

run:
	go run ./cmd/artifacts serve
