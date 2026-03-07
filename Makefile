.PHONY: build build-rv build-all test vet lint smoke run clean docker

build:
	go build ./cmd/harmonclaw/

build-rv:
	CGO_ENABLED=0 GOOS=linux GOARCH=riscv64 go build -o harmonclaw-rv ./cmd/harmonclaw/

build-all:
	@echo "Run scripts/build-all.ps1 for full cross-compile"

test:
	go vet ./...
	go test ./...

vet:
	go vet ./...

lint:
	go vet ./...

smoke:
	go test -count=1 ./test/ -run TestSmoke -timeout 60s

run:
	go run ./cmd/harmonclaw/

clean:
	rm -f harmonclaw harmonclaw.exe harmonclaw-rv harmonclaw-*

docker:
	docker build -t harmonclaw .
