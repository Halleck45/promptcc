BIN := promptcc
GO  ?= go

.PHONY: all build test fmt vet clean eval eval-update

all: build

build:
	$(GO) build -o $(BIN) ./cmd/promptcc

# gofmt is scoped to the project sources: eval/.cache holds third-party
# clones that must not be reformatted or checked.
GOFMT_DIRS := cmd internal

test:
	@test -z "$$(gofmt -l $(GOFMT_DIRS))" || (echo "gofmt needed on:" && gofmt -l $(GOFMT_DIRS) && exit 1)
	$(GO) vet ./...
	$(GO) test -race ./...

fmt:
	gofmt -w $(GOFMT_DIRS)

vet:
	$(GO) vet ./...

# Scan pinned open-source repositories (aider, cline, prism) and compare the
# results against the snapshots in eval/expected/. Catches extraction and
# scoring regressions on real-world code. If a diff is intentional (new rule,
# new weights), refresh the snapshots with `make eval-update`.
eval: build
	./eval/run.sh

eval-update: build
	EVAL_UPDATE=1 ./eval/run.sh

clean:
	rm -f $(BIN)
	rm -rf dist
