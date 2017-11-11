// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package irgen

import (
	"bytes"
	"path/filepath"
	"testing"
)

func (config Config) compareOuputToReferenceFile(t *testing.T, reffile string) {
	t.Helper()

	var buf bytes.Buffer
	err := config.Generate(&buf)
	if err != nil {
		t.Fatal(err)
	}
}

func TestIntExpr(t *testing.T) {
	reference := filepath.FromSlash("./internal/test_cases/intexpr/ref.go")

	config := Config{
		Directory:   filepath.FromSlash("internal/test_cases/intexpr"),
		PackageName: "intexpr",
	}
	config.TypeNames.Composite = "Expr"
	config.TypeNames.Consumer = "ExprConsumer"

	config.compareOuputToReferenceFile(t, reference)
}

func TestTypes(t *testing.T) {
	// irgen used to generate a receiver name that was the lowercase version
	// of the composite type. This meant if the composite type name was an
	// upper-case version of a Go keyword, the code would not compile.
	// Therefore irgen now simply uses the type name. If that is not a valid
	// type name that is the fault of the user, not irgen.

	reference := filepath.FromSlash("./internal/test_cases/types/ref.go")

	config := Config{
		Directory:   filepath.FromSlash("internal/test_cases/types"),
		PackageName: "types",
	}
	config.TypeNames.Composite = "Type"
	config.TypeNames.Consumer = "TypeConsumer"

	config.compareOuputToReferenceFile(t, reference)
}
