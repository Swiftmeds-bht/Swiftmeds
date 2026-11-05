## What and why
<!-- One or two sentences: what this PR changes and why. -->

## Jira
SCRUM-

## How to test
<!-- Commands or steps a reviewer can run, e.g. `make test`, a curl call, a page to open. -->

## Checklist
- [ ] Title follows `type(scope): summary (SCRUM-n)`, e.g. `feat(inventory): reserve stock (SCRUM-12)`
- [ ] Targets `develop` (only sprint-end release PRs target `main`)
- [ ] Tests added or updated; `make test` passes
- [ ] `make lint` passes
- [ ] Core service (Inventory, Holds, Search, Ranking, Gateway, Freshness, Substitution): coverage is still at least 80%
- [ ] No `.proto` changes, or the architecture lead has approved them
- [ ] README or docs updated if setup or behaviour changed
- [ ] No secrets committed (`.env` stays local)
