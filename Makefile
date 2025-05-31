GIT_VERSION := `git describe --abbrev=0 --tags`
GIT_COMMIT := `git rev-list -1 HEAD`
GIT_DATE := `git log -1 --date=format:"%Y.%m.%dT%T" --format="%ad"`

.PHONY: version
version:
	@echo "GIT_COMMIT: $(GIT_COMMIT)" &&\
	echo "GIT_VERSION: $(GIT_VERSION)" &&\
	echo "GIT_DATE:" $(GIT_DATE)

.PHONY: build
build: version
	@echo "Building..." &&\
	test -d build || mkdir build &&\
	set GOARCH=amd64&& set GOOS=windows&& set GODEBUG=cgocheck=0&& go build -ldflags "-X main.version=$(GIT_VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.gitDate=$(GIT_DATE)" -v -o build/iso2repo.exe main.go &&\
	GOARCH=amd64 GOOS=linux GODEBUG=cgocheck=0 go build -ldflags "-X main.version=$(GIT_VERSION) -X main.gitCommit=$(GIT_COMMIT) -X main.gitDate=$(GIT_DATE)" -v -o build/iso2repo main.go

.PHONY: run
run: build
	@echo "Running..." &&\
	build/iso2repo.exe

.PHONY: clean
clean:
	@echo "Cleaning..." &&\
	rm -f -R build
