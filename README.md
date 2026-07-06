<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-roda/brand/main/social/go-ruby-roda-roda.png" alt="go-ruby-roda/roda" width="720"></p>

# roda — go-ruby-roda

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-roda.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of the routing-tree engine at the heart of
Ruby's [Roda](https://roda.jeremyevans.net)** — the segment-consuming request
router of MRI 4.0.5's `roda` gem (the Roda 3.x line). It models Roda's defining
idea — a single route block that *peels path segments off the front of the
request* as it descends, capturing them for the branch that handles them — and
produces the Rack `[status, headers, body]` tuple, **without any Ruby runtime**.

It is a routing backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), but is a
**standalone, reusable** module — a sibling of
[go-ruby-rack](https://github.com/go-ruby-rack/rack) (the Rack core, which it
builds on for the environment and header machinery),
[go-ruby-erb](https://github.com/go-ruby-erb/erb) and
[go-ruby-regexp](https://github.com/go-ruby-regexp/regexp).

> **What it is — and isn't.** Matching against the request and consuming the
> path is fully deterministic and lives here as pure Go. Running the route
> **block** is not: it is a Ruby block (in a rbgo binding) or a Go closure
> (standalone), supplied through the injectable [`Handler`](#the-route-block-seam)
> seam. The engine drives the routing tree and the block re-enters it. The HTTP
> server — the socket accept loop, TLS — is the **host's** job and is out of
> scope, exactly as it is for Rack.

## The routing tree

A Roda app is one route block. Inside it, matcher methods each try to match the
front of the *remaining path*; a matched `on` **consumes** the segments it
matched, yields any captures to its block, and terminates the request:

```go
app := roda.New(func(r *roda.RodaRequest) (bool, any) {
    r.Root(func(r *roda.RodaRequest, _ []any) (bool, any) {
        return true, "welcome"                       // GET /
    })
    r.On(func(r *roda.RodaRequest, _ []any) (bool, any) {
        // remaining path here is what's left after "/api"
        r.Is(func(r *roda.RodaRequest, c []any) (bool, any) {
            return true, "user " + c[0].(string)     // GET /api/users/42 -> "user 42"
        }, "users", roda.Sym("id"))
        r.Get(func(r *roda.RodaRequest, _ []any) (bool, any) {
            return true, "api root"                   // GET /api
        })
        return false, nil
    }, "api")
    return false, nil                                 // nothing matched -> 404
})

status, headers, body := app.Call(env)                // Rack triplet
```

## Matchers

`On`, `Is` and the verb matchers take any number of matcher values; the engine
dispatches on each value's dynamic type, exactly as the roda gem dispatches on
the matcher's class:

| Go value          | Ruby matcher            | Behaviour                                             |
| ----------------- | ----------------------- | ---------------------------------------------------- |
| `string`          | `"segment"`             | match one path segment literally                     |
| `roda.Sym("id")`  | `:id` (Symbol)          | match any non-empty segment, capture it as a string  |
| `roda.String`     | `String` (class)        | like a Symbol capture                                 |
| `roda.Integer`    | `Integer` (class)       | match a numeric segment, capture it as an `int`      |
| `true` / `false`  | `true` / `false`        | always / never match (consumes nothing)              |
| `roda.Hash{…}`    | `{:param=>…}` etc.       | keyed matchers: `"method"`, `"param"`, `"extension"` |
| `[]any{a, b, …}`  | `[a, b, …]` (Array)     | alternation: first element that matches wins         |
| `*regexp.Regexp`  | `/re/` (Regexp)         | match & capture groups (auto-anchored to `\A/`)      |

- **`On`** enters a branch (partial match); **`Is`** matches only when the path
  is fully consumed (terminal). The verb matchers **`Get`/`Post`/`Put`/`Delete`**
  add a method test: bare (`r.Get(h)`) they match when the path is fully
  consumed; with matchers they are terminal like `Is`. **`Root`** matches a `GET`
  to exactly `/`.
- **`Redirect(path[, status])`** sets a redirect (default 302) and terminates;
  **`Halt`** terminates immediately with the current response.

## The route-block seam

The route block is the one thing Roda cannot make deterministic, so it is an
explicit seam:

```go
type Handler func(r *RodaRequest, captures []any) (handled bool, body any)
```

When a matcher matches, the engine calls the branch's `Handler` with the
captures accumulated so far. The handler may re-enter the tree (call more
matcher methods on `r`) — that is how nested routing works — and returns whether
it produced a body and, if so, the body (a `string`, a `[]string`, or `nil`). A
matched branch always terminates the request, mirroring how Roda throws `:halt`
after a matched `on`. In a rbgo binding, the `Handler` runs the Ruby block.

`Roda.Call` builds the request and response, runs the route block, catches the
terminating halt, and finishes the response — defaulting to **404** when nothing
matched and **200** when a body was written, with `Content-Type: text/html` and
`Content-Length` filled in, exactly as `RodaResponse#finish` does.

## Install

```sh
go get github.com/go-ruby-roda/roda
```

## Fidelity vs the `roda` gem

Modelled on the Roda 3.x request router (tracking the MRI 4.0.5 line):

- **Segment consumption** is byte-for-byte the gem's: `_match_string` matches on
  segment boundaries (`/foo` does not match `/foobar`), `_match_symbol` /
  `String` capture `\A/([^/]+)`, `Integer` captures `\A/(\d+)` bounded by a
  segment edge, and regexps are auto-anchored to `\A/(?:…)` with their captures
  concatenated and the match consumed.
- **`on` / `is` / verb / `root` / `redirect` / `halt`** semantics match the gem,
  including the save/restore of the remaining path on a failed branch and the
  always-terminate-after-match rule.
- **`RodaResponse#finish` defaults** match: empty body → 404, present body →
  200, `text/html` default Content-Type, and Content-Type/Length stripped for
  1xx/204/304.
- **Scope.** This is the *core routing engine*. The gem's plugin system, the
  full `Hash`-matcher key set (only `:method`, `:param`, `:extension` are
  modelled here), and rendering are out of scope — they layer on top of this
  engine. The `:extension` matcher captures the segment base name and consumes
  through the extension.

## Tests & coverage

```sh
go test -race -cover ./...
```

The suite enforces **100% line coverage** — every matcher type, capture
passing, verb and root matching, halt/redirect, the response defaults, and the
unmatched → 404 path — and cross-compiles on all six supported 64-bit targets
(`amd64`, `arm64`, `riscv64`, `loong64`, `ppc64le`, `s390x`, including the
big-endian `s390x`). CI runs the suite on Linux, macOS and Windows and under
QEMU for the non-native arches.

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright (c) 2026, the go-ruby-roda/roda
authors.
