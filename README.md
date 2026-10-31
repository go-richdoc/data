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

⛔ **One value, and said out loud when there are more** (v0.2.0). `Decoder.Decode`
reads one value and leaves the rest of the stream alone, so `{"a":1} {"b":2}` came
back as a document holding the first object and **no error at all** — a file
converted to half of itself, in silence, which is the same kind of loss as
rendering `null` as the empty string. A reader whose honest answer is "some of your
file" has to say so.

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
| **cells in a table** | **500 000** |
| nesting | 100 |
| values in a tree | 500 000 |
| **values in JSON source** | **1 000 002**, counted before decoding |

Depth and breadth are different ways to be large, and bounding only one leaves
the other. Each refusal says **which** ceiling and what it is, because the value
of a refusal is that the reader can tell a hostile file from a legitimately
enormous one.

### ⛔ A ceiling can stop the answer and not the work

How many bytes of heap does one byte of source buy? Nobody had measured it, and
the ceilings above are on what gets **built**, which is not the same question.
At each format's cheapest shape, per byte of source:

| | allocated | retained |
| --- | --- | --- |
| CSV, 1 000 columns of one character | **60×** | 49× |
| JSON, `[1,1,1,…]` | **176×** | 62× |
| XML, `<a/><a/>…` | **148×** | 43× |
| text, one-character lines | 63× | 0.8× |

Then the measurement that mattered. Two flat JSON arrays, the second **eight
times** the source of the first, both refused by the same ceiling at the same
node — so the reader was supposed to have stopped at the same byte in both. They
cost **243 MB** and **1.275 GB**: a ratio of 5.2. `encoding/json` decodes the
whole value into an `any` *before* the converter counts its first node, so the
node ceiling was a refusal that arrived after the allocation it exists to
prevent. The same test on the XML reader, which counts inside its own token
walk, gave **1.0** — which is what makes it a measurement rather than the
observation that a bigger file allocates more.

The fix is a pass over the source bytes that counts **values** before anything
is decoded. A hostile 16 MiB array is now refused in **40 bytes**.

⛔ **The fuzz target broke the first draft of it in 700 milliseconds**, and the
minimised input is worth keeping: `0 0 0[0[0[0`. Nine values to a scan of the
file; **one** value to `Decoder.Decode`, which reads one and leaves the stream
alone. So the guard refused a file the converter accepts — and a guard that
refuses what the thing it protects would accept is worse than no guard. It now
stops where the decoder stops.

That is also how `Parse` came to notice it was **returning half a file in
silence**: `{"a":1} {"b":2}` gave a document holding the first object and no
error. It now says so. ⛔ The check reads one more **token** and asks for
`io.EOF`, not `dec.More()` — `More` peeks for a non-space byte and answers false
on a `]` or a `}`, so `1]` would have walked straight through the check written
to catch it.

The ceiling constant is derived, not chosen: an array of *n* single-key objects is the thinnest shape
there is — `1+2n` values in the source against `1+n` nodes in the converter,
because `table()` visits the cells and not the objects holding them — so
`MaxSourceValues = 2·MaxNodes + 2` can only ever refuse a file the converter was
going to refuse anyway. A test asserts that relation, and a fuzz target hunts
for a counterexample to it.

⛔ Nesting needs no such pass. `encoding/json` refuses past
`maxNestingDepth = 10000` (`encoding/json/scanner.go:148`, and the same constant
at `encoding/json/jsontext/state.go:53`, so `GOEXPERIMENT=jsonv2` does not
change it) — the decode of a value into an `any` *does* recurse, one frame per
level, and it is that constant and not iteration that keeps it bounded.

### ⛔ Two ceilings that multiply

`MaxRows` and `MaxColumns` bounded the factors and nothing bounded the product:
100 000 rows of 1 000 columns is within both and is a **hundred million** cells,
measured at 120 bytes allocated each — 12 GB from a file the input ceiling
allows. The number this closed with is the tree readers' own `MaxNodes`, because
it is **the same quantity under another name**. The defect was an
*inconsistency*: an export of 100 000 rows by 20 fields was already refused as
JSON at 2 000 000 nodes and accepted as CSV at 2 000 000 cells. One repository
answering two ways about one quantity of data is what `shape` exists to prevent,
and a test now asks both readers the same question. `Options.MaxCells` restores
the old ceiling for a caller who wants it.

⛔ The count is of the cells **built**, not the cells read: ragged rows are
squared off to the widest one, so a file of one 1 000-column row and six hundred
one-column rows is 601 000 cells from 1 600 read. A count of what was read would
undercount exactly the file that exploits it.

## Checks

**100 %** statement coverage — measured with `-coverpkg=./...`, because
`go test ./...` attributes coverage only to a package's *own* tests, so the
cross-package tests here counted for nothing and the repository read as 95 %
when it was already at 100 %. `-race` green, `go vet` and `gofmt` clean, **ten**
build targets including `js/wasm`, `loong64` and `s390x`.

**Five fuzz targets** — one per reader, plus one on the agreement between the
pre-scan and the converter. ⛔ Each asserts a property of the **output**, not
the absence of a panic: a document XOR an error, nothing built deeper than the
reader's own ceiling allows, and the same bytes twice giving the same document
(Go randomises map iteration on purpose, and a conversion that differs between
two runs makes every diff of two conversions meaningless).

Twelve mutations against the decisions above. ⛔ Four survived the first round
and each named a real hole: two fixtures where the delimiter sniffer's two rules
happened to agree, and two where the text reader's two signals did. **A rule
needs a case where it alone decides.**

## Licence

BSD-3-Clause.
