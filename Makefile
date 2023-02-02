ifeq (, $(shell which golangci-lint))
$(warning "could not find golangci-lint in $(PATH), run: curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh")
endif

ifeq (, $(shell which richgo))
$(warning "could not find richgo, run: go get github.com/kyoh86/richgo")
	GO_TEST = "go"
else
	GO_TEST = "richgo"
endif

.PHONY: default
default: all

# all: fmt test
.PHONY: all
all: test test_race

.PHONY: lint
lint:
	$(info ******************** running lint tools ********************)
	golangci-lint run ./...

.PHONY: test
test:
	$(info ******************** running tests ********************)
	@$(GO_TEST) test -v ./...

.PHONY: test_race
test_race:
	$(info ***************** running tests (race) ****************)
	@$(GO_TEST) test -race ./...

.PHONY: test_gocommand_all
test_gocommand_all:
	$(info ************* running tests (GoCommandAll) ************)
	@$(GO_TEST) test -run TestGoCommandAll -gocommand-all

.PHONY: clean
clean:
	@go clean
	@rm -rf bin
