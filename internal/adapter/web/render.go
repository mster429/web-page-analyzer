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

func render(w http.ResponseWriter, status int, data pageData) {
	var buf bytes.Buffer
	if err := page.Execute(&buf, data); err != nil {
		http.Error(w, "the page could not be rendered", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}
