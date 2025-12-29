.PHONY: build
build:
	TMPDIR=/tmp GOTMPDIR=/tmp go build ./cmd/tfmodmake

.PHONY: install
install:
	TMPDIR=/tmp GOTMPDIR=/tmp go install ./cmd/tfmodmake

.PHONY: test
test:
	TMPDIR=/tmp GOTMPDIR=/tmp go test -count=1 ./...

.PHONY: test-examples
test-examples:
	TMPDIR=/tmp bash ./scripts/test_examples.sh
