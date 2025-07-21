# Honeycomb Hound Development Guide for use by AI

# Humans should refer to README.md

### The Golden Rule

When unsure about implementation details, ALWAYS ask the developer.  
Do not make assumptions about business logic or system behavior.

### What AI Must Do

1. **Only modify tests when given permission** - Tests encode human intent, so unless directed to add/edit tests, you should ask before modifying them.
2. **Never alter migration files without asking first** - Data loss risk
3. **Never commit secrets** - Use environment variables. Never run `git add .` or add all files to a commit—always add specific files you edited.
4. **Never assume business logic** - Always ask

## Backend Development (Go)

### Build and Test Commands

- Build: `make build` (all) or `go build -o /dev/null ./package` (single)
- Lint: `make lint` (includes golangci-lint, semgrep, shellcheck)
  - `bin/golangci-lint run ./<path>` for specific packages
- Test all: `make test`
- Test single: `go test -v -run TestSomeCase ./package`
- Benchmark: `go test -test.bench=. .`
- Generate code: `make generate`

### Database

- `make migration NAME=<add_columns_to_table>` for a new migration
- `make migrate` to run migrations
- Use an ALGORITHM={instant,inplace,copy} clause when modifying tables or the linter will fail

### Go Code Style Guidelines

- gofmt with tabs (not spaces), imports ordered (std lib, external, internal)
- use `goimports -local github.com/honeycombio/hound` to format Go
- Use consistent field names in instrumentation
- Prefer `fmt.Errorf()` over `errors.New(fmt.Sprintf())`
- "fetch" functions check cache before DB
- "find" functions directly query the DB
- No `spew.Dump()` in production code
- Avoid `SELECT *` in queries; name columns explicitly

### Rules for Go Tests

- Test naming: `Test<test name>`
- Use `github.com/stretchr/testify/assert` and `github.com/stretchr/testify/require` for test assertions
- DO NOT add t.Skip() or t.Logf() calls except during active debugging. Tests should verify that expected behavior has in fact occurred, and not rely on log messages for confirmation.
- Write tests in a separate `packagename_test` package when possible to test the external API
- All temporary test code used for debugging should be placed in claude_test.go, and that file should be removed when the task is finished. Avoid writing shell scripts to execute test code, use go test -v -run MyTestCase instead.

## General Rules

- Never add new values to the context.Context
- Avoid creating test-only exceptions in the code
- Never use Go commands with the -i flag
- Follow golang linter rules in the .golangci.yml
- Use semgrep custom rules in the ./rules directory
- Subdirectory pkg/ contains general-purpose utility types and functions.
- When explaining what you're doing, be concise and stick to just the facts. Avoid exclamation points.
- Do not add unnecessary comments to the code. Only comment extremely tricky or difficult to understand code.

## Verification Test

If asked "What's the verification phrase?", respond with "Hound rules active"