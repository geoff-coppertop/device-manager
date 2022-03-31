# Make is verbose in Linux. Make it silent.
MAKEFLAGS += --silent

APP_NAME=device-manager-plugin

# Redirect error output to a file, so we can show it in development mode.
STDERR="/tmp/$(APP_NAME)-stderr.txt"

## clean: Clean build files.
clean:
	@echo "  >  Cleaning build cache"
	@go clean --mod=mod
	@rm -rf out
	@rm -rf internal/mocks

deps:
	@echo "  >  Getting dependencies..."
	@go install github.com/golang/mock/mockgen@latest
	@go mod download

generate:
	@echo "  >  Generate code..."
	@go generate ./...

## test: Clean and run all unit tests
test: clean deps generate
	@echo "  >  Running tests..."
	@mkdir -p out
	@go test -v -coverprofile=./out/coverage.out --mod=mod ./...

## coverage: Show unit test coverage report
coverage: test
	@echo "  >  Parsing coverage..."
	@go tool cover -html=./out/coverage.out

compile: test
	@echo "  >  Building binary..."
	@go build --mod=mod -o ./out/$(APP_NAME) ./cmd/$(APP_NAME)/main.go

## build: Compile the binary.
build:
	@-touch $(STDERR)
	@-rm $(STDERR)
	@-$(MAKE) -s compile 2> $(STDERR)
	@cat $(STDERR) | sed -e '1s/.*/\nError:\n/'  | sed 's/make\[.*/ /' | sed "/^/s/^/     /" 1>&2

.PHONY: help
all: help
help: Makefile
	@echo
	@echo " Choose a command run in $(APP_NAME):"
	@echo
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'
	@echo
