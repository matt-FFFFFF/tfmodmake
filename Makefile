.PHONY: build
build:
	go build ./cmd/tfmodmake

.PHONY: install
install:
	go install ./cmd/tfmodmake

.PHONY: test
test:
	go test -count=1 ./...

.PHONY: test-examples
test-examples:
	./scripts/test_examples.sh
