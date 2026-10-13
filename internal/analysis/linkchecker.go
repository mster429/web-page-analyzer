package analysis

import "context"

type LinkChecker interface {
	Check(ctx context.Context, links []string) LinkReport
}

type LinkReport struct {
	Checked      int // links that got an answer one way or the other
	Inaccessible []string
	TimedOut     []string
}
