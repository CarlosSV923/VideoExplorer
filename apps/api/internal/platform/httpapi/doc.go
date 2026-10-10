// Package httpapi holds the HTTP contract generated from api/openapi.yaml
// (models and the strict server interface). Bounded contexts implement
// StrictServerInterface; this package never holds hand-written handlers.
package httpapi

//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../../../../api/openapi.yaml
