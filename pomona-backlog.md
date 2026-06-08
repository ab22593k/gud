# Pomona Backlog

## P1 — Do First

*No current P1 tasks.*

## P2 — Quick Wins

- [ ] **Handle unchecked error of os.Chdir in diff_test.go**
  - **File**: `internal/git/diff_test.go`
  - **Source**: golangci-lint rule violation (errcheck)
  - **Details**: Error return values of `os.Chdir` on lines 129, 176, 201 are not checked. Since these are in deferred statements in tests, they should be explicitly handled or ignored with `_ = os.Chdir(...)` or an helper function to avoid lint warnings.

- [ ] **Handle unchecked errors of os.MkdirAll and os.WriteFile in hook_test.go**
  - **File**: `internal/git/hook_test.go`
  - **Source**: golangci-lint rule violation (errcheck)
  - **Details**: Error return values of `os.MkdirAll` (lines 46, 65) and `os.WriteFile` (line 68) are not checked. They should be checked using `t.Fatal(err)` or `if err != nil` to make sure setup/teardown in tests is correct.

## P3 — Worth Doing, Plan the Split

*No current P3 tasks.*

## P4 — Parking Lot

*No current P4 tasks.*
