# Empacotamento das funções para o runtime provided.al2023.
#
# Duas exigências do runtime custom, e as duas falham em silêncio se erradas:
# o binário DENTRO do zip precisa se chamar `bootstrap`, e a arquitetura
# precisa casar com a declarada na função. Usamos arm64 (Graviton): ~20% mais
# barato e mais rápido que x86 na Lambda.
FUNCOES := auth-token auth-authorizer
GOOS    := linux
GOARCH  := arm64

.PHONY: build test lint limpar
.DEFAULT_GOAL := build

build: $(addprefix dist/,$(addsuffix .zip,$(FUNCOES)))

dist/%.zip: cmd/%/main.go $(shell find internal -name '*.go' 2>/dev/null)
	@mkdir -p dist/$*
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -trimpath -ldflags="-s -w" -o dist/$*/bootstrap ./cmd/$*
	cd dist/$* && zip -q -X ../$*.zip bootstrap
	@echo "dist/$*.zip  $$(du -h dist/$*.zip | cut -f1)"

test:
	go test ./... -race -cover

lint:
	go vet ./...
	gofmt -l .

limpar:
	rm -rf dist
