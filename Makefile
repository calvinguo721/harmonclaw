.PHONY: run build check-rv2 clean

run:
	go run ./cmd/harmonclaw/

build:
	go build -o harmonclaw.exe ./cmd/harmonclaw/

check-rv2:
	CGO_ENABLED=0 GOOS=linux GOARCH=riscv64 go build -o NUL ./cmd/harmonclaw/

clean:
	rm -f harmonclaw.exe
