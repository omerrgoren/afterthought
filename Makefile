.PHONY: build test race vet smoke check install dist
build:
	go build -o bin/afterthought ./cmd/afterthought
test:
	go test ./...
race:
	go test -race ./...
vet:
	go vet ./...
smoke: build
	./scripts/smoke.sh ./bin/afterthought
check: vet race smoke
install:
	go install ./cmd/afterthought
dist:
	goreleaser release --snapshot --clean
