.PHONY: fmt fmt-files hooks test build check

fmt:
	@files="$$(git ls-files '*.go')"; \
	if [ -z "$$files" ]; then \
		echo "no tracked Go files to format"; \
	else \
		$(MAKE) fmt-files FILES="$$files"; \
	fi

fmt-files:
	@if [ -z "$(FILES)" ]; then \
		echo "set FILES to one or more Go files"; \
		exit 1; \
	fi
	goimports -w $(FILES)
	golines --base-formatter gofumpt -w $(FILES)

hooks:
	@git config core.hooksPath .githooks
	@echo "configured git hooksPath to .githooks"

test:
	go test ./...

build:
	go build ./...

check: test build
