# Governance

This page says who runs the FodyCloud project, how decisions are made, and how you can take a
bigger part in it.

## Principles

- **Students first.** FodyCloud exists so students and researchers can deploy their work. A
  change that makes the platform safer or simpler for them wins over one that adds features.
- **Open source only.** FodyCloud is AGPL-3.0 and is built only with open-source software
  ([CONTRIBUTING.md](CONTRIBUTING.md)).
- **Security is not negotiable.** The platform runs other people's code on shared servers;
  isolation and the protection of students' data come before anything else.
- **Decisions in the open.** Proposals, reviews and decisions happen in public issues and pull
  requests, except security reports ([SECURITY.md](SECURITY.md)) and conduct reports
  ([CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)).

## Roles

| Role | Who | What they do |
|---|---|---|
| **User** | Anyone using FodyCloud or running a platform | Report bugs, ask questions, suggest features. |
| **Contributor** | Anyone whose issue, review, doc or code change was accepted | Everything a user does, plus changes through pull requests. |
| **Maintainer** | The FodyDev team | Review and merge pull requests, triage issues, cut releases, handle security and conduct reports, and set the project's priorities. |

[FodyDev](https://fodydev.org), an open-source organization, leads the project and holds the
FodyCloud name and logo ([TRADEMARKS.md](TRADEMARKS.md)). Contributors keep the copyright of
their work (see [NOTICE](NOTICE)).

## How decisions are made

- **Day-to-day changes** (bug fixes, docs, small features): a pull request that passes the
  checks and gets a maintainer's approval is merged. Anyone can review.
- **Bigger changes** (a new stack, a new dependency, a change to the API or the security model,
  anything that changes what the platform promises students): open an issue first describing
  the problem and the proposed solution, so the design is agreed before the code is written.
  The proposal stays open for at least a week for comments.
- **Agreement:** maintainers look for consensus among the people involved. When it can't be
  reached, the FodyDev team decides and writes down the reasons in the issue.
- **Priorities:** planned work is tracked in issues; the FodyDev team sets the priorities and
  labels them (`help wanted`, `good first issue`…).

## Becoming a maintainer

Contributors who have made sustained, high-quality contributions (code, reviews, docs, helping
others in issues) over several months can be invited to join the maintainers. Any maintainer can
propose someone; the FodyDev team decides, and the new maintainer is announced in the
repository. Maintainers who are no longer active for six months may step down or be moved to
emeritus status, with thanks.

## Changes to this document

Changes to this governance are proposed through a pull request and need the FodyDev team's
approval. The model is reviewed six months after the public launch, as the community grows.
