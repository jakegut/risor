
.PHONY: bench
bench:
	go build
	./risor -profile cpu.out ./benchmark/main.mon
	go tool pprof -http=:8080 ./cpu.out

# https://code.visualstudio.com/api/working-with-extensions/publishing-extension#packaging-extensions
.PHONY: install-tools
install-tools:
	npm install -g vsce

.PHONY: extension-login
extension-login:
	cd vscode && vsce login $(VSCE_LOGIN)

.PHONY: extension
extension:
	cd vscode && vsce package && vsce publish

.PHONY: postgres
postgres:
	docker run --rm --name pg -p 5432:5432 -e POSTGRES_PASSWORD=pwd -d postgres

.PHONY: test
test:
	go test -coverprofile cover.out ./...
	go tool cover -html=cover.out

.PHONY: lambda
lambda:
	mkdir -p dist
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o dist/risor-lambda ./cmd/risor-lambda
	zip -j dist/risor-lambda.zip dist/risor-lambda
	aws s3 cp dist/risor-lambda.zip s3://test-506282801638/dist/risor-lambda.zip
