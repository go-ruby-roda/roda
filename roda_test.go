// Copyright (c) the go-ruby-roda/roda authors
//
// SPDX-License-Identifier: BSD-3-Clause

package roda

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/go-ruby-rack/rack"
)

// mkenv builds a minimal Rack env for the given method, path and query string.
func mkenv(method, path, query string) rack.Env {
	return rack.Env{
		rack.RequestMethod: method,
		rack.PathInfo:      path,
		rack.QueryString:   query,
	}
}

// body returns a Handler that reports the given string as the response body.
func body(s string) Handler {
	return func(_ *RodaRequest, _ []any) (bool, any) { return true, s }
}

// capture returns a Handler that renders the captures via fmt-like join.
func capture(fn func(c []any) any) Handler {
	return func(_ *RodaRequest, c []any) (bool, any) { return true, fn(c) }
}

// run dispatches one request and returns status and joined body.
func run(t *testing.T, app *Roda, method, path, query string) (int, string) {
	t.Helper()
	status, _, b := app.Call(mkenv(method, path, query))
	s := ""
	for _, c := range b {
		s += c
	}
	return status, s
}

func TestOnStringNestedAndRoot(t *testing.T) {
	app := New(func(r *RodaRequest) (bool, any) {
		r.On(func(r *RodaRequest, _ []any) (bool, any) {
			r.Is(body("users"), "users")
			r.Get(body("api-root"))
			return false, nil
		}, "api")
		r.Root(body("home"))
		return false, nil
	})

	if st, b := run(t, app, "GET", "/api/users", ""); st != 200 || b != "users" {
		t.Fatalf("nested: got %d %q", st, b)
	}
	if st, b := run(t, app, "GET", "/api", ""); st != 200 || b != "api-root" {
		t.Fatalf("bare-get: got %d %q", st, b)
	}
	if st, b := run(t, app, "GET", "/", ""); st != 200 || b != "home" {
		t.Fatalf("root: got %d %q", st, b)
	}
	// Nothing matches -> Roda's 404 default.
	if st, b := run(t, app, "GET", "/nope", ""); st != 404 || b != "" {
		t.Fatalf("404: got %d %q", st, b)
	}
}

func TestSymbolStringIntegerCaptures(t *testing.T) {
	app := New(func(r *RodaRequest) (bool, any) {
		r.On(capture(func(c []any) any { return c[0].(string) }), "u", Sym("id"))
		r.On(capture(func(c []any) any { return c[0].(string) }), "s", String)
		r.On(capture(func(c []any) any {
			if c[0].(int) == 7 {
				return "seven"
			}
			return "other"
		}), "n", Integer)
		return false, nil
	})

	if _, b := run(t, app, "GET", "/u/42", ""); b != "42" {
		t.Fatalf("symbol: %q", b)
	}
	if _, b := run(t, app, "GET", "/s/abc", ""); b != "abc" {
		t.Fatalf("String class: %q", b)
	}
	if _, b := run(t, app, "GET", "/n/7", ""); b != "seven" {
		t.Fatalf("Integer: %q", b)
	}
	// Non-numeric segment fails the Integer matcher -> 404.
	if st, _ := run(t, app, "GET", "/n/x", ""); st != 404 {
		t.Fatalf("Integer non-numeric should 404, got %d", st)
	}
	// Trailing non-digit inside the segment fails the boundary check.
	if st, _ := run(t, app, "GET", "/n/12a", ""); st != 404 {
		t.Fatalf("Integer boundary should 404, got %d", st)
	}
	// Integer with a following segment: matches, consumes "/7".
	app2 := New(func(r *RodaRequest) (bool, any) {
		r.On(func(r *RodaRequest, c []any) (bool, any) {
			r.Is(body("ok"), "edit")
			return false, nil
		}, "n", Integer)
		return false, nil
	})
	if _, b := run(t, app2, "GET", "/n/7/edit", ""); b != "ok" {
		t.Fatalf("Integer with trailing: %q", b)
	}
}

func TestBoolMatchers(t *testing.T) {
	app := New(func(r *RodaRequest) (bool, any) {
		r.On(body("never"), false) // never matches
		r.On(body("always"), true) // always matches, consumes nothing
		return false, nil
	})
	if _, b := run(t, app, "GET", "/anything", ""); b != "always" {
		t.Fatalf("bool: %q", b)
	}
}

func TestIsTerminal(t *testing.T) {
	app := New(func(r *RodaRequest) (bool, any) {
		r.Is(body("exact"), "a")
		r.On(body("prefix"), "a")
		return false, nil
	})
	if _, b := run(t, app, "GET", "/a", ""); b != "exact" {
		t.Fatalf("is exact: %q", b)
	}
	// /a/b: is fails (term), on matches.
	if _, b := run(t, app, "GET", "/a/b", ""); b != "prefix" {
		t.Fatalf("is non-terminal falls through: %q", b)
	}
}

func TestStringNoMatch(t *testing.T) {
	app := New(func(r *RodaRequest) (bool, any) {
		r.On(body("hit"), "foo")
		return false, nil
	})
	// "/foobar" must not match the "foo" segment.
	if st, _ := run(t, app, "GET", "/foobar", ""); st != 404 {
		t.Fatalf("segment boundary: got %d", st)
	}
}

func TestSegmentEdgeCases(t *testing.T) {
	app := New(func(r *RodaRequest) (bool, any) {
		r.On(capture(func(c []any) any { return c[0].(string) }), Sym("a"), Sym("b"))
		return false, nil
	})
	// "/x//y": first Sym captures "x", second sees "//y" -> empty segment fails.
	if st, _ := run(t, app, "GET", "/x//y", ""); st != 404 {
		t.Fatalf("empty segment should fail, got %d", st)
	}
	// A lone "/" cannot satisfy a Symbol capture.
	app2 := New(func(r *RodaRequest) (bool, any) {
		r.On(body("hit"), Sym("id"))
		return false, nil
	})
	if st, _ := run(t, app2, "GET", "/", ""); st != 404 {
		t.Fatalf("root symbol should fail, got %d", st)
	}
}

func TestHashMethod(t *testing.T) {
	app := New(func(r *RodaRequest) (bool, any) {
		r.On(body("posted"), Hash{"method": "POST"})
		r.On(body("verbs"), Hash{"method": []any{"PUT", "PATCH"}})
		r.On(body("badtype"), Hash{"method": 5})
		return false, nil
	})
	if _, b := run(t, app, "POST", "/", ""); b != "posted" {
		t.Fatalf("method string: %q", b)
	}
	if _, b := run(t, app, "PUT", "/", ""); b != "verbs" {
		t.Fatalf("method array: %q", b)
	}
	// GET matches none of the above -> 404 (also exercises array no-match).
	if st, _ := run(t, app, "GET", "/", ""); st != 404 {
		t.Fatalf("method mismatch: got %d", st)
	}
}

func TestHashParam(t *testing.T) {
	app := New(func(r *RodaRequest) (bool, any) {
		r.On(capture(func(c []any) any { return c[0].(string) }), Hash{"param": "q"})
		r.On(body("noparam"), Hash{"param": 5}) // non-string key -> never
		return false, nil
	})
	if _, b := run(t, app, "GET", "/", "q=hello"); b != "hello" {
		t.Fatalf("param present: %q", b)
	}
	// Missing param -> falls through -> 404.
	if st, _ := run(t, app, "GET", "/", ""); st != 404 {
		t.Fatalf("param missing: got %d", st)
	}
	// Empty param value -> not matched.
	if st, _ := run(t, app, "GET", "/", "q="); st != 404 {
		t.Fatalf("param empty: got %d", st)
	}
	// Malformed query -> GET() errors -> param matcher fails.
	if st, _ := run(t, app, "GET", "/", "q=%zz"); st != 404 {
		t.Fatalf("param bad query: got %d", st)
	}
}

func TestHashExtension(t *testing.T) {
	app := New(func(r *RodaRequest) (bool, any) {
		r.On(capture(func(c []any) any { return c[0].(string) }), Hash{"extension": "json"})
		r.On(body("badtype"), Hash{"extension": 5})
		return false, nil
	})
	if _, b := run(t, app, "GET", "/post.json", ""); b != "post" {
		t.Fatalf("extension: %q", b)
	}
	// Extension with a following segment consumes just the first segment.
	app2 := New(func(r *RodaRequest) (bool, any) {
		r.On(func(r *RodaRequest, c []any) (bool, any) {
			r.Is(body(c[0].(string)+":x"), "x")
			return false, nil
		}, Hash{"extension": "json"})
		return false, nil
	})
	if _, b := run(t, app2, "GET", "/post.json/x", ""); b != "post:x" {
		t.Fatalf("extension trailing: %q", b)
	}
	// No extension -> no match.
	if st, _ := run(t, app, "GET", "/post", ""); st != 404 {
		t.Fatalf("no extension: got %d", st)
	}
	// Bare ".json" (empty base) -> no match.
	if st, _ := run(t, app, "GET", "/.json", ""); st != 404 {
		t.Fatalf("empty base extension: got %d", st)
	}
	// Root path -> no segment.
	if st, _ := run(t, app, "GET", "/", ""); st != 404 {
		t.Fatalf("root extension: got %d", st)
	}
}

func TestArrayAlternation(t *testing.T) {
	app := New(func(r *RodaRequest) (bool, any) {
		r.On(capture(func(c []any) any { return c[0] }), []any{"a", "b"})
		r.On(capture(func(c []any) any { return c[0].(string) }), []any{Sym("id")})
		return false, nil
	})
	// Plain-string element is captured.
	if _, b := run(t, app, "GET", "/a", ""); b != "a" {
		t.Fatalf("array string a: %q", b)
	}
	if _, b := run(t, app, "GET", "/b", ""); b != "b" {
		t.Fatalf("array string b: %q", b)
	}
	// Symbol element inside array captures the segment.
	if _, b := run(t, app, "GET", "/c/rest", ""); b != "c" {
		t.Fatalf("array symbol: %q", b)
	}
}

func TestRegexpMatcher(t *testing.T) {
	digits := regexp.MustCompile(`([0-9]+)`)
	app := New(func(r *RodaRequest) (bool, any) {
		r.On(capture(func(c []any) any { return c[0].(string) }), digits)
		return false, nil
	})
	if _, b := run(t, app, "GET", "/123", ""); b != "123" {
		t.Fatalf("regexp: %q", b)
	}
	// Second call exercises the anchored-matcher cache hit.
	if _, b := run(t, app, "GET", "/456/x", ""); b != "456" {
		t.Fatalf("regexp cached: %q", b)
	}
	// Non-matching input.
	if st, _ := run(t, app, "GET", "/abc", ""); st != 404 {
		t.Fatalf("regexp no match: got %d", st)
	}
	// Alternation with a non-participating group -> empty capture.
	alt := regexp.MustCompile(`(a)|(b)`)
	app2 := New(func(r *RodaRequest) (bool, any) {
		r.On(capture(func(c []any) any { return c[0].(string) + "|" + c[1].(string) }), alt)
		return false, nil
	})
	if _, b := run(t, app2, "GET", "/a", ""); b != "a|" {
		t.Fatalf("regexp optional group: %q", b)
	}
}

func TestVerbs(t *testing.T) {
	app := New(func(r *RodaRequest) (bool, any) {
		r.Get(body("g"), "res")
		r.Post(body("p"), "res")
		r.Put(body("u"), "res")
		r.Delete(body("d"), "res")
		return false, nil
	})
	for _, tc := range []struct{ m, want string }{
		{"GET", "g"}, {"POST", "p"}, {"PUT", "u"}, {"DELETE", "d"},
	} {
		if _, b := run(t, app, tc.m, "/res", ""); b != tc.want {
			t.Fatalf("%s: %q", tc.m, b)
		}
	}
	// Wrong method for a verb matcher -> 404.
	if st, _ := run(t, app, "GET", "/res", ""); st != 200 {
		t.Fatalf("get sanity: %d", st)
	}
	if st, _ := run(t, app, "PATCH", "/res", ""); st != 404 {
		t.Fatalf("unhandled verb: %d", st)
	}
}

func TestBareVerbNonEmptyPath(t *testing.T) {
	app := New(func(r *RodaRequest) (bool, any) {
		r.Get(body("bare")) // matches only when path fully consumed
		return false, nil
	})
	// Path not consumed -> bare get does not match.
	if st, _ := run(t, app, "GET", "/x", ""); st != 404 {
		t.Fatalf("bare verb non-empty: %d", st)
	}
}

func TestRootNonMatches(t *testing.T) {
	app := New(func(r *RodaRequest) (bool, any) {
		r.Root(body("home"))
		return false, nil
	})
	// Non-root path.
	if st, _ := run(t, app, "GET", "/x", ""); st != 404 {
		t.Fatalf("root wrong path: %d", st)
	}
	// Root path, wrong method.
	if st, _ := run(t, app, "POST", "/", ""); st != 404 {
		t.Fatalf("root wrong method: %d", st)
	}
}

func TestRedirectAndHalt(t *testing.T) {
	app := New(func(r *RodaRequest) (bool, any) {
		r.On(func(r *RodaRequest, _ []any) (bool, any) {
			r.Redirect("/login")
			return false, nil
		}, "go")
		r.On(func(r *RodaRequest, _ []any) (bool, any) {
			r.Redirect("/perm", 301)
			return false, nil
		}, "perm")
		r.On(func(r *RodaRequest, _ []any) (bool, any) {
			r.Response().SetStatus(418)
			r.Response().Write("teapot")
			r.Halt()
			return false, nil
		}, "teapot")
		return false, nil
	})
	st, h, _ := app.Call(mkenv("GET", "/go", ""))
	if st != 302 || h.Get("location") != "/login" {
		t.Fatalf("redirect: %d %v", st, h.Get("location"))
	}
	st, h, _ = app.Call(mkenv("GET", "/perm", ""))
	if st != 301 || h.Get("location") != "/perm" {
		t.Fatalf("redirect 301: %d %v", st, h.Get("location"))
	}
	if st, b := run(t, app, "GET", "/teapot", ""); st != 418 || b != "teapot" {
		t.Fatalf("halt: %d %q", st, b)
	}
}

func TestNilHandlerMatchHalts(t *testing.T) {
	app := New(func(r *RodaRequest) (bool, any) {
		r.On(nil, true) // matches, nil handler -> empty body -> 404 default
		return false, nil
	})
	if st, b := run(t, app, "GET", "/x", ""); st != 404 || b != "" {
		t.Fatalf("nil handler: %d %q", st, b)
	}
}

func TestTopLevelBlockBody(t *testing.T) {
	// Top-level block returns a body without any branch matching.
	app := New(func(r *RodaRequest) (bool, any) {
		return true, "toplevel"
	})
	if st, b := run(t, app, "GET", "/", ""); st != 200 || b != "toplevel" {
		t.Fatalf("toplevel body: %d %q", st, b)
	}
	// Top-level returns a []string body.
	app2 := New(func(r *RodaRequest) (bool, any) {
		return true, []string{"a", "b"}
	})
	if st, b := run(t, app2, "GET", "/", ""); st != 200 || b != "ab" {
		t.Fatalf("toplevel []string: %d %q", st, b)
	}
	// Top-level returns handled with an unsupported body type -> writes nothing.
	app3 := New(func(r *RodaRequest) (bool, any) {
		return true, 12345
	})
	if st, b := run(t, app3, "GET", "/", ""); st != 404 || b != "" {
		t.Fatalf("toplevel other-type: %d %q", st, b)
	}
}

func TestRequestAccessors(t *testing.T) {
	var seen struct {
		env    rack.Env
		method string
		remain string
		resp   *RodaResponse
		caps   []any
	}
	app := New(func(r *RodaRequest) (bool, any) {
		r.On(func(r *RodaRequest, c []any) (bool, any) {
			seen.env = r.Env()
			seen.method = r.RequestMethod()
			seen.remain = r.RemainingPath()
			seen.resp = r.Response()
			seen.caps = r.Captures()
			return true, "ok"
		}, "a", Sym("id"))
		return false, nil
	})
	run(t, app, "GET", "/a/99", "")
	if seen.method != "GET" || seen.remain != "" || seen.resp == nil {
		t.Fatalf("accessors: %+v", seen)
	}
	if len(seen.caps) != 1 || seen.caps[0] != "99" {
		t.Fatalf("captures: %v", seen.caps)
	}
	if seen.env[rack.PathInfo] != "/a/99" {
		t.Fatalf("env: %v", seen.env)
	}
}

func TestNonHaltPanicPropagates(t *testing.T) {
	app := New(func(r *RodaRequest) (bool, any) {
		panic("boom")
	})
	defer func() {
		rec := recover()
		if rec != "boom" {
			t.Fatalf("expected boom to propagate, got %v", rec)
		}
	}()
	app.Call(mkenv("GET", "/", ""))
}

func TestUnknownMatcher(t *testing.T) {
	// A matcher of an unsupported type never matches.
	app := New(func(r *RodaRequest) (bool, any) {
		r.On(body("hit"), 3.14) // float64 is not a supported matcher
		return false, nil
	})
	if st, _ := run(t, app, "GET", "/x", ""); st != 404 {
		t.Fatalf("unknown matcher should 404, got %d", st)
	}
}

func TestResponseUnit(t *testing.T) {
	resp := NewResponse()
	if !resp.Empty() || resp.Status() != 0 {
		t.Fatalf("fresh response")
	}
	resp.SetStatus(201)
	resp.SetHeader("x-test", "v")
	if resp.Status() != 201 || resp.GetHeader("x-test") != "v" {
		t.Fatalf("set/get")
	}
	resp.Write("hi")
	if resp.Empty() || len(resp.Body()) != 1 {
		t.Fatalf("write")
	}
	if resp.Headers() == nil {
		t.Fatalf("headers nil")
	}

	// writeBody with a []string and with nil / other types.
	r2 := NewResponse()
	r2.writeBody([]string{"a", "b"})
	if !reflect.DeepEqual(r2.Body(), []string{"a", "b"}) {
		t.Fatalf("writeBody []string: %v", r2.Body())
	}
	r2.writeBody(nil)   // no-op
	r2.writeBody(12345) // no-op
	if len(r2.Body()) != 2 {
		t.Fatalf("writeBody noop changed body: %v", r2.Body())
	}
}

func TestFinishDefaults(t *testing.T) {
	// Non-empty body defaults to 200 and sets content-type + content-length.
	r := NewResponse()
	r.Write("hello")
	st, h, b := r.Finish()
	if st != 200 || h.Get(rack.ContentTypeKey) != defaultContentType || h.Get(rack.ContentLengthKey) != "5" {
		t.Fatalf("finish 200: %d %v", st, h.ToMap())
	}
	if len(b) != 1 {
		t.Fatalf("finish body: %v", b)
	}

	// Empty body defaults to 404 and normalises a nil body to [].
	r = NewResponse()
	st, _, b = r.Finish()
	if st != 404 || b == nil || len(b) != 0 {
		t.Fatalf("finish 404: %d %v", st, b)
	}

	// noEntityBody status strips content-type/length.
	r = NewResponse()
	r.SetStatus(204)
	st, h, b = r.Finish()
	if st != 204 || h.Has(rack.ContentTypeKey) || h.Has(rack.ContentLengthKey) || len(b) != 0 {
		t.Fatalf("finish 204: %d %v", st, h.ToMap())
	}

	// Explicit status and pre-set headers are preserved (not overwritten).
	r = NewResponse()
	r.SetStatus(201)
	r.SetHeader(rack.ContentTypeKey, "application/json")
	r.SetHeader(rack.ContentLengthKey, "99")
	r.Write("{}")
	st, h, _ = r.Finish()
	if st != 201 || h.Get(rack.ContentTypeKey) != "application/json" || h.Get(rack.ContentLengthKey) != "99" {
		t.Fatalf("finish preserve: %d %v", st, h.ToMap())
	}
}

// reqWith builds a RodaRequest with a crafted remaining path, to exercise the
// segment-matcher guards that normal routing (paths are always "" or "/...")
// cannot reach.
func reqWith(remain string) *RodaRequest {
	r := newRequest(New(nil), mkenv("GET", "/", ""), NewResponse())
	r.remainingPath = remain
	return r
}

func TestMatcherPathGuards(t *testing.T) {
	for _, remain := range []string{"", "abc"} {
		if reqWith(remain).matchSegment() {
			t.Fatalf("matchSegment(%q) should not match", remain)
		}
		if reqWith(remain).matchInteger() {
			t.Fatalf("matchInteger(%q) should not match", remain)
		}
		if reqWith(remain).matchExtension("json") {
			t.Fatalf("matchExtension(%q) should not match", remain)
		}
	}
}

func TestNoEntityBody(t *testing.T) {
	for _, s := range []int{100, 199, 204, 304} {
		if !noEntityBody(s) {
			t.Fatalf("expected noEntityBody(%d)", s)
		}
	}
	for _, s := range []int{200, 302, 404} {
		if noEntityBody(s) {
			t.Fatalf("expected !noEntityBody(%d)", s)
		}
	}
}
