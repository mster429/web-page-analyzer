package app

import "net/http"

// userAgent names the application in every request it sends. Many sites refuse Go's default "Go-http-client/1.1"
const userAgent = "WebPageAnalyzer/1.0 (+https://github.com/mster429/web-page-analyzer)"

// userAgentTransport sets the User-Agent header and passes the request on to next.
type userAgentTransport struct {
	next http.RoundTripper
}

func (t userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("User-Agent", userAgent)
	return t.next.RoundTrip(req)
}
