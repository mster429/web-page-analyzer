package analysis

// Result is output returned after page analysis completed
type Result struct {
	URL          string // after redirects
	HTMLVersion  string
	Title        string
	Headings     []HeadingCount
	Links        LinkCounts
	HasLoginForm bool
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
	Inaccessible int
	TimedOut     int
}
