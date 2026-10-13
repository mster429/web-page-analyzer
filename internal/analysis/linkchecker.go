package analysis

import "context"

// LinkChecker finds out which links cannot be reached.
type LinkChecker interface {
	Check(ctx context.Context, links []string) LinkReport
}

// LinkReport is what a link check generates.
type LinkReport struct {
	Checked      int // links that got an answer one way or the other
	Inaccessible []string
	TimedOut     []string
}
