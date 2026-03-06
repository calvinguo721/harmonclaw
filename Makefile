.PHONY: build build-rv test smoke run

build:
	go build ./cmd/harmonclaw/

build-rv:
	CGO_ENABLED=0 GOOS=linux GOARCH=riscv64 go build -o harmonclaw-rv ./cmd/harmonclaw/

test:
	go vet ./...
	go build ./cmd/harmonclaw/

smoke:
	go test -count=1 ./test/ -run TestSmoke -timeout 60s

run:
	go run ./cmd/harmonclaw/
