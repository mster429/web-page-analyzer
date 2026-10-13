// Package analysis is the core of the application: it turns a web page into a Result.
package analysis

// Result is output returned after page analysis completed
type Result struct {
	URL          string // after redirects
	HTMLVersion  string
	Title        string
	Headings     []HeadingCount
	Links        LinkCounts
	HasLoginForm bool
	LinkCheckID  string // the page asks for the link check with this, empty when there are no links
}

// HeadingCount is the number of headings of one level, 1 to 6.
type HeadingCount struct {
	Level int
	Count int
}

// LinkCounts are known from parsing alone, before any link is requested.
type LinkCounts struct {
	Internal int
	External int
}

// LinkCheck is the link section after every unique link was requested.
type LinkCheck struct {
	Internal     int
	External     int
	Inaccessible []string // unique addresses, sorted
	TimedOut     []string // unique addresses, sorted
	NotChecked   int      // links left when the time for the check ran out
}
