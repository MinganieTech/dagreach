# Security policy

## Reporting a vulnerability

Write to **contact@minganietechnologies.com**, with `dagreach` in the subject.

Please do not open a public issue, a discussion or a pull request for a vulnerability that has no
fix yet. A public report is a working exploit handed to everyone running the tool before anyone
running it can do something about it.

Useful in a report, as far as you have it: what version you are on (`dagreach --version`), what the
input looked like, what happened, and what you expected instead. A file that reproduces it is worth
more than a description of one — and if the file is sensitive, say so and send the shape rather than
the content.

**Acknowledgement within 5 business days.** That is a receipt, not a fix: it says a human has read
it and what happens next. If you have not heard back in that time, assume the mail went astray and
send it again.

## Supported versions

| Version | Supported |
|---|---|
| the most recent release | yes |
| anything older | no |

dagreach is before 1.0 and maintained by one author, so there is no long-term support branch and no
backporting. A fix ships in the next release, and the answer to "which version should I run" is the
latest one.

## What is in scope

dagreach reads a file somebody else produced and writes a report other systems act on, so the
interesting failures are the ones that cross that boundary:

- an input that makes the tool execute, write or read something it was never asked to
- a report that can be forged by the graph it describes — a node identifier that changes the shape
  of a pull-request comment, or that escapes the HTML page
- a policy that can be made to pass by the file it is judging
- a crash on untrusted input, if it can be turned into something worse than an exit code

Out of scope: the exit code being 1 when you wanted 0, a graph you disagree with, and anything that
requires already being able to run commands on the machine dagreach runs on.

## What this project does not promise

No bug bounty, no CVE assignment on request, and no coordinated disclosure timeline negotiated in
advance. What it promises is a reply, an honest answer about whether it is a real problem, and a fix
in a release if it is.
