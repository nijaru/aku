.PHONY: fmt fmt-check fmt-files hooks test build check

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

fmt-check:
	@files="$$(git ls-files '*.go')"; \
	if [ -z "$$files" ]; then \
		echo "no tracked Go files to check"; \
	else \
		needs="$$( { goimports -l $$files; golines --base-formatter gofumpt --list-files $$files; } | sort -u )"; \
		if [ -n "$$needs" ]; then \
			echo "formatting drift detected:"; \
			printf '%s\n' "$$needs"; \
			exit 1; \
		fi; \
	fi

hooks:
	@git config core.hooksPath .githooks
	@echo "configured git hooksPath to .githooks"

test:
	go test ./...

build:
	go build ./...

check: fmt-check test build
