// Copyright (c) the go-ruby-roda/roda authors
//
// SPDX-License-Identifier: BSD-3-Clause

package roda

import (
	"strconv"

	"github.com/go-ruby-rack/rack"
)

// defaultContentType is the Content-Type Roda's RodaResponse assigns when the
// route block sets none (RodaResponse#default_headers).
const defaultContentType = "text/html"

// RodaResponse models Roda's `RodaResponse` — the mutable response the route
// block writes into. It buffers a status, an ordered header map (reusing
// [rack.Headers]) and a list of body chunks, and assembles the Rack
// `[status, headers, body]` triplet in [RodaResponse.Finish].
//
// It mirrors Roda's defaulting rules: a response whose body is empty defaults
// to status 404, one with a body defaults to 200, and the Content-Type defaults
// to text/html — so an app whose routing tree matches nothing yields a 404
// without the block doing anything.
type RodaResponse struct {
	status  int
	headers *rack.Headers
	body    []string
	length  int
}

// NewResponse returns an empty RodaResponse with no status set.
func NewResponse() *RodaResponse {
	return &RodaResponse{headers: rack.NewHeaders()}
}

// Status returns the currently set status (0 if the block set none; [RodaResponse.Finish] applies the default).
func (r *RodaResponse) Status() int { return r.status }

// SetStatus sets the response status (RodaResponse#status=).
func (r *RodaResponse) SetStatus(status int) { r.status = status }

// Headers returns the underlying header map (RodaResponse#headers).
func (r *RodaResponse) Headers() *rack.Headers { return r.headers }

// SetHeader sets a header value (RodaResponse#[]=).
func (r *RodaResponse) SetHeader(key string, val any) { r.headers.Set(key, val) }

// GetHeader returns a header value (RodaResponse#[]).
func (r *RodaResponse) GetHeader(key string) any { return r.headers.Get(key) }

// Body returns the buffered body chunks (RodaResponse#body).
func (r *RodaResponse) Body() []string { return r.body }

// Empty reports whether nothing has been written to the body
// (RodaResponse#empty?).
func (r *RodaResponse) Empty() bool { return len(r.body) == 0 }

// Write appends a chunk to the body and tracks its length, matching
// RodaResponse#write.
func (r *RodaResponse) Write(chunk string) {
	r.body = append(r.body, chunk)
	r.length += len(chunk)
}

// writeBody coerces a route block's return value into the body, mirroring how
// Roda's `throw_response` handles the block result: a string is written, a
// []string is written chunk by chunk, and nil (or any other type) writes
// nothing.
func (r *RodaResponse) writeBody(body any) {
	switch b := body.(type) {
	case string:
		r.Write(b)
	case []string:
		for _, c := range b {
			r.Write(c)
		}
	}
}

// Redirect sets the status and Location header for a redirect, matching
// RodaResponse#redirect. A zero status defaults to 302 (Found).
func (r *RodaResponse) Redirect(target string, status int) {
	if status == 0 {
		status = 302
	}
	r.status = status
	r.headers.Set("location", target)
}

// noEntityBody reports whether the status forbids an entity body (1xx, 204,
// 304), matching the set RodaResponse#finish strips Content-Type/Length for.
func noEntityBody(status int) bool {
	return status == 304 || status == 204 || (status >= 100 && status < 200)
}

// Finish assembles the Rack `[status, headers, body]` triplet, applying Roda's
// defaults (RodaResponse#finish): an empty body defaults to 404 and a present
// body to 200; Content-Type defaults to text/html; Content-Length is set from
// the buffered length except for statuses that forbid an entity body, whose
// Content-Type/Length headers are stripped.
func (r *RodaResponse) Finish() (status int, headers *rack.Headers, body []string) {
	s := r.status
	if r.Empty() {
		if s == 0 {
			s = 404
		}
	} else if s == 0 {
		s = 200
	}
	r.status = s

	if noEntityBody(s) {
		r.headers.Delete(rack.ContentTypeKey)
		r.headers.Delete(rack.ContentLengthKey)
		return s, r.headers, []string{}
	}

	if !r.headers.Has(rack.ContentTypeKey) {
		r.headers.Set(rack.ContentTypeKey, defaultContentType)
	}
	if !r.headers.Has(rack.ContentLengthKey) {
		r.headers.Set(rack.ContentLengthKey, strconv.Itoa(r.length))
	}
	if r.body == nil {
		r.body = []string{}
	}
	return s, r.headers, r.body
}
