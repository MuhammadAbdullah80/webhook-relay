# Contributing

## Getting set up

```sh
go build ./...
go test -race ./...
```

Go 1.22 or newer. There are no dependencies beyond the standard library, and
adding one should be a deliberate decision rather than a convenience.

## Before opening a pull request

- `go vet ./...` is clean
- `go test -race ./...` passes — the relay is concurrent by design, so a change
  that passes without `-race` has not really been tested
- New behaviour has a test alongside it

## Design constraints

Two properties are the reason this exists, and a change that breaks either one
needs a good argument:

1. **One slow target cannot affect the others.** Each target owns an
   independent queue and its own workers. Anything that introduces a shared
   lock on the delivery path defeats the purpose.
2. **The inbound handler never blocks.** `/hook` answers `202` once the payload
   is queued. A saturated queue sheds rather than waits.

## Commit messages

Explain why the change is needed, not just what it does. The diff already says
what it does.
