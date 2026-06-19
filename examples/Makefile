.PHONY: run test tidy help

run: ## Run the example tour
	go run .

test: ## Run example tests
	go test -race ./...

tidy: ## Tidy modules
	go mod tidy

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-6s\033[0m %s\n", $$1, $$2}'
