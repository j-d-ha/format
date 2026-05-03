# General

- Always add idiomatic godoc to your code.
- wrap all errors with this format:
  `[in <package>.<method>] <short message explaining what and why>: %w`

# Go Tests

- Prefer idiomatic table-driven tests for related cases.
- Define test cases as a `map[string]struct{ ... }`, where the key is the descriptive test name.
- Keep each case focused: inputs, expected outputs, expected errors, and any setup data belong in the struct.
- Fan out cases with `for name, tc := range tests { t.Run(name, func(t *testing.T) { ... }) }`.
- When using `t.Parallel()` inside subtests, capture loop variables first: `name, tc := name, tc`.
- Use clear failure messages that include the observed and expected values.
- Prefer `errors.Is` / `errors.As` for error assertions instead of string matching.
- For complex values, use `cmp.Diff`, `reflect.DeepEqual`, or equivalent project conventions rather than hand-written field comparisons.
- Keep test helpers small, mark them with `t.Helper()`, and avoid sharing mutable state between table cases.
