---
name: tdd
description: Test-driven development procedure (red, green, refactor) for this repo. Use when writing tests before implementation, when a bug fix needs a regression test first, or when the tester agent is told to follow TDD.
---

# TDD

1. **Red.** Write one test for one behavior, named for the behavior (`returns 403 without order.ship`). Run it and read the failure: it must fail because the behavior is missing. A test that passes first, or fails on a typo or compile error, proves nothing.
2. **Green.** Write the least production code that passes. No extra features, no refactor yet.
3. **Refactor.** With tests green, clean up and re-run. Never change a test and the code under it in the same step.
4. Repeat per behavior. Cover the failure paths too: unauthorized, invalid input, not found, conflict, upstream down.

Rules: test behavior through public interfaces, not private details; no network or real clocks in unit tests; use the repo's existing helpers and in-memory fakes; never skip, delete or loosen a test to get green. For a bug, the first step is a test that reproduces it.

Where tests live: Go `*_test.go` beside the package (run `go test ./...` from the service module); Postgres tests skip without `POSTGRES_URL`.
