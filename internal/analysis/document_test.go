package analysis

import (
	"net/url"
	"slices"
	"testing"
)

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("bad test url %q: %v", raw, err)
	}
	return u
}

func parse(t *testing.T, html string) Document {
	t.Helper()
	doc, err := ParseHTML([]byte(html), mustURL(t, "https://www.example.com/blog/post"))
	if err != nil {
		t.Fatalf("ParseHTML returned an error: %v", err)
	}
	return doc
}

func TestParseHTML_NilPageURL(t *testing.T) {
	if _, err := ParseHTML([]byte("<html></html>"), nil); err == nil {
		t.Error("expected an error when pageURL is nil")
	}
}

func TestParseHTML_Version(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{"html5", `<!DOCTYPE html><html></html>`, "HTML5"},
		{"html5 upper case", `<!DOCTYPE HTML><html></html>`, "HTML5"},
		{"html 4.01 strict", `<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01//EN" "http://www.w3.org/TR/html4/strict.dtd">`, "HTML 4.01 Strict"},
		{"html 4.01 transitional", `<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01 Transitional//EN">`, "HTML 4.01 Transitional"},
		{"html 3.2", `<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 3.2 Final//EN">`, "HTML 3.2"},
		{"xhtml 1.0 strict", `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">`, "XHTML 1.0 Strict"},
		{"xhtml 1.1", `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.1//EN" "http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd">`, "XHTML 1.1"},
		{"public id in lower case", `<!DOCTYPE html PUBLIC "-//w3c//dtd html 4.01//en">`, "HTML 4.01 Strict"},
		{"unknown public id", `<!DOCTYPE html PUBLIC "-//ACME//DTD Something//EN">`, "Unknown"},
		{"not an html doctype", `<!DOCTYPE svg><html></html>`, "Unknown"},
		{"no doctype", `<html><body>hi</body></html>`, "Unknown"},
		{"empty page", ``, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parse(t, tt.html).HTMLVersion; got != tt.want {
				t.Errorf("HTMLVersion = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseHTML_Title(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{"simple", `<title>Hello</title>`, "Hello"},
		{"whitespace collapsed", "<title>\n  Hello \t  World\n</title>", "Hello World"},
		{"no title", `<html><body><p>hi</p></body></html>`, ""},
		{"empty title", `<title></title>`, ""},
		{"first one wins", `<title>First</title><svg><title>Second</title></svg>`, "First"},
		{"entities decoded", `<title>Tom &amp; Jerry</title>`, "Tom & Jerry"},
		{"upper case tag", `<TITLE>Hello</TITLE>`, "Hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parse(t, tt.html).Title; got != tt.want {
				t.Errorf("Title = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseHTML_Headings(t *testing.T) {
	tests := []struct {
		name string
		html string
		want [6]int
	}{
		{"none", `<p>text</p>`, [6]int{}},
		{"one of each", `<h1>a</h1><h2>b</h2><h3>c</h3><h4>d</h4><h5>e</h5><h6>f</h6>`, [6]int{1, 1, 1, 1, 1, 1}},
		{"repeated", `<h2>a</h2><h2>b</h2><h2>c</h2>`, [6]int{0, 3, 0, 0, 0, 0}},
		{"nested in other elements", `<div><section><h3>a</h3></section></div>`, [6]int{0, 0, 1, 0, 0, 0}},
		{"upper case", `<H1>a</H1>`, [6]int{1, 0, 0, 0, 0, 0}},
		{"empty heading still counts", `<h1></h1>`, [6]int{1, 0, 0, 0, 0, 0}},
		{"similar tags are not headings", `<header>a</header><hr><hgroup></hgroup>`, [6]int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headings := parse(t, tt.html).Headings
			if len(headings) != 6 {
				t.Fatalf("got %d heading levels, want 6", len(headings))
			}
			for i, h := range headings {
				if h.Level != i+1 || h.Count != tt.want[i] {
					t.Errorf("Headings[%d] = %+v, want level %d with count %d", i, h, i+1, tt.want[i])
				}
			}
		})
	}
}

func TestParseHTML_LoginForm(t *testing.T) {
	tests := []struct {
		name string
		html string
		want bool
	}{
		{"login form", `<form><input type="text" name="user"><input type="password"></form>`, true},
		{"no form", `<p>hello</p>`, false},
		{"form without a password", `<form><input type="search"></form>`, false},
		{"sign-up form with two passwords", `<form><input type="password"><input type="password"></form>`, false},
		{"password outside a form", `<input type="password">`, false},
		{"type in upper case", `<form><input type="PASSWORD"></form>`, true},
		{"password nested deeper", `<form><div><label><input type="password"></label></div></form>`, true},
		{"login form then search form", `<form><input type="password"></form><form><input type="search"></form>`, true},
		{"search form then login form", `<form><input type="search"></form><form><input type="password"></form>`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parse(t, tt.html).HasLoginForm; got != tt.want {
				t.Errorf("HasLoginForm = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseHTML_Links(t *testing.T) {
	// the page is https://www.example.com/blog/post
	tests := []struct {
		name string
		html string
		want []Link
	}{
		{"absolute internal", `<a href="https://www.example.com/about">x</a>`,
			[]Link{{URL: "https://www.example.com/about", IsInternal: true}}},
		{"absolute external", `<a href="https://other.org/page">x</a>`,
			[]Link{{URL: "https://other.org/page", IsInternal: false}}},
		{"root relative", `<a href="/about">x</a>`,
			[]Link{{URL: "https://www.example.com/about", IsInternal: true}}},
		{"path relative", `<a href="contact.html">x</a>`,
			[]Link{{URL: "https://www.example.com/blog/contact.html", IsInternal: true}}},
		{"scheme relative", `<a href="//cdn.example.net/lib">x</a>`,
			[]Link{{URL: "https://cdn.example.net/lib", IsInternal: false}}},
		{"www is ignored", `<a href="https://example.com/a">x</a>`,
			[]Link{{URL: "https://example.com/a", IsInternal: true}}},
		{"host case is ignored", `<a href="https://WWW.Example.COM/a">x</a>`,
			[]Link{{URL: "https://WWW.Example.COM/a", IsInternal: true}}},
		{"port and scheme are ignored", `<a href="http://example.com:8080/a">x</a>`,
			[]Link{{URL: "http://example.com:8080/a", IsInternal: true}}},
		{"other subdomain is external", `<a href="https://blog.example.com/">x</a>`,
			[]Link{{URL: "https://blog.example.com/", IsInternal: false}}},
		{"fragment is dropped", `<a href="/docs#install">x</a>`,
			[]Link{{URL: "https://www.example.com/docs", IsInternal: true}}},
		{"fragment only points at the page", `<a href="#top">x</a>`,
			[]Link{{URL: "https://www.example.com/blog/post", IsInternal: true}}},
		{"spaces around href", `<a href="  /about  ">x</a>`,
			[]Link{{URL: "https://www.example.com/about", IsInternal: true}}},
		{"repeated links are all kept", `<a href="/a">x</a><a href="/a">y</a>`,
			[]Link{{URL: "https://www.example.com/a", IsInternal: true}, {URL: "https://www.example.com/a", IsInternal: true}}},

		{"no href", `<a name="top">x</a>`, nil},
		{"empty href", `<a href="">x</a>`, nil},
		{"mailto", `<a href="mailto:me@example.com">x</a>`, nil},
		{"tel", `<a href="tel:+94112345678">x</a>`, nil},
		{"javascript", `<a href="javascript:void(0)">x</a>`, nil},
		{"ftp", `<a href="ftp://example.com/file">x</a>`, nil},
		{"invalid address", `<a href="http://[::1">x</a>`, nil},
		{"link element is not a link", `<link rel="stylesheet" href="/style.css">`, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parse(t, tt.html).Links; !slices.Equal(got, tt.want) {
				t.Errorf("Links = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseHTML_WholePage(t *testing.T) {
	doc := parse(t, `<!DOCTYPE html>
<html>
<head><title>My page</title></head>
<body>
  <h1>Welcome</h1>
  <h2>News</h2>
  <h2>Contact</h2>
  <form action="/login"><input name="user"><input type="password" name="pass"></form>
  <a href="/about">About</a>
  <a href="https://other.org">Other</a>
</body>
</html>`)

	if doc.HTMLVersion != "HTML5" {
		t.Errorf("HTMLVersion = %q, want HTML5", doc.HTMLVersion)
	}
	if doc.Title != "My page" {
		t.Errorf("Title = %q, want My page", doc.Title)
	}
	if doc.Headings[0].Count != 1 || doc.Headings[1].Count != 2 {
		t.Errorf("Headings = %+v, want one h1 and two h2", doc.Headings)
	}
	if !doc.HasLoginForm {
		t.Error("HasLoginForm = false, want true")
	}
	if len(doc.Links) != 2 {
		t.Errorf("got %d links, want 2", len(doc.Links))
	}
}
