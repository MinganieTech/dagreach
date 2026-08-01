# The GitHub Action

The CLI is the product; the Action is how a team gets it without reading any documentation. It
runs one dagreach command, posts the report as a pull-request comment, and fails the job when a
policy fails.

```yaml
permissions:
  contents: read
  pull-requests: write   # only needed when the action comments

jobs:
  impact:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: terraform graph > infra.dot
      - uses: MinganieTech/dagreach@v0.1.0-rc.1
        with:
          command: impact
          file: infra.dot
          changed: aws_vpc.main
          fail-if-reaches: group=aws_db_instance
```

The comment looks like this — verdicts first, because that is what a reviewer reads, and the
evidence right under them:

> ### dagreach
>
> sbom.json: pkg:npm/qs@6.11.0 reaches 4 of 4 nodes (100%)
>
> | verdict | policy | detail |
> | --- | --- | --- |
> | **FAIL** | `fail-if-reaches group=root` | 1 node(s) matching group=root are reached |
>
> - `pkg:npm/checkout-service@2.4.0` &larr; `pkg:npm/qs@6.11.0 -> pkg:npm/body-parser@1.20.2 -> ...`
>
> <details><summary>Full report</summary> … </details>

The same markdown goes to the job summary, so the report is there even when commenting is off or
the event is not a pull request.

## Inputs

| Input | Meaning |
|---|---|
| `command` | `impact` (default), `diff`, `stats`, `parse` |
| `file` | the graph, for everything but `diff` |
| `before`, `after` | the two graphs, for `diff` |
| `changed` | changed node ids, comma separated |
| `profile` | `terraform`, `dbt`, `cyclonedx`, `generic`; detected from the file when empty |
| `edge-semantics` | `feeds` or `depends-on`; the profile decides when empty |
| `format` | `dot` or `jgf`; detected when empty |
| `fail-if-reaches` | selector such as `group=production`, one per line |
| `fail-on-new-reach` | selector judged against `before`, for `diff`, one per line |
| `max-impacted` | fail when more than N nodes are impacted |
| `fail-on` | `cycle` |
| `explain` | show why each node is reached (default `true`) |
| `limit` | items per list in the comment, `0` for everything |
| `comment` | post and update a pull-request comment (default `true`) |
| `comment-tag` | identifies the comment to update |
| `version` | the released tag to download (default `v0.1.0-rc.1`) |
| `token` | needs `pull-requests: write`, and downloads the release |

## Outputs

| Output | Meaning |
|---|---|
| `verdict` | `pass`, `fail`, or `unknown` |
| `commented` | `true` when the report was posted; `false` otherwise, including when commenting is off, the event is not a pull request, or the head branch lives elsewhere |
| `exit-code` | `0` ok, `1` a policy failed, `2` usage, `3` a policy could not be settled, `4` the input could not be read |
| `report` | path to the markdown report |

## Four behaviours worth knowing

**One comment per tag, updated in place.** A gate that stacks twenty comments on a busy pull
request is a gate people mute. Give each job its own `comment-tag` when you run several.

**An unusable run is not a clean gate.** Exit `2` (bad usage) and `4` (unreadable input) fail the
step loudly rather than passing quietly — the distinction the exit codes exist for. Only exit `1`
means a policy actually failed.

**A pull request whose head lives elsewhere gets no comment, and that is not a bug.** Those
workflows run with a read-only `GITHUB_TOKEN`, so posting would fail the job over a permission
nobody can grant from the workflow. The action compares the head repository against
`github.repository`, skips the comment, and says so with a `::notice::`; the report is still in the
job summary and the verdict still gates the build. The `commented` output is `false`.

The test is where the head branch lives, not `head.repo.fork`: a repository that is itself a fork
sets that flag on its own internal pull requests, where the token is writable and the comment works.

Switching to `pull_request_target` would provide a writable token — and would run the workflow
against the fork's code with the base repository's secrets. That is not a trade worth a comment.

**UNKNOWN comments, warns, and does not fail the build.** Exit `3` means the graph could not settle
a policy — usually a selector reading an attribute the producer does not emit. The action posts the
report, adds a `::warning::`, and leaves the decision to you: gate on the `verdict` output when your
project wants an unsettled policy to block.

```yaml
- uses: MinganieTech/dagreach@v0.1.0-rc.1
  id: impact
  with:
    fail-if-reaches: attr:risk=high
- if: steps.impact.outputs.verdict == 'unknown'
  run: exit 1
```

## Where the binary comes from

The action downloads the release named by `version` and checks it against the `checksums.txt` of
that same release, then runs it. It does not install Go and does not compile: the gate that judges
your pull request is the artifact that was published, not one built on the spot.

The checksum proves the download arrived intact. It does not prove the release is what its author
meant to publish, since both come from the same place - pinning `version` to a tag you have looked
at is what does that, and it is why the input has no floating default.

## What it does not do yet
- It maps changed *files* to changed *nodes* nowhere: you pass node ids. Deriving them from a diff
  is producer-specific, and doing it generically would be guessing.
