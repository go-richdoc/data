# data

**CSV, JSON, XML and plain text into the neutral
[richdoc](https://github.com/go-richdoc/richdoc) document model.** Pure Go,
CGO-free, including `GOOS=js`.

```sh
richdata sales.csv                        # Markdown on standard output
richdata -format json -title Orders o.json
richdata -o notes.md -verbatim server.log
```

```go
doc, err := csv.Parse(src, csv.Options{})   // -> *richdoc.Document
doc, err := json.Parse(src, json.Options{})
doc, err := xml.Parse(src, xml.Options{})
doc, err := text.Parse(src, text.Options{})
```

From there every converter already around `richdoc` applies: typeset it with
[`latex/pdf`](https://github.com/go-richdoc/latex), write it as
[Markdown](https://github.com/go-richdoc/markdown) or
[reST](https://github.com/go-richdoc/rst), or save it as an
[ODT](https://github.com/go-odf/odf).

## Why one repository and not four

This organisation's rule is *one format, one converter*. These four share
something the document formats do not: **none of them is a document.** A CSV is
a grid, a JSON value is a tree, an XML document is a tree with attributes, a
text file is a run of bytes. Each has to be given a shape it did not have — and
the interesting decisions are the same decisions four times over. They live in
`shape/` rather than in four repositories where they would drift.

## The decisions, and why each has a test

### CSV: the delimiter is sniffed, the header is not

⛔ **A file called `.csv` is as likely to be semicolon-separated.** That is what
a spreadsheet writes anywhere the comma is a decimal point, and a reader that
assumes a comma turns every row into one column — with no error.

The sniffer counts **outside quotes** and prefers the delimiter whose count is
the **same on every line**. Steadiness, not frequency: "the most common
character" picks the comma out of English prose every time.

⛔ **What it will not guess is the header.** Promoting a row of data *loses* it
and a reader cannot get it back; leaving a header as data is ugly and
recoverable. So `Detect` promotes only on evidence — first row all text and
non-numeric, second row holding a number — and `Always`/`Never` decide when you
know.

Ragged rows are **squared off, not refused**: exports are ragged all the time,
and every consumer of `richdoc` expects a rectangle.

### JSON: an array of uniform objects is a table

⛔ That is the shape almost every export has, and rendering it as six hundred
nested bullet lists produces something nobody can read out of data that was a
grid all along. The test is **strict** — every element an object, same keys, no
nested values — because a table built from "most of them agree" silently drops
whatever the odd element held, and the odd element is the interesting one.

⛔ **`UseNumber` is load-bearing.** The default turns every number into a
`float64`, so an identifier past fifteen digits comes back with its last digits
replaced by zeros. Nothing errors; the document is simply wrong in a way that
looks plausible.

⛔ **`null` is not `""`**, and an empty object is not nothing. Keys are sorted,
because Go randomises map iteration on purpose and two conversions of one file
must agree, or no diff of two documents means anything.

### XML: repeated siblings are a table, attributes are columns

The same decision as JSON, for the same reason. An `isbn` is the identifying
fact of a book; a table of titles and years with nothing to join on is not the
document anybody wanted.

⛔ This renders XML **as data**. It does not know what any vocabulary means: an
ODT body belongs to [`go-odf/odf`](https://github.com/go-odf/odf). Handed one,
this faithfully renders the *markup* — the right answer to the question asked
and the wrong answer to the question probably meant.

### Text: a blank line, and nothing else, is assumed

⛔ It does **not** guess that an underlined line is a heading or that a dash
starts a list. Those are Markdown's rules, and a file that wanted them should be
read by the Markdown reader.

What it does keep is text laid out **on purpose**: a run of lines that are
indented, or that hold runs of two or more spaces, stays verbatim. A table drawn
with spaces, a log, a column of figures — every naive reader flows them into a
paragraph and the file becomes nonsense, with nothing anywhere to say so.

Invalid UTF-8 is **repaired, not refused**: half the world's `.txt` is Latin-1,
and a run of replacement characters is visible, which is what somebody needs in
order to notice and re-save it.

## Ceilings

⛔ Every one of these formats lets a small file ask for a large document.

| | |
| --- | --- |
| input | 64 MiB |
| rows / columns | 100 000 / 1 000 |
| nesting | 100 |
| values in a tree | 500 000 |

Depth and breadth are different ways to be large, and bounding only one leaves
the other. Each refusal says **which** ceiling and what it is, because the value
of a refusal is that the reader can tell a hostile file from a legitimately
enormous one.

## Checks

**100 %** statement coverage — measured with `-coverpkg=./...`, because
`go test ./...` attributes coverage only to a package's *own* tests, so the
cross-package tests here counted for nothing and the repository read as 95 %
when it was already at 100 %. `-race` green, `go vet` and `gofmt` clean, **ten**
build targets including `js/wasm`, `loong64` and `s390x`.

Twelve mutations against the decisions above. ⛔ Four survived the first round
and each named a real hole: two fixtures where the delimiter sniffer's two rules
happened to agree, and two where the text reader's two signals did. **A rule
needs a case where it alone decides.**

## Licence

BSD-3-Clause.
