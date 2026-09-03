# Pinned codegen tool versions and flags.
# Generation is deterministic: same pinned version + same input document = same output.

OAPI_CODEGEN_VERSION := v2.8.0
OAPI_CODEGEN_PKG := gen
OAPI_CODEGEN_GENERATE := types,chi-server
OPENAPI_TYPESCRIPT_VERSION := 7.13.0
REDOC_CLI_VERSION := 0.13.21

OPENAPI_DOC := procrastinator-backend/api/openapi.yaml
GEN_DIR := procrastinator-backend/api/gen
DOCS_DIR := procrastinator-backend/api/docs

.PHONY: codegen codegen-go codegen-ts docs codegen-drift-check budget-check

## codegen: Run all code generators (Go server types, TS client types, docs).
codegen: codegen-go codegen-ts docs

## codegen-go: Generate Go server types + chi router wiring from openapi.yaml.
codegen-go:
	cd procrastinator-backend && go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION) -package $(OAPI_CODEGEN_PKG) -generate $(OAPI_CODEGEN_GENERATE) -o api/gen/openapi.gen.go api/openapi.yaml

## codegen-ts: Generate TypeScript client types from openapi.yaml.
codegen-ts:
	cd ui && npx openapi-typescript@$(OPENAPI_TYPESCRIPT_VERSION) "$(abspath $(OPENAPI_DOC))" --output src/lib/api/generated/paths.d.ts

## docs: Generate static API documentation (single-file HTML) from openapi.yaml.
## redoc-cli bundles the spec into a self-contained, zero-dependency HTML file.
## For a pinned redoc version + input document the output is byte-for-byte
## stable (no volatile timestamps/version banners are embedded), so the
## committed file can be drift-checked.
docs:
	mkdir -p $(DOCS_DIR)
	npx redoc-cli@$(REDOC_CLI_VERSION) build $(OPENAPI_DOC) --output $(DOCS_DIR)/index.html

## codegen-drift-check: Re-run all generators and verify committed artifacts match.
## (Go + TypeScript generators.)
codegen-drift-check:
	cd procrastinator-backend && go test ./api/gen/ -run TestCodegenDrift -count=1
	cd ui && npx vitest run src/lib/api/codegen.drift.test.ts

## budget-check: Assert codegen performance and size budgets (tasks 7.1/7.2/7.3).
budget-check:
	cd procrastinator-backend && go test ./api/gen/ -run 'TestFullCodegenWithinTimeBudget|TestDriftCheckWithinTimeBudget|TestOpenAPIDocumentWithinSizeBudget' -count=1
