.PHONY: lint
lint:
	@echo "@==> $@"
	@VERSION=$$(go run github.com/mikefarah/yq/v4@v4.34.1 '.jobs.lint.steps[] | select(.uses == "golangci/golangci-lint-action*") | .with.version' .github/workflows/lint.yaml) && \
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@$$VERSION run ./... --fix
