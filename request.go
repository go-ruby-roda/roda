// Copyright (c) the go-ruby-roda/roda authors
//
// SPDX-License-Identifier: BSD-3-Clause

package roda

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/go-ruby-rack/rack"
)

// Handler is the injectable seam for a Roda route block. When a matcher method
// matches, the engine calls the Handler for that branch with the captures
// accumulated so far. The Handler may re-enter the tree (call more matcher
// methods on r) — that is how nested routing works — and returns whether it
// produced a response body and, if so, the body value.
//
//   - handled == true: the returned body (a string, a []string, or nil) becomes
//     the response body.
//   - handled == false: no body is written by this block.
//
// Either way, a matched branch terminates the request (Roda always throws
// :halt after a matched `on`), unless the Handler itself re-entered and an inner
// branch terminated first. In a rbgo binding, the Handler runs the Ruby block.
type Handler func(r *RodaRequest, captures []any) (handled bool, body any)

// haltSignal is thrown (via panic) to unwind the routing tree when a branch
// terminates the request; [Roda.Call] recovers it.
type haltSignal struct {
	response *RodaResponse
}

// RodaRequest models Roda's `RodaRequest`: the request as seen by the routing
// tree. It carries the still-unconsumed remaining path and the captures peeled
// off it so far, and exposes the matcher methods that consume the path.
type RodaRequest struct {
	roda          *Roda
	env           rack.Env
	rackReq       *rack.Request
	response      *RodaResponse
	remainingPath string
	captures      []any
}

// newRequest builds a RodaRequest from a Rack env, seeding the remaining path
// from PATH_INFO (RodaRequest#initialize / #remaining_path).
func newRequest(roda *Roda, env rack.Env, response *RodaResponse) *RodaRequest {
	rr := rack.NewRequest(env)
	return &RodaRequest{
		roda:          roda,
		env:           env,
		rackReq:       rr,
		response:      response,
		remainingPath: rr.PathInfo(),
	}
}

// Env returns the underlying Rack environment.
func (r *RodaRequest) Env() rack.Env { return r.env }

// Response returns the response being built for this request.
func (r *RodaRequest) Response() *RodaResponse { return r.response }

// RequestMethod returns the HTTP method (REQUEST_METHOD).
func (r *RodaRequest) RequestMethod() string { return r.rackReq.RequestMethod() }

// RemainingPath returns the still-unconsumed portion of the path
// (RodaRequest#remaining_path).
func (r *RodaRequest) RemainingPath() string { return r.remainingPath }

// Captures returns the segments captured so far (RodaRequest#captures).
func (r *RodaRequest) Captures() []any { return r.captures }

// On matches the given matchers against the request. If they all match, it
// consumes the matched segments, yields the captures to the Handler and
// terminates the request. If they do not all match, the path and captures are
// restored and On returns so the next branch can be tried. Mirrors
// RodaRequest#on.
func (r *RodaRequest) On(handler Handler, matchers ...any) {
	r.ifMatch(handler, matchers)
}

// Is is like [RodaRequest.On] but only matches when the matchers consume the
// entire remaining path (a terminal match). Mirrors RodaRequest#is.
func (r *RodaRequest) Is(handler Handler, matchers ...any) {
	r.ifMatch(handler, appendTerm(matchers))
}

// Get matches a GET request. With no matchers it matches only when the whole
// path is already consumed; with matchers it matches them terminally (like
// [RodaRequest.Is]). Mirrors RodaRequest#get.
func (r *RodaRequest) Get(handler Handler, matchers ...any) {
	r.verb(rack.MethodGet, handler, matchers)
}

// Post matches a POST request; see [RodaRequest.Get] for the matcher semantics.
func (r *RodaRequest) Post(handler Handler, matchers ...any) {
	r.verb(rack.MethodPost, handler, matchers)
}

// Put matches a PUT request; see [RodaRequest.Get].
func (r *RodaRequest) Put(handler Handler, matchers ...any) {
	r.verb(rack.MethodPut, handler, matchers)
}

// Delete matches a DELETE request; see [RodaRequest.Get].
func (r *RodaRequest) Delete(handler Handler, matchers ...any) {
	r.verb(rack.MethodDelete, handler, matchers)
}

// Root matches a GET request whose remaining path is exactly "/", without
// consuming it. Mirrors RodaRequest#root.
func (r *RodaRequest) Root(handler Handler) {
	if r.remainingPath == "/" && r.RequestMethod() == rack.MethodGet {
		r.dispatch(handler)
	}
}

// Redirect sets a redirect response (default status 302) and terminates the
// request. Mirrors RodaRequest#redirect.
func (r *RodaRequest) Redirect(target string, status ...int) {
	st := 0
	if len(status) > 0 {
		st = status[0]
	}
	r.response.Redirect(target, st)
	r.halt()
}

// Halt terminates the request immediately with the current response, mirroring
// RodaRequest#halt with no arguments.
func (r *RodaRequest) Halt() { r.halt() }

// halt unwinds the routing tree back to [Roda.Call].
func (r *RodaRequest) halt() { panic(haltSignal{r.response}) }

// verb implements the shared verb-matcher logic (RodaRequest#_verb).
func (r *RodaRequest) verb(method string, handler Handler, matchers []any) {
	if r.RequestMethod() != method {
		return
	}
	if len(matchers) == 0 {
		if r.remainingPath == "" {
			r.dispatch(handler)
		}
		return
	}
	r.ifMatch(handler, appendTerm(matchers))
}

// ifMatch tries to match all matchers; on success it dispatches the Handler
// (which terminates the request), on failure it restores the path and captures.
// Mirrors RodaRequest#if_match.
func (r *RodaRequest) ifMatch(handler Handler, matchers []any) {
	savedPath := r.remainingPath
	savedCap := len(r.captures)
	if r.matchAll(matchers) {
		r.dispatch(handler)
	}
	r.remainingPath = savedPath
	r.captures = r.captures[:savedCap]
}

// matchAll reports whether every matcher matches, consuming as it goes
// (RodaRequest#match_all).
func (r *RodaRequest) matchAll(matchers []any) bool {
	for _, m := range matchers {
		if !r.match(m) {
			return false
		}
	}
	return true
}

// dispatch runs the Handler for a matched branch and terminates the request,
// mirroring how Roda always throws :halt with the block's result after a match.
func (r *RodaRequest) dispatch(handler Handler) {
	var body any
	handled := false
	if handler != nil {
		handled, body = handler(r, r.captures)
	}
	if handled {
		r.response.writeBody(body)
	}
	r.halt()
}

// match dispatches on the matcher's dynamic type (RodaRequest#match).
func (r *RodaRequest) match(matcher any) bool {
	switch m := matcher.(type) {
	case string:
		return r.matchString(m)
	case Sym:
		return r.matchSegment()
	case StringMatcher:
		return r.matchSegment()
	case IntegerMatcher:
		return r.matchInteger()
	case bool:
		return m
	case term:
		return r.remainingPath == ""
	case Hash:
		return r.matchHash(m)
	case []any:
		return r.matchArray(m)
	case *regexp.Regexp:
		return r.matchRegexp(m)
	default:
		return false
	}
}

// matchString matches one literal path segment (RodaRequest#_match_string).
func (r *RodaRequest) matchString(str string) bool {
	rp := r.remainingPath
	prefix := "/" + str
	if !strings.HasPrefix(rp, prefix) {
		return false
	}
	rest := rp[len(prefix):]
	switch {
	case rest == "":
		r.remainingPath = ""
		return true
	case rest[0] == '/':
		r.remainingPath = rest
		return true
	default:
		return false
	}
}

// matchSegment matches and captures one non-empty segment as a string, backing
// both the Symbol and String-class matchers (RodaRequest#_match_symbol).
func (r *RodaRequest) matchSegment() bool {
	rp := r.remainingPath
	if len(rp) == 0 || rp[0] != '/' {
		return false
	}
	rest := rp[1:]
	if idx := strings.IndexByte(rest, '/'); idx >= 0 {
		if idx == 0 {
			return false
		}
		r.captures = append(r.captures, rest[:idx])
		r.remainingPath = rest[idx:]
		return true
	}
	if rest == "" {
		return false
	}
	r.captures = append(r.captures, rest)
	r.remainingPath = ""
	return true
}

// matchInteger matches a numeric segment and captures it as an int
// (RodaRequest#_match_class for Integer).
func (r *RodaRequest) matchInteger() bool {
	rp := r.remainingPath
	if len(rp) == 0 || rp[0] != '/' {
		return false
	}
	rest := rp[1:]
	i := 0
	for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
		i++
	}
	if i == 0 {
		return false
	}
	if i < len(rest) && rest[i] != '/' {
		return false
	}
	n, _ := strconv.Atoi(rest[:i])
	r.captures = append(r.captures, n)
	r.remainingPath = rest[i:]
	return true
}

// matchRegexp matches the (auto-anchored) regexp at the front of the remaining
// path, capturing its groups and consuming the match (RodaRequest#_match_regexp).
func (r *RodaRequest) matchRegexp(re *regexp.Regexp) bool {
	anchored := r.roda.anchoredMatcher(re)
	rp := r.remainingPath
	loc := anchored.FindStringSubmatchIndex(rp)
	if loc == nil {
		return false
	}
	for i := 2; i < len(loc); i += 2 {
		if loc[i] < 0 {
			r.captures = append(r.captures, "")
		} else {
			r.captures = append(r.captures, rp[loc[i]:loc[i+1]])
		}
	}
	r.remainingPath = rp[loc[1]:]
	return true
}

// matchArray implements alternation: the first element that matches wins, and a
// matching plain-string element is captured (RodaRequest#_match_array).
func (r *RodaRequest) matchArray(arr []any) bool {
	for _, m := range arr {
		if r.match(m) {
			if s, ok := m.(string); ok {
				r.captures = append(r.captures, s)
			}
			return true
		}
	}
	return false
}

// matchHash matches the supported keyed matchers, in a fixed order so path
// consumption is deterministic (RodaRequest#_match_hash). Every present key
// must match.
func (r *RodaRequest) matchHash(h Hash) bool {
	if v, ok := h["method"]; ok && !r.matchMethod(v) {
		return false
	}
	if v, ok := h["param"]; ok && !r.matchParam(v) {
		return false
	}
	if v, ok := h["extension"]; ok && !r.matchExtension(v) {
		return false
	}
	return true
}

// matchMethod matches when the request method equals the value (a string) or
// any element of it (a []any of strings), case-insensitively. Consumes nothing.
func (r *RodaRequest) matchMethod(v any) bool {
	want := strings.ToUpper(r.RequestMethod())
	switch m := v.(type) {
	case string:
		return strings.ToUpper(m) == want
	case []any:
		for _, e := range m {
			if s, ok := e.(string); ok && strings.ToUpper(s) == want {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// matchParam matches when the named request parameter is present and non-empty,
// capturing its value. Consumes nothing (RodaRequest#match_param).
func (r *RodaRequest) matchParam(v any) bool {
	key, ok := v.(string)
	if !ok {
		return false
	}
	params, err := r.rackReq.GET()
	if err != nil {
		return false
	}
	val, ok := params.Get(key)
	if !ok {
		return false
	}
	s, ok := val.(string)
	if !ok || s == "" {
		return false
	}
	r.captures = append(r.captures, s)
	return true
}

// matchExtension matches when the next segment ends in "."+ext, capturing the
// segment's base name and consuming through the extension.
func (r *RodaRequest) matchExtension(v any) bool {
	ext, ok := v.(string)
	if !ok {
		return false
	}
	rp := r.remainingPath
	if len(rp) == 0 || rp[0] != '/' {
		return false
	}
	rest := rp[1:]
	end := strings.IndexByte(rest, '/')
	seg := rest
	tail := ""
	if end >= 0 {
		seg = rest[:end]
		tail = rest[end:]
	}
	suffix := "." + ext
	if !strings.HasSuffix(seg, suffix) || len(seg) == len(suffix) {
		return false
	}
	base := seg[:len(seg)-len(suffix)]
	r.captures = append(r.captures, base)
	r.remainingPath = tail
	return true
}

// appendTerm returns a copy of matchers with the terminal marker appended.
func appendTerm(matchers []any) []any {
	out := make([]any, 0, len(matchers)+1)
	out = append(out, matchers...)
	return append(out, term{})
}
