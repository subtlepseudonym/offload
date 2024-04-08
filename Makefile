BINARY=offload

BUILD=$$(vtag)
REVISION=`git rev-list -n1 HEAD`
BUILDTAGS=
LDFLAGS=--ldflags "-X main.Version=${BUILD} -X main.Revision=${REVISION} -X \"main.BuildTags=${BUILDTAGS}\""
DOCKERTAG=dev-latest

default: all

all: test build

build: format
	go build ${LDFLAGS} -o ${BINARY} -v main.go

docker: format
	docker build --network=host --tag "subtlepseudonym/${BINARY}:${DOCKERTAG}" -f Dockerfile .

test:
	gotest --race ./...

format fmt:
	gofmt -l -w .

clean:
	go mod tidy
	go clean
	rm $(BINARY)

get-tag:
	echo ${BUILD}

.PHONY: all build test format fmt clean get-tag
