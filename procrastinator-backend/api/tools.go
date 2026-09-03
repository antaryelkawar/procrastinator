//go:build tools

// Package api — pinned codegen tool dependencies.
//
// This file ensures the codegen tools are recorded as direct dependencies
// in go.mod so that `go:generate` invocations are reproducible.
package api

import (
	_ "github.com/oapi-codegen/oapi-codegen/v2/pkg/codegen"
)
