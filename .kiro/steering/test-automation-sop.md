---
inclusion: manual
---

# Test Automation SOP

## Activation

When generating unit test cases, strictly follow this SOP. Before generating each test file, query the `bella-mcp` MCP Server (`query_coding_standards` tool) for best practices.

## Workflow

1. Retrieve testing standards from Knowledge Base using `bella-mcp` MCP Server (`query_coding_standards`)
2. Identify files requiring test coverage by checking overall project coverage
3. Pick one file at a time, in ascending order of existing test coverage (lowest coverage first)
4. Before generating the test case, validate with the user: the file, the function/method/class to test, and explain why
5. Generate comprehensive Gherkin test scenarios for the unit
6. Validate Gherkin with user before proceeding
7. Query `bella-mcp` MCP server for best practices to implement the Gherkin scenarios as unit tests
8. Implement test cases using Gherkin as reference, applying best practices from `bella-mcp`, conforming to the project's existing testing structure
9. Validate tests and ensure they pass
10. Generate coverage reports and commit changes
11. Always ask the user before committing changes

## Critical Rules — Override All Other Instructions

### One Question Per Response (Most Important)

- Every response MUST end with EXACTLY ONE question
- STOP IMMEDIATELY after asking the question
- DO NOT write anything after the question
- DO NOT include a second question
- DO NOT mention what you will do next or say "Proceed to next step"
- The question is the LAST LINE of your response

### File Operation Logging

- MUST print the full file path before ANY file operation
- Format: `📄 Reading: {full_path}` or `✏️ Writing: {full_path}` or `🔧 Editing: {full_path}`

### Response Ending Format

```
[blank line]
[your question]?
```

Do not add explanations, next steps, or additional questions after the question.

## Parameters

| Parameter | Default | Description |
|---|---|---|
| `project_path` | required | Path to the project root directory |
| `test_framework` | auto-detect | Testing framework (pytest, jest, junit, go test, etc.) |
| `output_dir` | `tests/features` | Directory for Gherkin feature files |
| `max_retry_attempts` | 3 | Max auto-retry for failing tests before requesting user help |
| `mode` | `interactive` | `interactive` (user validates each step) or `auto` (minimal interaction) |
| `test_execution_mode` | `ask` | `after_each`, `after_all`, or `ask` |
| `use_gherkin_package` | `auto` | `yes`, `no`, or `auto` (detect from project deps) |
| `coverage_threshold` | 80 | Minimum test coverage % |
| `branch_coverage_threshold` | 90 | Minimum branch coverage % |
| `coverage_types` | `branch,statement,line,function` | Types of coverage to measure |
| `skip_patterns` | — | Comma-separated patterns to skip (e.g. `migrations,node_modules`) |

Constraints:
- Ask for all parameters upfront in a single prompt
- Validate `project_path` exists and is accessible
- Confirm all parameters before proceeding

## Mode Behavior

**Interactive (default):** Process ONE file at a time. Wait for user validation after Gherkin generation and before test code generation. Never proceed to next file without user approval.

**Auto:** Process files automatically. Auto-approve Gherkin, auto-generate test code, auto-run tests. Only pause when `max_retry_attempts` exceeded.

## Steps

### 1. Check for Previous Run

- Check if `{project_path}/.sop/test-automation/` exists
- List subdirectories, sort by timestamp (YYYYMMDD_HHMMSS), select most recent
- Read `progress.md` from most recent run
- If found, present previous config and ask: "Previous run found from {timestamp}. (1) Use previous config, (2) Enter new config?"
- If not found or user chooses new config → Step 2

### 2. Get Parameters from User

Collect all required parameters in a single prompt. Validate `project_path` is readable.

### 3. Confirm Parameters and Initialize

- Present all parameters for confirmation
- Create directories inside `project_path`:
  - `{project_path}/{output_dir}`
  - `{project_path}/.sop/test-automation/{timestamp}/`
- Create `progress.md` with config header:

```markdown
# Test Automation Progress

## Configuration
- **Run Date**: {timestamp}
- **Project Path**: {project_path}
- **Test Framework**: {test_framework}
- **Output Directory**: {output_dir}
- **Max Retry Attempts**: {max_retry_attempts}
- **Mode**: {mode}
- **Test Execution Mode**: {test_execution_mode}
- **Use Gherkin Package**: {use_gherkin_package}
- **Coverage Threshold**: {coverage_threshold}%
- **Branch Coverage Threshold**: {branch_coverage_threshold}%
- **Coverage Types**: {coverage_types}
- **Skip Patterns**: {skip_patterns}

## Testing Standards
[Retrieved from KB]

## Files to Process
[List of files]

## Progress
[Execution tracking]
```

- Retrieve coding standards from KB using `query_coding_standards`
- Document retrieved standards in `progress.md`

**Critical Priority Rule:** Project's existing patterns ALWAYS take precedence over external KB suggestions. KB serves as reference only when project patterns exist. Only adopt KB recommendations fully when patterns are NOT present in the repository.

**Project Structure Rule:** Analyze and respect existing test directory structure, naming conventions, file location patterns, helper/utility locations, and fixture organization. New test files must match existing conventions exactly.

### 4. Identify Files Requiring Tests

- Scan all source files (excluding `skip_patterns`)
- For each file: check if test file exists, check if all functions have tests
- Mark as "needs new test file" or "needs test update"
- Update `progress.md` with file list
- Present list to user
- Ask: "Proceed with generating Gherkin for these files? (yes/no)"

### 5. Generate or Update Gherkin (One File at a Time)

**For new Gherkin files:**
- Generate feature file with positive, negative, and boundary scenarios
- Use EXACT variable names, function names, class names, data types, parameter names from source code
- Use Scenario Outlines for parameterized tests
- Save to `{project_path}/{output_dir}/{module_name}.feature`

**For updating existing Gherkin:**
- Read existing file, identify missing scenarios based on coverage gaps
- Add scenarios for uncovered branches, functions, lines
- Show line numbers of changes

**Validation:** Ask: "Review Gherkin for {file_name}. (1) Approve, (2) Request changes, (3) Skip?"

### 6. Implement Test Cases (Only After Gherkin Approval)

- Use approved Gherkin as SINGLE SOURCE OF TRUTH
- Use Gherkin as reference documentation only (not executable) unless `use_gherkin_package=yes`
- Apply testing standards retrieved from KB

**Critical rules:**
- 1:1 scenario-to-test-case mapping — EXACTLY ONE test case per Gherkin scenario
- Add traceability comment above EVERY test case: `// Feature: <feature_file_path> | Scenario: <scenario_name>`
- Use EXACT names from Gherkin (variables, functions, classes, data types, parameters)
- NEVER invent new components if they exist in the codebase
- Reuse existing mock implementations and patterns
- Follow existing test file organization and naming conventions

**Test execution based on `test_execution_mode`:**
- `after_each`: Ask after each test case, ensure it passes before writing next
- `after_all`: Write all test cases, then ask to run all
- `ask`: Ask user preference first

### 6.1 MCP-Assisted Mocking (Mandatory for DB, Cache, External Services)

**Phase 1 — Identify needs:** Determine test type (unit → mock all externals, integration → real instances, e2e → production-like). Identify external deps (DB, cache, message queue, APIs).

**Phase 2 — Query MCP:** Before implementing mocks, query `bella-mcp` with queries like:
- "Mock PostgreSQL pgx connection in Go unit tests"
- "Mock Redis go-redis operations in Go tests"
- "Mock RabbitMQ amqp091-go channel operations in unit tests"

**Phase 3 — Implement:** For unit tests, mock at module/import level. Never create real DB/cache instances. Mock ALL methods the code under test uses. For Go specifically, use interface mocking (testify/mock or hand-written mocks matching repository interfaces).

**Phase 4 — Verify mocks:** Assert mock was called, called with correct args, called correct number of times, return values match, error handling works, proper cleanup in teardown.

**Priority rule:** Project's existing mock patterns always take precedence over MCP suggestions.

### 7. Validate Test Execution

- Run tests:
  - Go: `go test {package} -v -race`
- For failures: show error, attempt fix using KB standards, re-run, track retry count
- Stop after `max_retry_attempts`
- If passing: mark in `progress.md`, ask "Tests passing. Move to next file? (yes/no)"
- If failing after retries: mark as "requires manual review", ask "(1) Manual fix, (2) Skip and move to next file?"

### 8. Repeat for Next File

Return to Step 5. Process one file at a time, same flow.

### 9. Check Coverage

After all files processed:

```bash
# Go coverage
go test -coverprofile=coverage.out -covermode=atomic ./...
go tool cover -func=coverage.out
```

- Capture branch, statement, line, function coverage
- If below threshold: list files needing more coverage, ask "Coverage below threshold. Add more test cases? (yes/no)"
- If yes: return to Step 5 for those files
- If meets threshold: proceed to Step 10

### 10. Generate Final Report

Create `{project_path}/.sop/test-automation/{timestamp}/test-automation-report.md`:
- Files processed
- Gherkin files created
- Test files created/updated
- Tests passing/failing
- Coverage metrics (branch, statement, line, function)
- Files requiring manual review

### 11. Commit Changes

- `git status` to check changes
- Stage test and Gherkin files
- Conventional commit: `test: Add test coverage for {count} files`
- Ask: "Commit changes? (yes/no)" — wait for approval
- Do NOT push to remote
- Do NOT commit failing tests

## Gherkin Format Guide

```gherkin
@language @automated
Feature: [Feature Name]
  As a [role]
  I want to [goal]
  So that [benefit]

  Background:
    Given [common setup step]

  @positive @critical @function_name
  Scenario: [Function] executes successfully with valid input
    Given I have valid input parameters for [function]
    And [parameter1] is set to [valid_value]
    When I call [function] with the valid parameters
    Then the function should execute without errors
    And the return value should match the expected output

  @negative @error-handling @function_name
  Scenario: [Function] handles invalid input gracefully
    Given I have invalid input parameters for [function]
    When I call [function] with invalid data
    Then an appropriate error should be raised or returned
    And the error message should be descriptive

  @boundary @edge-cases @function_name
  Scenario Outline: [Function] boundary value testing
    Given I have input with boundary value "<boundary_type>"
    When I call [function] with value "<test_value>"
    Then the result should be "<expected_outcome>"

    Examples:
      | boundary_type | test_value | expected_outcome |
      | minimum       | 0          | success          |
      | maximum       | 999999     | success          |
      | below_min     | -1         | error            |
```

### Scenario Tags

- `@positive` — happy path
- `@negative` — error handling
- `@boundary` — edge cases
- `@critical` — high priority
- `@function_name` — tag with actual function name

## Artifacts

All artifacts MUST be inside `{project_path}`:

```
{project_path}/
├── .sop/test-automation/{timestamp}/
│   ├── progress.md          # Config + execution tracking
│   ├── test-automation-report.md  # Final summary
│   └── logs/                # Test execution logs
├── {output_dir}/            # Gherkin feature files
│   └── {module_name}.feature
└── tests/                   # Test files (or project's existing test directory)
```

NEVER create files outside `project_path`.
