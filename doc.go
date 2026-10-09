// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package data and its subpackages read CSV, JSON, XML and plain text into the
// neutral [github.com/go-richdoc/richdoc] document model, so that any of them
// can be typeset, written as Markdown or reST, or turned into an ODT by the
// converters that already exist around richdoc.
//
// # Why one repository rather than four
//
// This organisation's rule is one format, one converter. These four share
// something the document formats do not: none of them IS a document. A CSV is
// a grid, a JSON value is a tree, an XML document is a tree with attributes,
// and a text file is a run of bytes. Every one of them has to be given a shape
// it did not have, and the interesting decisions — when a tree becomes a table
// and when it becomes a list, what a header row is, what happens to a key with
// no value — are the same decisions four times over.
//
// Splitting them would copy those decisions into four repositories, where they
// would drift. They live in [github.com/go-richdoc/data/shape] instead.
//
// # Ceilings
//
// ⛔ Every one of these formats lets a small file ask for a large document: a
// CSV row can declare a million columns, a JSON array can nest ten thousand
// deep, an XML element can do the same. Each reader bounds what it will build
// and says which ceiling it hit — see [github.com/go-richdoc/data/shape] for
// the numbers and for what they are measured against.
package data
