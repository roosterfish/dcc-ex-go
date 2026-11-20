static:
ifeq ($(shell command -v golangci-lint 2> /dev/null),)
	curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $$HOME/go/bin latest
endif
	golangci-lint run

unit:
	go test ./...

integration:
	go test -v -tags=integration ./...
