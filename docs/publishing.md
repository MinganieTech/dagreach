# Publishing

Written while the repository was private, as the list of what had to be true on the day it went
public. It is kept as the record of what was decided and what is still owed: a checklist thrown
away once it is ticked teaches nobody why.

## Claims

- [x] **Nothing promises what does not exist.** The README states the release candidate status and
      the four known limits; the documented action reference is a pinned tag.
- [x] **`Version` in `internal/dagreach/cli.go`** is `0.1.0-rc.1`, and the release workflow refuses
      a tag that disagrees with it.
- [ ] `.github/release-notes.md` still says "release candidate before 1.0". It is the body of every
      release, so it is the thing to revisit when that stops being true.
- [x] The repository description and topics say what the tool does, in the words people search for.

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

**Deliberately not a gate on the release.** The owner's decision, taken with the release candidate
in hand: a repository nobody has been told about is a safer place to find out than a checklist, and
anything the real graphs turn up is addressed as a fix on top of `rc.1` rather than held for before
it.

The graphs still worth running it against, because the profiles are where a wrong answer is
confident rather than loud:

- a `terraform graph` from a live stack
- a `dbt` manifest from a real project
- a CycloneDX SBOM from a real scan
- one generic graph nobody wrote a profile for

For each: does `parse` load it, does the orientation line say the right thing, and does one policy
answer a question its owner already knows the answer to? That last one is the test — a gate is only
worth trusting where it agrees with somebody who knows.

## What a stranger will read

- [x] `CLAUDE.md` is gone from the history, not merely untracked: one pass of
      `git filter-repo --invert-paths --path CLAUDE.md` before the tag, because it costs one command
      while the repository has one author and cannot be had cleanly afterwards. It held working
      conventions for the owner and the assistant, no secret and nothing about another project, so
      this was a preference about what the history reveals rather than a necessity.

      Two commits went with it - they touched that file and nothing else - and the resulting tree is
      byte-identical to the one before the rewrite. The `Co-Authored-By: Claude` trailers stay:
      they were never a reason either way.
- [x] The branches. `spike/go-port` is deleted; `main` is the product and the only branch.
- [x] History and pull requests carry no secrets and nothing about unrelated projects. Verified
      before publishing: no such reference exists in any file, commit message, or branch.
- [x] `examples/` is the front door for most readers. It is organised by outcome and every example
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
