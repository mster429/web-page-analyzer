package analysis

import "context"

type LinkChecker interface {
	Check(ctx context.Context, links []string) LinkReport
}

type LinkReport struct {
	Inaccessible []string
	TimedOut     []string
}
