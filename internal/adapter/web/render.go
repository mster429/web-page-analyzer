package web

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"

	"github.com/mster429/web-page-analyzer/internal/analysis"
)

//go:embed templates/page.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

var page = template.Must(template.ParseFS(templateFS, "templates/page.html"))

type pageData struct {
	URL    string
	Result *analysis.Result
	Error  *pageError
}

type pageError struct {
	Status      string // e.g. "404 Not Found"
	Description string
}

// linksData is passed to the "links" template, the link section of the page.
type linksData struct {
	Internal     int
	External     int
	Inaccessible []string
	TimedOut     []string
	NotChecked   int
	Checked      bool   // false while the link check is still running
	CheckID      string // set while pending; the script asks for the result with it
	Note         string // shown when the links could not be checked
}

// Links builds the link section for the result page
func (d pageData) Links() linksData {
	return linksData{
		Internal: d.Result.Links.Internal,
		External: d.Result.Links.External,
		Checked:  d.Result.LinkCheckID == "",
		CheckID:  d.Result.LinkCheckID,
	}
}

func render(w http.ResponseWriter, status int, data pageData) {
	write(w, status, "page.html", data)
}

func renderLinks(w http.ResponseWriter, status int, data linksData) {
	write(w, status, "links", data)
}

func write(w http.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := page.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, "the page could not be rendered", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}
