COMMANDS=kperf
CONTRIB_COMMANDS=runkperf

# PREFIX is base path to install.
PREFIX ?= /usr/local

GO_BUILDTAGS = -tags "osusergo netgo static_build"

# IMAGE_REPO is default repo for image-build recipe.
IMAGE_REPO ?= localhost:5000
IMAGE_TAG ?= latest
IMAGE_NAME = $(IMAGE_REPO)/kperf:$(IMAGE_TAG)
GO_VERSION := $(strip $(shell cat .go-version))

BINARIES=$(addprefix bin/,$(COMMANDS))
CONTRIB_BINARIES=$(addprefix bin/contrib/,$(CONTRIB_COMMANDS))

# default recipe is build
.DEFAULT_GOAL := build

# Always build
ALWAYS:

bin/%: cmd/% ALWAYS
	@echo $@
	@CGO_ENABLED=0 go build -o $@ ${GO_BUILDTAGS} ./$<

bin/contrib/%: contrib/cmd/% ALWAYS
	@echo $@
	@CGO_ENABLED=0 go build -o $@ ${GO_BUILDTAGS} ./$<

build: $(BINARIES) $(CONTRIB_BINARIES) ## build binaries
	@echo "$@"

install: ## install binaries
	@install -d $(PREFIX)/bin
	@install $(BINARIES) $(PREFIX)/bin
	@install $(CONTRIB_BINARIES) $(PREFIX)/bin

image-build: ## build image
	@echo building ${IMAGE_NAME}
	@docker build --build-arg GO_VERSION=${GO_VERSION} . -t ${IMAGE_NAME}

image-push: image-build ## push image
	@echo pushing ${IMAGE_NAME}
	@docker push ${IMAGE_NAME}

image-clean: ## clean image
	@echo cleaning ${IMAGE_NAME}
	@docker rmi ${IMAGE_NAME}

test: ## run test
	@go test -v ./...

lint: ## run lint
	@golangci-lint run --config .golangci.yml

.PHONY: clean
clean: ## clean up binaries
	@rm -f $(BINARIES)
	@rm -f $(CONTRIB_BINARIES)

.PHONY: help
help: ## this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-36s\033[0m%s\n", $$1, $$2}' $(MAKEFILE_LIST)
