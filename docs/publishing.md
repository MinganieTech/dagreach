# Before this repository goes public

The repository is private until the work is complete and its owner has run a user acceptance test.
Everything below is what must be true on the day it flips, gathered as it came up rather than
invented at the end.

## Claims

- [x] **Nothing promises what does not exist.** The README states the release candidate status and
      the four known limits; the documented action reference is a pinned tag.
- [x] **`Version` in `internal/dagreach/cli.go`** is `0.1.0-rc.1`, and the release workflow refuses
      a tag that disagrees with it.
- [ ] `.github/release-notes.md` still says "release candidate before 1.0". It is the body of every
      release, so it is the thing to revisit when that stops being true.
- [ ] The repository description and topics say what the tool does, in the words people search for.

## Releasing

Two thresholds, and they are not the same day. **Public** needs only the claims above to be true —
the repository can be read, built and judged without a tag. **Released** is the promise that
somebody can depend on it, and that is the list below.

- [x] A release workflow builds the static binary on tag for Linux, macOS and Windows, `amd64` and
      `arm64`, and attaches them. It refuses a tag whose version disagrees with the binary, and
      refuses to overwrite a release that already exists.
- [x] SHA-256 sums published beside the binaries as `checksums.txt`.
- [x] The action downloads the released binary, checks it against that release's `checksums.txt`,
      and no longer installs Go. The documented reference is a pinned tag.
- [ ] **The action's download path has never run.** It cannot, before a release exists: the example
      gate now exercises the CLI from source instead. First thing to verify once the tag is cut -
      on Linux, macOS and Windows, since the archive format and the checksum tool differ by
      platform.
- [ ] `go install github.com/MinganieTech/dagreach/cmd/dagreach@vX.Y.Z` is verified from a clean
      machine, since that is the path Go users will take first.
- [x] `SECURITY.md`: where to report, what is in scope, and what an answer looks like. The
      five-business-day acknowledgement is a commitment the owner has to be able to keep.
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

Said plainly in the README's status block and in the release notes, before anyone has to find out:

- [x] dagreach reads graphs; it does not build them. Producing the file is the user's job, and the
      profiles cover four producers.
- [x] It does not map changed files to changed nodes. You pass node ids.
- [x] Reachability is structural: it reports what a change **can** reach, never what will execute.
- [x] `UNKNOWN` catches an attribute the graph never declares, not one the graph declares unevenly.

The release notes add two more, which belong to a reader choosing whether to trust a number: the
ranking is not a dominator ranking, and the HTML report is not an explorer.

## Not blocking, and worth doing anyway

- [x] `CHANGELOG.md`.
- [ ] Provenance attestations (`actions/attest-build-provenance`) and Sigstore signatures on the
      archives. Both are one step in the release workflow, and both are worth more once anybody
      depends on the binaries than they are today.
- [ ] An SBOM for the released binaries - dagreach has no dependencies, so it would describe one
      component and the Go toolchain, which is honest but thin.
