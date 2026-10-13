package analysis

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type Document struct {
	HTMLVersion string
	Title       string
}

const (
	versionUnknown = "Unknown"
	versionHTML5   = "HTML5"
)

// legacyVersions maps the public identifier of a DOCTYPE, in upper case, to the version it declares.
var legacyVersions = map[string]string{
	"-//IETF//DTD HTML 2.0//EN":              "HTML 2.0",
	"-//W3C//DTD HTML 3.2 FINAL//EN":         "HTML 3.2",
	"-//W3C//DTD HTML 4.0//EN":               "HTML 4.0 Strict",
	"-//W3C//DTD HTML 4.0 TRANSITIONAL//EN":  "HTML 4.0 Transitional",
	"-//W3C//DTD HTML 4.0 FRAMESET//EN":      "HTML 4.0 Frameset",
	"-//W3C//DTD HTML 4.01//EN":              "HTML 4.01 Strict",
	"-//W3C//DTD HTML 4.01 TRANSITIONAL//EN": "HTML 4.01 Transitional",
	"-//W3C//DTD HTML 4.01 FRAMESET//EN":     "HTML 4.01 Frameset",
	"-//W3C//DTD XHTML 1.0 STRICT//EN":       "XHTML 1.0 Strict",
	"-//W3C//DTD XHTML 1.0 TRANSITIONAL//EN": "XHTML 1.0 Transitional",
	"-//W3C//DTD XHTML 1.0 FRAMESET//EN":     "XHTML 1.0 Frameset",
	"-//W3C//DTD XHTML 1.1//EN":              "XHTML 1.1",
	"-//W3C//DTD XHTML BASIC 1.1//EN":        "XHTML Basic 1.1",
}

// ParseHTML reads the version, title, headings, login form and links from body.
func ParseHTML(body []byte, pageURL *url.URL) (Document, error) {
	// relative links are identified using pageURL
	if pageURL == nil {
		return Document{}, errors.New("pageURL is required")
	}

	root, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return Document{}, fmt.Errorf("failed to parse HTML: %w", err)
	}

	doc := Document{HTMLVersion: versionUnknown}
	for node := range root.Descendants() {
		if node.Type == html.DoctypeNode {
			doc.HTMLVersion = htmlVersion(node)
		}

		if node.Type != html.ElementNode {
			continue
		}

		switch node.DataAtom {
		case atom.Title:
			// the first non-empty title
			if doc.Title == "" {
				doc.Title = text(node)
			}
		}

	}

	return doc, nil
}

func htmlVersion(doctype *html.Node) string {
	if doctype.Data != "html" {
		return versionUnknown
	}
	publicID := ""
	for _, attr := range doctype.Attr {
		if attr.Key == "public" {
			publicID = attr.Val
		}
	}
	// <!DOCTYPE html> has no public identifier
	if publicID == "" {
		return versionHTML5
	}

	if version, ok := legacyVersions[strings.ToUpper(publicID)]; ok {
		return version
	}

	return versionUnknown
}

// text returns the text of node with whitespaces reduced to single spaces.
func text(node *html.Node) string {
	if node.FirstChild == nil || node.FirstChild.Type != html.TextNode {
		return ""
	}
	//all whitespaces reduced to single spaces
	return strings.Join(strings.Fields(node.FirstChild.Data), " ")
}
