// Package api — codegen entry point.
//
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 -package gen -generate types,chi-server,strict-server -o gen/openapi.gen.go openapi.yaml
package api
