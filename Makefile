.PHONY: build test

build:
	go build ./...

test:
	go test ./...

all: bin/sqlc-gen-kotlin bin/sqlc-gen-kotlin.wasm

# -trimpath keeps absolute build paths out of the binary, so the same commit
# built from two different checkouts produces byte-identical output. That is what
# lets a published release artifact be reproduced and verified -- scripts/release.sh
# ships whatever this target builds, so the flags must live in one place.
GO_BUILD_FLAGS := -trimpath -ldflags "-s -w"

bin/sqlc-gen-kotlin: bin go.mod go.sum $(wildcard **/*.go)
	cd plugin && CGO_ENABLED=0 go build $(GO_BUILD_FLAGS) -o ../bin/sqlc-gen-kotlin ./main.go

bin/sqlc-gen-kotlin.wasm: bin/sqlc-gen-kotlin
	cd plugin && CGO_ENABLED=0 GOOS=wasip1 GOARCH=wasm go build $(GO_BUILD_FLAGS) -o ../bin/sqlc-gen-kotlin.wasm main.go

bin:
	mkdir -p bin

