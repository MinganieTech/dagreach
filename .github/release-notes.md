**Portable change-impact analysis for dependency graphs.** See what a change can reach, why it
reaches it, and whether CI should allow it.

## Status

This is a **release candidate before 1.0**. The command line, the exit codes and the JSON report are
meant to be stable, and a breaking change to any of them will bump the schema version or the major
version — but nothing here has been used in anger by anyone outside the project yet. That is what a
release candidate is for. Report what breaks.

## Install

Download the archive for your system below, or:

```bash
go install github.com/MinganieTech/dagreach/cmd/dagreach@latest
```

Every archive holds one static binary: no runtime, no dependencies, nothing to install alongside it.
`checksums.txt` carries the SHA-256 of every archive in this release.

```bash
sha256sum --check --ignore-missing checksums.txt
```

## What it does

| Command | Answers |
|---|---|
| `impact` | what a change reaches, over which path, and whether a policy refuses it |
| `diff` | which reach relationships a pull request made possible that were not |
| `stats` | the shape of the graph: depth, width, cycles, and what each node holds up |
| `parse` | does my export even load, and how was it read |

Reads DOT and JSON Graph Format, with profiles for `terraform graph`, dbt manifests and CycloneDX
SBOMs. Reports as text, `--json`, `--markdown` or `--html`.

Exit codes are the contract: `0` ok, `1` a policy failed, `2` usage, `3` a policy this graph could
not settle, `4` unreadable input.

## Known limits

- **It reads graphs; it does not build them.** Producing the file is your job. Four producers are
  covered by a profile; everything else is DOT or JSON Graph Format plus `--edge-semantics`.
- **It does not map changed files to changed nodes.** You pass node ids. Deriving them from a diff
  is producer-specific, and doing it generically would be guessing.
- **Reachability is structural.** It reports what a change *can* reach, never what will execute.
- **`UNKNOWN` tests presence, not coverage.** A selector on an attribute no node declares cannot be
  settled and exits `3`; an attribute declared by *some* nodes settles normally, so a partially
  classified export can pass.
- **The ranking is not a dominator ranking.** "Reaches 286" means 286 sit behind it, not that 286
  become unreachable without it.
- **The HTML report is a report, not an explorer.** It draws no graph and runs no script.
