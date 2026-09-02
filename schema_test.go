package mwanachamagit

import (
	"testing"

	"github.com/aosanya/mwanachama-go-shared/entitygraph"
)

// TestDefaultGitSchemaValidates guards against schema-authoring mistakes
// (dangling ToType references, missing inverse relationships, UniqueKey
// fields that don't match a declared property) that entitygraph.Publish
// would otherwise only catch at runtime.
func TestDefaultGitSchemaValidates(t *testing.T) {
	if err := entitygraph.ValidateSchema(DefaultGitSchema()); err != nil {
		t.Fatal(err)
	}
}
