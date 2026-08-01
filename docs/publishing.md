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

- [ ] A release workflow builds the static binary for Linux, macOS and Windows on tag, and attaches
      them. Cross-compilation is a matrix of `GOOS`/`GOARCH`; there are no dependencies to vendor.
- [ ] The action stops compiling from source and downloads the released binary instead, and the
      documented reference becomes a pinned tag rather than a branch.
- [ ] `go install github.com/MinganieTech/dagreach/cmd/dagreach@vX.Y.Z` is verified from a clean
      machine, since that is the path Go users will take first.

## What a stranger will read

- [ ] `CLAUDE.md` is no longer tracked - it holds working conventions for the owner and the
      assistant, not documentation for readers. It remains in every commit that carried it, and the
      decision is taken: **the history will be rewritten to remove it before the repository goes
      public.** That is a single pass with `git filter-repo`, cheap while the repository has one
      author and impossible to do cleanly afterwards, so it happens before the flip and not after.
      Anything in the file that readers *should* have belongs in `docs/` instead.
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
