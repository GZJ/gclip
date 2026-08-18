.PHONY: all windows clean build release fmt

all: bin/gcopy bin/gpaste bin/gpaste-image bin/gclipboard

bin/gcopy: cmd/gcopy/main_linux.go
	go build -o bin/gcopy ./cmd/gcopy/

bin/gpaste: cmd/gpaste/main_linux.go
	go build -o bin/gpaste ./cmd/gpaste/

bin/gpaste-image: cmd/gpaste-image/main_linux.go
	go build -o bin/gpaste-image ./cmd/gpaste-image/

bin/gclipboard: cmd/gclipboard/main_linux.go
	go build -o bin/gclipboard ./cmd/gclipboard/

windows:
	GOOS=windows GOARCH=amd64 go build -o bin/windows/gcopy.exe ./cmd/gcopy/
	GOOS=windows GOARCH=amd64 go build -o bin/windows/gpaste.exe ./cmd/gpaste/
	GOOS=windows GOARCH=amd64 go build -o bin/windows/gpaste-image.exe ./cmd/gpaste-image/
	GOOS=windows GOARCH=amd64 go build -o bin/windows/gclipboard.exe ./cmd/gclipboard/

fmt:
	gofumpt -w .

build:
	GOOS=linux GOARCH=amd64 goreleaser build --single-target --snapshot --clean

release:
	goreleaser release --clean

clean:
	rm -rf bin/ dist/
