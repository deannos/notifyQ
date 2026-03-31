.PHONY: run build tidy clean

BINARY=notifyq
GOFLAGS=CGO_ENABLED=1

run:
	$(GOFLAGS) go run .

build:
	$(GOFLAGS) go build -ldflags="-s -w" -o $(BINARY) .

tidy:
	go mod tidy

clean:
	rm -f $(BINARY) notifications.db
