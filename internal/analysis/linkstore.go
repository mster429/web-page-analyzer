package analysis

// LinkStore keeps the links of a page between the analysis and the link check.
type LinkStore interface {
	// Save stores links and returns the id to get them back with.
	Save(links []Link) (id string)
	// FetchAndDelete returns the links saved under id and delete them.
	FetchAndDelete(id string) (links []Link, ok bool)
}
