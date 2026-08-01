# dagreach — working conventions

## Language

**Everything committed to this repository is written in English** — code comments, docstrings,
README and docs, CLI output and error messages, commit messages, issues and pull requests.
Conversation with the owner happens in French; the artifacts never do.

## Scope discipline

`dagreach` answers one question: **what can this change reach, why, and should CI allow it?**
The metrics exist to support that decision; they are not the product.

- Inputs are files (DOT, JSON Graph Format). No database, no daemon, no runtime dependency on any
  orchestrator. Everything must work offline.
- The visualiser is an output format, never the product.
- Structural linting is deliberately out of scope until 1.0 ships.
- This project has no relationship with any other codebase. Do not import concepts, vocabulary or
  code from elsewhere: no gates, no evidence, no packets, no admission, no governance.

## Engineering rules

- **Go, standard library only.** The tool ships as a single static binary because the promise is
  "a file in, an answer out, no runtime" - and a Python runtime contradicted it. The port kept
  behaviour identical, verified by running both implementations over the same corpus and diffing
  their JSON and text output (43/43 identical) before the Python source was removed; that harness
  is the pattern to reuse for any rewrite.
- **Dependencies are a deliberate decision, not a reflex.** There are none. In Go they would be
  invisible to users, which makes the bar *lower*, not zero: a dependency still has to earn its
  place, and the reason gets written down here first.
- **Printed text is ASCII.** A Windows console on a legacy code page renders anything else as `?`.
  Docstrings and documentation are free; CLI output is not, and a test enforces it.
- **Exit codes are a public contract** (`0` ok, `1` a policy failed, `2` usage error, `4` the input
  could not be read; `3` is retired). CI pipelines depend on them; changing one is a breaking
  change. A broken selector or a missing file must never exit `1`: an unusable input must
  not be indistinguishable from a clean gate.
- **A format is only supported when it carries a graph.** DOT and JSON Graph Format do; YAML does
  not, so there is no generic YAML reader - only producer profiles that happen to read YAML. The
  same test applies to any future format: if reading it generically would require guessing what a
  node or an edge is, it becomes a profile or nothing.
- **A profile states what it decided.** An identifier it rewrote keeps the original as an
  attribute, a fallback it took is a warning, and detection itself is announced. `load` returns the
  producer's own orientation; the single reversal stays in `semantics.orient`.
- **Never guess which way an edge points.** The orientation applied is stated in every report, and
  a file whose shape contradicts it raises a warning. A graph read backwards produces confident,
  fluent nonsense - the worst failure this tool can have.
- **Every decision carries its evidence.** A policy verdict ships the matching nodes and a witness
  path. A gate that cannot explain itself gets disabled by the team that depends on it.
- **A recoverable oddity is a warning, never a failure and never a silence.** Readers record what
  they had to work around and the commands surface it. Anything accepted beyond a specification
  says so in the warning.
- The version lives in `internal/dagreach/cli.go` (`Version`) and nowhere else.
- Metric definitions must be documented and defensible. State the assumption in the output when one
  is made — e.g. an unweighted critical path is measured in edges, and must say so.
- **Measure before claiming a cost, and measure on a big graph.** Complexity reasoning misses what
  a profiler finds: a reach diff once rebuilt a set difference inside a per-node loop and took
  23 s where it now takes milliseconds, and the Go port shipped an insertion sort that made the
  analysis 25x slower than the code it replaced. Both were found by one large-graph run. Every
  performance claim in the docs is a measurement.
- Every slice keeps `gofmt -l .` empty and `go vet ./...` and `go test ./...` green on Linux,
  Windows and macOS.
