---
name: ship-feature
description: Run the six-stage agent pipeline (plan, failing tests, implement, independent review, verify, remember) for a feature or bug fix, delegating each stage to its own subagent. Use when asked to build a feature end to end, "run the pipeline", or "ship" a scoped change.
---

# Ship a feature through the pipeline

You are the **orchestrator**. Subagents cannot spawn subagents and start with no memory, so **you** call each stage with the Agent tool and they pass work through files in `.claude/pipeline/<slug>/` (git-ignored). Pick a short kebab-case `<slug>` and create the directory. Tell every subagent that path and the request text; never paste your own conclusions into the reviewer's prompt.

## Stages

1. **Plan.** Agent `planner`. Read `plan.md`. If it has **Open questions**, or the scope is larger than the request, ask the user before continuing. Otherwise show the user the requirements and test list and get a go-ahead (this is the scope gate).
2. **Tests (red).** Agent `tester`. Read `red.md`. Confirm yourself: run the listed command and see the new tests fail for the stated reason. Snapshot the test files: `git status --short` and a hash/diff of the files under `## Test files`.
3. **Implement (green).** Agent `implementer`. Afterwards check the test files are unchanged from the snapshot; if any changed, reject the run and send the implementer back. Allow at most 3 implement attempts, then stop and report to the user.
4. **Review.** Agent `reviewer` (the repo's read-only reviewer), given only the request, `plan.md` and the instruction to review the working-tree diff. Never include `impl.md` or the implementer's reasoning. Critical and high findings go back to `implementer` (with a test per real defect), then re-run `reviewer` once. Remaining findings are reported, not hidden.

   **Review passed** means no open critical or high finding after at most one re-review. When it passes, **commit** (see Commit below). If it does not pass, do not commit: report the open findings to the user.
5. **Verify.** Agent `verifier`. Read `verify.md`. Anything other than PASS goes back to step 3 (counts toward the attempt limit) or to the user if it is a requirement problem.
6. **Remember.** See below. Then report to the user: what was built, the commit hash, the verdict with evidence, review findings left open, any instruction change with its reason, what was not run.

## Remember and improve

Collect lessons from this run: surprises in the code, a convention a reviewer caught, a command that was wrong, a pattern worth reusing. Append **at most five** short, verified bullets to `.claude/agent-notes.md` under a dated heading, each naming the file or command it came from. Do not record secrets, personal data, one-off task details or anything the repo docs already say (link instead). The planner reads `agent-notes.md` next time, so keep it short and prune stale lines.

## Changing instructions needs a reason

The instruction files are `CLAUDE.md`, `AGENTS.md`, `.claude/agents/*`, `.claude/skills/*` and `.claude/agent-notes.md`. Only the orchestrator may change them, only in the Remember stage, and **every change must carry a written reason**: what happened in this run (with the evidence: a finding, a failed command, a surprise), why the current text caused or allowed it, and why the new wording fixes it. Put the reason in the final report and in the commit message. Make instruction changes in a **separate commit** from the feature so the user can revert them alone. `tester` and `implementer` must never edit these files; if they think one is wrong, they say so and the orchestrator decides. No reason, no change.

## Commit

When review passes, commit once, on the current branch (if it is the default branch, `main` or `master`, create a branch first; never commit there directly):

- Stage only the files in the plan's **Affected files**, the tests from `red.md` and any docs the plan lists. Run `git status` first and leave out anything else. `.claude/pipeline/` is git-ignored; never force-add it, and never add `.env` or secrets.
- Message: imperative subject under 72 chars, a short body saying what and why, the requirement IDs met, and the attribution trailers your session instructions give.
- Do not push and do not open a PR unless the user asks. If the verifier later fails, fix with a new commit and say so; never amend or rewrite history.

## Rules

- Commit exactly when review passes (see Commit). Never commit before that; push only if the user asks.
- Small tasks (one-line fix, doc edit) do not need the pipeline; say so and just do them.
- Report honestly: failed or skipped stages are stated, never smoothed over.
