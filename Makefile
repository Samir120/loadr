# Makefile
BINARY := loadr
PKG    := ./...

.PHONY: build test vet fmt lint check clean

build:
	go build -trimpath -o $(BINARY) ./cmd/loadr

test:
	go test -race $(PKG)

vet:
	go vet $(PKG)

fmt:
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then \
		echo "gofmt: unformatted files:"; echo "$$out"; exit 1; \
	fi

lint:
	staticcheck $(PKG)

check: fmt vet lint test

clean:
	rm -f $(BINARY)
