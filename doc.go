// Copyright (c) the go-ruby-roda/roda authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package roda is a pure-Go (no cgo) reimplementation of the routing-tree
// engine at the heart of Ruby's [Roda] web toolkit, matching the semantics of
// the MRI `roda` gem (Roda 3.x, tracking the 4.0.5 line) without any Ruby
// runtime.
//
// Roda's defining feature is its routing tree: a request is dispatched by a
// single route block that peels path segments off the front of the request as
// it descends, using matcher methods — [RodaRequest.On], [RodaRequest.Is], the
// verb matchers [RodaRequest.Get]/[RodaRequest.Post]/… , [RodaRequest.Root] —
// each of which consumes the segments it matches and yields any captured
// segments to the block that handles that branch. This package models that
// segment-consuming engine exactly.
//
// The route block itself is Ruby (in a rbgo binding) or Go (standalone): it is
// supplied through the injectable [Handler] seam. The engine drives the tree
// and the block re-enters it, so the same [RodaRequest] flows top-down through
// nested [Handler] calls. Matching against the request and consuming the path
// is fully deterministic and lives here as pure Go; running the block is the
// host's job.
//
// The HTTP server — the socket accept loop, TLS — is out of scope: like Rack,
// the host owns it. This package builds on [github.com/go-ruby-rack/rack] for
// the Rack environment and header machinery, and produces the Rack
// `[status, headers, body]` tuple through [RodaResponse.Finish].
//
// [Roda]: https://roda.jeremyevans.net
package roda
