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

type HeadingCount struct {
	Level int
	Count int
}

type LinkCounts struct {
	Internal int
	External int
}

type LinkCheck struct {
	Internal     int
	External     int
	Inaccessible []string // unique addresses, sorted
	TimedOut     []string // unique addresses, sorted
	NotChecked   int      // links left when the time for the check ran out
}
