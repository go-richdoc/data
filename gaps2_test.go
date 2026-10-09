// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package data_test

import (
	"strings"
	"testing"

	"github.com/go-richdoc/data/json"
	"github.com/go-richdoc/data/xml"
)

func TestACeilingReachedInsideANestedTablePropagates(t *testing.T) {
	// ⛔ The table is built deep inside the tree, and its refusal has to travel
	// back out. Without the error being carried through the recursion, the
	// enclosing list would come back holding nothing where the table should
	// have been — a document that is short by a table and says so nowhere.
	src := `<r><group><a><x>1</x></a><a><x>2</x></a><a><x>3</x></a></group></r>`
	if _, err := xml.Parse([]byte(src), xml.Options{MaxRows: 2}); err == nil {
		t.Error("a table past its ceiling, nested two levels down, was accepted")
	}
}

func TestSiblingsWithDifferentColumnCountsStayAList(t *testing.T) {
	// ⛔ Different NUMBER of columns, not different names. Comparing only the
	// names would index past the end of the shorter row; comparing only the
	// counts would mix columns up. Both are checked, and this is the half the
	// other test does not reach.
	src := `<r><a><x>1</x></a><a><x>2</x><y>3</y></a></r>`
	d, err := xml.Parse([]byte(src), xml.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := firstTable(d.Blocks); ok {
		t.Error("siblings with different column counts were put in one table")
	}
}

func TestACeilingInsideAJSONArrayPropagates(t *testing.T) {
	// The same question of the other tree reader: a refusal raised while
	// converting one element of an array has to stop the whole conversion
	// rather than leave a gap.
	src := `[{"a":{"b":{"c":{"d":1}}}}, 2, 3]`
	if _, err := json.Parse([]byte(src), json.Options{MaxDepth: 3}); err == nil {
		t.Error("a value past the depth ceiling inside an array was accepted")
	}
}

func TestAJSONObjectInsideAnArrayCarriesItsOwnRefusal(t *testing.T) {
	// And through the object path rather than the array path: the two
	// recursions are separate functions and each has to carry the error.
	src := `{"outer":[{"deep":{"deeper":{"deepest":1}}}]}`
	if _, err := json.Parse([]byte(src), json.Options{MaxDepth: 3}); err == nil {
		t.Error("a value past the depth ceiling inside an object's array was accepted")
	}
}

func TestADocumentOfOneHugeArrayIsStoppedByNodesNotRows(t *testing.T) {
	// Nodes and rows bound different things, and a file can reach either
	// first. This one is not a table at all, so only the node count can stop
	// it.
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < 200; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("1")
	}
	b.WriteString("]")
	if _, err := json.Parse([]byte(b.String()), json.Options{MaxNodes: 10}); err == nil {
		t.Error("an array of scalars past the node ceiling was built")
	}
}
