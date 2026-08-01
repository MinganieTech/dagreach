# Before this repository goes public

The repository is private until the work is complete and its owner has run a user acceptance test.
Everything below is what must be true on the day it flips, gathered as it came up rather than
invented at the end.

## Claims

- [ ] **Nothing promises what does not exist.** Today the README says "not released yet, build from
      a checkout" and the action is used as `uses: ./`. Both change the moment a release exists,
      and not before.
- [ ] **`Version` in `internal/dagreach/cli.go`** moves off `0.0.1` for the first tag.
- [ ] The repository description and topics say what the tool does, in the words people search for.

## Releasing

Two thresholds, and they are not the same day. **Public** needs only the claims above to be true —
the repository can be read, built and judged without a tag. **Released** is the promise that
somebody can depend on it, and that is the list below.

- [ ] A release workflow builds the static binary on tag for Linux, macOS and Windows, `amd64` and
      `arm64`, and attaches them. Cross-compilation is a matrix of `GOOS`/`GOARCH`; there are no
      dependencies to vendor.
- [ ] SHA-256 sums published beside the binaries.
- [ ] The action stops compiling from source and downloads the released binary instead, and the
      documented reference becomes a pinned tag rather than a branch.
- [ ] `go install github.com/MinganieTech/dagreach/cmd/dagreach@vX.Y.Z` is verified from a clean
      machine, since that is the path Go users will take first.
- [ ] `SECURITY.md`: where to report, and what an answer looks like.
- [ ] The first tag is `v0.1.0-rc.1`, not `v0.1.0`. A release candidate is what an external UAT is
      for, and 0.1.0 should mean somebody other than the author has run it.

## The user acceptance test

One real graph per producer, because the profiles are where a wrong answer is confident rather than
loud:

- [ ] a `terraform graph` from a live stack
- [ ] a `dbt` manifest from a real project
- [ ] a CycloneDX SBOM from a real scan
- [ ] one generic graph nobody wrote a profile for

For each: does `parse` load it, does the orientation line say the right thing, and does one policy
answer a question its owner already knows the answer to? That last one is the test — a gate is only
worth trusting where it agrees with somebody who knows.

## What a stranger will read

- [ ] `CLAUDE.md` is no longer tracked - it holds working conventions for the owner and the
      assistant, not documentation for readers. It remains in every commit that carried it, and the
      decision taken is to **rewrite the history to remove it before the repository goes public**:
      one pass with `git filter-repo`, cheap while the repository has one author and impossible to
      do cleanly afterwards. Anything in the file that readers *should* have belongs in `docs/`.

      Reviewed since, and worth recording: the file holds no secret and nothing about another
      project, so the rewrite is a preference about what the history reveals, not a necessity. It
      costs one command now and cannot be had later - which is the only reason to keep it on this
      list. Dropping it changes nothing else. The `Co-Authored-By: Claude` trailers are not a
      reason either way.
- [ ] The branches. `main` is the product; `spike/go-port` holds the Python-to-Go measurement and is
      worth keeping only if the reasoning is worth showing. Anything merged is deleted.
- [ ] History and pull requests carry no secrets and nothing about unrelated projects. Verified
      today: no such reference exists in any file, commit message, or branch.
- [ ] `examples/` is the front door for most readers. It is organised by outcome and every example
      runs in CI; keep it that way.

## Honest limits, stated rather than discovered

The README should say plainly, before anyone has to find out:

- [ ] dagreach reads graphs; it does not build them. Producing the file is the user's job, and the
      profiles cover four producers.
- [ ] It does not map changed files to changed nodes. You pass node ids.
- [ ] Reachability is structural: it reports what a change **can** reach, never what will execute.
- [ ] `UNKNOWN` catches an attribute the graph never declares, not one the graph declares unevenly.
      Stated in [policies.md](policies.md); it belongs in the README's limits too.
