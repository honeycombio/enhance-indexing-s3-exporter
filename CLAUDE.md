---
Source: .ruler/hound.md
---
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

---
Source: .ruler/poodle_frontend.md
---
# Frontend Development Guidelines (cmd/poodle/)

**These guidelines apply when working in the `cmd/poodle/` directory - the frontend TypeScript/React codebase.**

## Frontend Development (TypeScript/React)

### Build and Test Commands

- Build: `yarn build`
- Dev server: `yarn dev`
- Test: `yarn test` (Jest + React Testing Library)
- Lint: `yarn lint` (TypeScript/JavaScript)
- Lint styles: `yarn lint:styles` (CSS/SCSS)
- Type check: `yarn typecheck`
- Format: `yarn format`

### Frontend Architecture

- Frontend and backend code coexist in the `poodle` directory
- Feature code: `poodle/javascript/`
- Shared packages: `poodle/packages/`
- Design system: `poodle/lattice/` and `poodle/lattice-community/`
- Custom Webpack build setup
- TypeScript config: `poodle/tsconfig.json` (strict mode enabled)

### Design System (Lattice)

#### Component Discovery Priority

1. **Directory listings** - Look at folder structure to see what components exist
2. **Documentation** - Check for README files in component directories
3. **Search existing usage** - Look for import statements in the codebase

#### Component Import Hierarchy

- **Primary**: `@honeycombio/lattice/ComponentName` (from `poodle/lattice/`)
- **Community**: `@honeycombio/lattice-community/ComponentName` (from `poodle/lattice-community/`)
- **Packages**: `@honeycombio/componentname` (from `poodle/packages/`, lowercase)
- **Layout components**: `@honeycombio/layout` (for Flex, Grid, Stack, etc.)

**Temporary workaround**: If a component import fails with the standard pattern above, try the full path:

- `@honeycombio/lattice/ComponentName/ComponentName` (e.g., `@honeycombio/lattice/Badge/Badge`)
- This inconsistency will be resolved in the future

#### Primitives & Variants Architecture

- **Lattice Primitives**: Functionality-only components (unstyled, accept `className`)
  - Found in `poodle/lattice/primitives/`
  - Can be used by any company, no Honeycomb-specific styling
  - Based on [shadcn](https://ui.shadcn.com/) and [Radix](https://www.radix-ui.com/primitives/docs/overview/introduction) patterns
    - **shadcn**: A UI component library with beautifully-designed, accessible components
    - **Radix**: Low-level UI primitives focused on accessibility and customization
- **Lattice Variants**: Honeycomb-specific styled components
  - Built on top of primitives
  - Limited, opinionated prop APIs
  - Use `UNSAFE_className` as escape hatch for custom styling
  - Example: `<Button variant="primary" UNSAFE_className={customStyles} />`

#### Design Tokens

- **Token file**: `poodle/lattice/Tokens/dist/tokens.css`
- **Alternative token file**: `poodle/lattice/Tokens/dist/primitives/tokens.css` (if token not found in primary file)
- **Naming pattern**: `--lat-{category}-{subcategory}-{state/variant}`
- **Categories**:
  - Colors: `--lat-color-background-primary-default`, `--lat-color-foreground-critical-default`
  - Spacing: `--lat-sys-spacing-formvertical`, `--lat-sys-spacing-inlinetext`
  - Typography: `--lat-text-body-default`, `--lat-text-heading-1`
- Auto-generated by Style Dictionary (run `yarn build-tokens`)

### Styling Guidelines

- **Migration in progress**: Moving from Emotion to CSS modules
- **New code**: Use `.module.scss` files with CSS custom properties
- **Legacy code**: May use Emotion (avoid in new work)
- CSS & SCSS: Follow lint rules in `poodle/stylelint.config.js`
- Use design tokens from `poodle/lattice/Tokens/dist/tokens.css`
- TSX/JSX/HTML/CSS/SCSS: 4-space soft indents
- TypeScript/JS: 2-space soft indents
- **No utility classes available** (we don't have a utility class system)

#### CSS Modules Example

```scss
// MyComponent.module.scss
.container {
  display: flex;
  gap: var(--lat-sys-spacing-inlinetext);
  background-color: var(--lat-color-background-primary-default);
}
```

### State Management

- Use Context API, [Jotai](https://jotai.org/), or local component state
- **Jotai**: Atomic approach to React state management with automatic optimization
- Follow existing patterns in the area you're working in
- Don't create global state without explicit approval

### Data Fetching

- **Migration in progress**: Moving from homegrown `Requests` library to React Query
- Legacy: `poodle/packages/requests/index.ts`
- New: React Query wrapper at `poodle/packages/request-hooks`
  - Check `poodle/packages/request-hooks/index.tsx` for all available options
- Backend integration via REST APIs

### TypeScript Guidelines

- Use types from `poodle/packages/requests/payload_types.gen.ts` for API response types
- Prefer types over interfaces
- Don't bypass TypeScript with 'any' types. If TypeScript inference fails or types are complex, let the developer know and ask them what to do
- If you see an `@ts-expect-error` comment, ask if you can fix it

### Error Handling Patterns

- Use try-catch with fallback to default values
- ErrorBoundary components (located in `poodle/javascript/appLayout`)
- Wrap components with `<ErrorBoundary name="component-name">`
- Use Banner components (`poodle/packages/banner/index.tsx`) for user-facing error messages
- Do not ignore TypeScript errors. If you can't solve it, let the developer know. Do not default to `any` types

### Testing Guidelines

- Use Jest with React Testing Library
- Test naming: `describe()` and `it()` blocks
- Test files: `Component.test.tsx` (co-located with components)
- Focus on user interactions and behavior, not implementation details

### Naming Conventions

- Follow existing patterns in the area you're working in
- Generally: PascalCase for components, camelCase for functions/variables
- File extensions: `.ts`, `.tsx`, `.js`, `.jsx`

### Component Guidelines

- Keep complex logic separate from React components
- Use functional components with hooks
- Prefer composition over inheritance
- Use TypeScript for all new code
- **ALWAYS default to Lattice components first, regardless of existing patterns in the file**
  - Look for lattice components in `poodle/lattice` and lattice-community components in `poodle/lattice-community`.
  - For layout: use `@honeycombio/layout` (Flex, Grid, Stack, etc.)
  - For icons: use `@honeycombio/lattice/Icons`
  - Do NOT copy existing deprecated component patterns, even if the file uses them
  - Only use non-Lattice components when no Lattice equivalent exists
- **Be consistent throughout files - if using Lattice components, use them everywhere**
  - If adding Lattice components to a file with deprecated components, suggest updating the deprecated ones too
- **Always ask where to place new components** instead of guessing directory structure
  - Feature-specific: Usually `poodle/javascript/`
  - Reusable across teams: Usually `poodle/packages/`
  - When in doubt, ask for guidance
- **Ask before building a component in lattice or lattice-community**
  - Lattice components undergo a rigorous review process and should only be a small subset of components
  - Both lattice and lattice-community require approval before adding new components
  - **Before creating new components, follow these steps:**
  - 1. **Check if a Radix primitive exists first** - Review the [Radix Primitives documentation](https://www.radix-ui.com/primitives) to see if a suitable primitive already exists
  - 2. **Always try to use existing lattice and lattice-community components to build new lattice and lattice-community components**
  - 3. **Reference the ai-prompts folder** - Read all of the files in `poodle/lattice/ai-prompts/` for additional guidance, and follow the prompts to generate a new component.
  - 4. **Ask for approval** before proceeding with component creation

### Forms

- Use Lattice components along with Formik for form handling
- Combine Lattice UI components with Formik's form state management

## Poodle Verification Test

If asked "What's the poodle verification phrase?", respond with "Poodle rules active"
