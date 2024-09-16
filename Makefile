export DOCKER_BUILDKIT=1
LDFLAG_LOCATION := github.com/metal-toolbox/alloy/internal/version
GIT_COMMIT  := $(shell git rev-parse --short HEAD)
GIT_BRANCH  := $(shell git symbolic-ref -q --short HEAD)
GIT_SUMMARY := $(shell git describe --tags --dirty --always)
VERSION     := $(shell git describe --tags 2> /dev/null)
BUILD_DATE  := $(shell date +%s)
GIT_COMMIT_FULL  := $(shell git rev-parse HEAD)
GO_VERSION := $(shell expr `go version |cut -d ' ' -f3 |cut -d. -f2` \>= 16)
DOCKER_IMAGE_INBAND  := "ghcr.io/metal-toolbox/alloy-inband"
DOCKER_IMAGE  := "ghcr.io/metal-toolbox/alloy"
REPO := "https://github.com/metal-toolbox/alloy.git"

.DEFAULT_GOAL := help

## lint
lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@v1.56.2 run --config .golangci.yml --timeout=2m

## Go test
test: lint
	go test -timeout 1m -v -covermode=atomic ./...

## build osx bin
build-osx:
ifeq (${GO_VERSION}, 0)
	$(error build requies go version 1.17.n or higher)
endif
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o alloy \
		-ldflags \
		"-X ${LDFLAG_LOCATION}.GitCommit=${GIT_COMMIT} \
		 -X ${LDFLAG_LOCATION}.GitBranch=${GIT_BRANCH} \
		 -X ${LDFLAG_LOCATION}.GitSummary=${GIT_SUMMARY} \
		 -X ${LDFLAG_LOCATION}.Version=${VERSION} \
		 -X ${LDFLAG_LOCATION}.BuildDate=${BUILD_DATE}"

## Build linux bin
build-linux:
ifeq (${GO_VERSION}, 0)
	$(error build requies go version 1.16.n or higher)
endif
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o alloy \
		-ldflags \
		"-X ${LDFLAG_LOCATION}.GitCommit=${GIT_COMMIT} \
		 -X ${LDFLAG_LOCATION}.GitBranch=${GIT_BRANCH} \
		 -X ${LDFLAG_LOCATION}.GitSummary=${GIT_SUMMARY} \
		 -X ${LDFLAG_LOCATION}.Version=${VERSION} \
		 -X ${LDFLAG_LOCATION}.BuildDate=${BUILD_DATE}"

## build docker image and tag as ghcr.io/metal-toolbox/alloy:latest
build-image: build-linux
	@echo ">>>> NOTE: You may want to execute 'make build-image-nocache' depending on the Docker stages changed"
	docker build --rm=true -f Dockerfile -t ${DOCKER_IMAGE}:latest . \
		--label org.label-schema.schema-version=1.0 \
		--label org.label-schema.vcs-ref=${GIT_COMMIT_FULL} \
		--label org.label-schema.vcs-url=${REPO}

## build docker image and tag as ghcr.io/metal-toolbox/alloy-inband:latest
build-image-inband: build-linux
	@echo ">>>> NOTE: You may want to execute 'make build-image-nocache' depending on the Docker stages changed"
	docker build --rm=true -f Dockerfile.inband -t ${DOCKER_IMAGE_INBAND}:latest . \
		--label org.label-schema.schema-version=1.0 \
		--label org.label-schema.vcs-ref=${GIT_COMMIT_FULL} \
		--label org.label-schema.vcs-url=${REPO}

## build docker image, ignoring the cache
build-image-inband-nocache: build-linux
	docker build --no-cache --rm=true -f Dockerfile.inband -t ${DOCKER_IMAGE_INBAND}:latest . \
		--label org.label-schema.schema-version=1.0 \
		--label org.label-schema.vcs-ref=${GIT_COMMIT_FULL} \
		--label org.label-schema.vcs-url=${REPO}

## push devel docker image
push-image-devel: build-image
	docker tag ${DOCKER_IMAGE}:latest localhost:5001/alloy:latest
	docker push localhost:5001/alloy:latest
	kind load docker-image localhost:5001/alloy:latest

## push docker image
push-image:
	docker push ${DOCKER_IMAGE_INBAND}:latest


# https://gist.github.com/prwhite/8168133
# COLORS
GREEN  := $(shell tput -Txterm setaf 2)
YELLOW := $(shell tput -Txterm setaf 3)
WHITE  := $(shell tput -Txterm setaf 7)
RESET  := $(shell tput -Txterm sgr0)


TARGET_MAX_CHAR_NUM=20
## Show help
help:
	@echo ''
	@echo 'Usage:'
	@echo '  ${YELLOW}make${RESET} ${GREEN}<target>${RESET}'
	@echo ''
	@echo 'Targets:'
	@awk '/^[a-zA-Z\-\\_0-9]+:/ { \
		helpMessage = match(lastLine, /^## (.*)/); \
		if (helpMessage) { \
			helpCommand = substr($$1, 0, index($$1, ":")-1); \
			helpMessage = substr(lastLine, RSTART + 3, RLENGTH); \
			printf "  ${YELLOW}%-${TARGET_MAX_CHAR_NUM}s${RESET} ${GREEN}%s${RESET}\n", helpCommand, helpMessage; \
		} \
	} \
	{ lastLine = $$0 }' ${MAKEFILE_LIST}
