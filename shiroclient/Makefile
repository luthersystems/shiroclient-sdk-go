.PHONY: default
default:
	@

.PHONY:
go-test:
	ssh-add -l >/dev/null
	CGO_LDFLAGS_ALLOW=-I/usr/local/share/libtool go test -race -cover -v ./...
