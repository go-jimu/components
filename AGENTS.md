# Repository Guidance for Code Agents

A Go component library. Verify every change with `make test` (race detector
on) and `make lint`, as CI does. `make lint` needs golangci-lint v2, which
`make tools` installs.

## Using a package

Before writing or changing code that uses a package:

1. Find it in the package map in `README.md`.
2. Read `go doc -all <package>`. The package documentation holds its rules,
   contracts, and error identities, and its rules are the bar for done.
3. Read its `example_test.go`, which sets the usage shape and runs.
4. Read its README when it has one: `config`, `encoding`, `fsm`, and
   `taskqueue` walk through longer code there, and `taskqueue/README.md`
   holds the provider adapter checklist.

## Changing the library

- Keep exported API backward-compatible when practical. A breaking change
  uses a `!` commit type, such as `refactor(fsm)!:`.
- Record every user-visible change in `CHANGELOG.md` under the next patch
  tag, such as `## v0.12.11`, in entries headed Added, Changed, Fixed,
  Deprecated, Removed, or Docs.

## Writing docs

Doc comments and READMEs are written for agents: write and revise them with
the `writing-for-agents` skill. In this repository that means:

- `doc.go` holds each contract once, one `# Section` per concept, with the
  choice of path and numbered composition steps up front.
- A symbol comment states its own contract (inputs, defaults, errors) and
  points to the `doc.go` section by name, as in `See "Events".`
- A README walks through longer code and links to `go doc` for the rules.
- One term per concept, instructions stated positively, and every pointer
  names a section that exists.
