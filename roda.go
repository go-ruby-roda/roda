// Copyright (c) the go-ruby-roda/roda authors
//
// SPDX-License-Identifier: BSD-3-Clause

package roda

import (
	"regexp"
	"sync"

	"github.com/go-ruby-rack/rack"
)

// RouteBlock is the top-level route block of a Roda application — the single
// block passed to `route do |r| … end`. It receives the [RodaRequest] and
// drives the routing tree by calling matcher methods on it. It is the same
// shape as a [Handler] and, like one, is a seam the host supplies (the Ruby
// route block in a rbgo binding).
type RouteBlock func(r *RodaRequest) (handled bool, body any)

// Roda is a Roda application: a route block plus the machinery to dispatch a
// Rack request through it. Construct one with [New] and serve requests with
// [Roda.Call].
type Roda struct {
	route RouteBlock

	mu       sync.RWMutex
	anchored map[*regexp.Regexp]*regexp.Regexp
}

// New returns a Roda application that dispatches every request through route,
// mirroring `Roda.route { |r| … }`.
func New(route RouteBlock) *Roda {
	return &Roda{
		route:    route,
		anchored: make(map[*regexp.Regexp]*regexp.Regexp),
	}
}

// Call dispatches a Rack environment through the routing tree and returns the
// Rack `[status, headers, body]` triplet, mirroring `Roda#call`. It builds the
// request and response, runs the route block, catches the terminating :halt,
// and finishes the response — defaulting to a 404 when nothing matched.
func (app *Roda) Call(env rack.Env) (status int, headers *rack.Headers, body []string) {
	response := NewResponse()
	req := newRequest(app, env, response)
	app.dispatch(req, response)
	return response.Finish()
}

// dispatch runs the top-level route block, catching the halt thrown when a
// branch terminates the request. When the block returns without any branch
// matching, its own return value is treated as the response body (Roda's
// top-level throw_response), and the response defaults apply in
// [RodaResponse.Finish].
func (app *Roda) dispatch(req *RodaRequest, response *RodaResponse) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(haltSignal); ok {
				return
			}
			panic(r)
		}
	}()
	handled, bodyVal := app.route(req)
	if handled {
		response.writeBody(bodyVal)
	}
}

// anchoredMatcher returns a cached copy of re anchored to the start of a path
// segment, mirroring how Roda wraps a route regexp as `\A/(?:source)`.
func (app *Roda) anchoredMatcher(re *regexp.Regexp) *regexp.Regexp {
	app.mu.RLock()
	cached, ok := app.anchored[re]
	app.mu.RUnlock()
	if ok {
		return cached
	}
	compiled := regexp.MustCompile(`\A/(?:` + re.String() + `)`)
	app.mu.Lock()
	app.anchored[re] = compiled
	app.mu.Unlock()
	return compiled
}
