GO=go
PKG=github.com/mcwalrus/go-sentinel
COV=coverage.out

TEST_OPTS=-coverprofile $(COV)
ifdef TESTCASE
	TEST_OPTS=-run $(TESTCASE)
endif

TEST_OPTS:=$(TEST_OPTS) -v -count=1

.PHONY: test
test:
	$(GO) test $(TEST_OPTS) $(PKG)

.PHONY: race
race:
	$(GO) test -race $(TEST_OPTS) $(PKG)

.PHONY: cover
cover:
	$(GO) tool cover -func $(COV);

.PHONY: vcover
vcover:
	go tool cover -html=coverage.out

.PHONY: lint
lint:
	golangci-lint run