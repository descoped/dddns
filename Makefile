hello:
	echo "Hello"

build:
	go build -o bin/main main.go

compile:
	GOOS=darwin GOARCH=amd64 go build -o bin/main-macos main.go
	GOOS=linux GOARCH=arm GOARM=7 go build -o bin/main-arm-7 main.go
	GOOS=linux GOARCH=mips64 go build -o bin/main-mips64 main.go
