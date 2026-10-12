run:
	go run ./cmd/portal

test:
	go test ./...

build:
	go build -o portal ./cmd/portal

build-mac:
	GOOS=darwin GOARCH=arm64 go build -o portal ./cmd/portal
