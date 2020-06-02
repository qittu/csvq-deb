GOPATH := $(shell pwd)/build
BINARY := csvq
PRERELEASE_ARCH := darwin/amd64 linux/amd64 windows/amd64

ifneq ($(shell command -v git && git remote -v 2>/dev/null | grep mithrandie/csvq.git && echo true),true)
	VERSION := $(shell git describe --tags --always 2>/dev/null)
endif

ifdef VERSION
	LDFLAGS := -ldflags="-X github.com/mithrandie/csvq/lib/query.Version=$(VERSION)"
endif

DIST_DIRS := find * -type d -exec

.DEFAULT_GOAL: $(BINARY)

$(BINARY): build

.PHONY: build
build:
	GOPATH=$(GOPATH) go build $(LDFLAGS) -o $(GOPATH)/bin/$(BINARY)

.PHONY: install
install:
	GOPATH=$(GOPATH) go install $(LDFLAGS)

.PHONY: clean
clean:
	GOPATH=$(GOPATH) go clean -i -cache -modcache

.PHONY: install-gox
install-gox:
ifeq ($(shell command -v gox 2>/dev/null),)
	go get github.com/mitchellh/gox
endif

.PHONY: build-all
build-all: install-gox
	GOPATH=$(GOPATH) gox $(LDFLAGS) -output="dist/${BINARY}-${VERSION}-{{.OS}}-{{.Arch}}/{{.Dir}}"

.PHONY: build-pre-release
build-pre-release: install-gox
	GOPATH=$(GOPATH) gox $(LDFLAGS) --osarch="$(PRERELEASE_ARCH)" -output="dist/${BINARY}-${VERSION}-{{.OS}}-{{.Arch}}/{{.Dir}}"

.PHONY: dist
dist:
	cd dist && \
	$(DIST_DIRS) cp ../LICENSE {} \; && \
	$(DIST_DIRS) cp ../BINARY_CODE_LICENSE {} \; && \
	$(DIST_DIRS) cp ../README.md {} \; && \
	$(DIST_DIRS) cp ../CHANGELOG.md {} \; && \
	$(DIST_DIRS) tar -zcf {}.tar.gz {} \; && \
	cd ..

.PHONY: release
release:
ifeq ($(shell git tag --contains 2>/dev/null),)
	$(error HEAD commit is not tagged)
else
	git push origin $(VERSION)
endif

.PHONY: install-goyacc
install-goyacc:
ifeq ($(shell command -v goyacc 2>/dev/null),)
	go get github.com/cznic/goyacc
endif

.PHONY: yacc
yacc: install-goyacc
	cd lib/parser && \
	goyacc -o parser.go -v parser.output parser.y && \
	cd ../../lib/json && \
	goyacc -p jq -o query_parser.go -v query_parser.output query_parser.y && \
	goyacc -p jp -o path_parser.go -v path_parser.output path_parser.y && \
	cd ../..

