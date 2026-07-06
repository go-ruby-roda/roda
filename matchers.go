// Copyright (c) the go-ruby-roda/roda authors
//
// SPDX-License-Identifier: BSD-3-Clause

package roda

// This file defines the matcher value types accepted by the routing methods
// ([RodaRequest.On], [RodaRequest.Is], the verb matchers, …). A matcher is any
// Go value; the engine dispatches on its dynamic type in [RodaRequest.match].
// The mapping mirrors the matcher classes the roda gem understands:
//
//	Go value            Ruby matcher            Behaviour
//	------------------  ----------------------  --------------------------------
//	string              "segment"               match one path segment literally
//	Sym("name")         :name (Symbol)          match any non-empty segment,
//	                                             capture it as a string
//	StringMatcher       String (class)          like a Symbol capture
//	IntegerMatcher      Integer (class)          match a numeric segment, capture
//	                                             it as an int
//	true / false        true / false            always / never match (no consume)
//	Hash{...}           {:param=>…,:extension=>…} keyed matchers
//	[]any{...}          [a, b, …] (Array)        alternation: first element that
//	                                             matches wins
//	*regexp.Regexp      /re/ (Regexp)           match & capture groups
//
// Anything else never matches (the engine's match returns false), mirroring how
// an unrecognised matcher fails a Roda branch rather than crashing the server.

// Sym is a Symbol matcher: it matches any single non-empty path segment and
// captures that segment (as a string) into the captures passed to the
// [Handler]. It is the workhorse of Roda routing (`r.on :id do |id| … end`).
type Sym string

// StringMatcher is the Roda `String` class matcher: like a [Sym], it matches
// and captures any single non-empty segment.
type StringMatcher struct{}

// IntegerMatcher is the Roda `Integer` class matcher: it matches a single
// segment made entirely of ASCII digits and captures it as an int.
type IntegerMatcher struct{}

// Hash is a keyed matcher (the Roda Hash matcher). The supported keys are:
//
//   - "method": a string, or []any of strings — matches when the request method
//     equals any of them (case-insensitive). Consumes nothing.
//   - "param": a string key — matches when that request parameter is present and
//     non-empty, capturing its value. Consumes nothing.
//   - "extension": a string extension (e.g. "json") — matches when the next
//     segment ends in "."+ext, capturing the segment's base name and consuming
//     through the extension.
//
// All present keys must match (like Ruby's `all?`), and they are evaluated in a
// fixed order (method, param, extension) for deterministic path consumption.
type Hash map[string]any

// term is the internal terminal marker appended by [RodaRequest.Is] and the
// argument form of the verb matchers. It matches only when the whole path has
// been consumed.
type term struct{}

// Sentinel matcher values, provided so callers can write the class matchers
// without allocating.
var (
	// String is the Roda `String` class matcher (see [StringMatcher]).
	String = StringMatcher{}
	// Integer is the Roda `Integer` class matcher (see [IntegerMatcher]).
	Integer = IntegerMatcher{}
)
