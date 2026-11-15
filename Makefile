# Makefile
BINARY := loadr
PKG    := ./...

.PHONY: build test vet fmt lint check clean

# CGO_ENABLED=0 keeps the binary static, as DESIGN promises: with cgo on,
# the net package links against the host's libc.
build:
	CGO_ENABLED=0 go build -trimpath -o $(BINARY) ./cmd/loadr

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
