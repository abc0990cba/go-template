package httpapp

// listOrEmpty keeps nil slices off the wire: a nil slice marshals to JSON
// null, and the SPA's list code expects an array everywhere.
func listOrEmpty[T any](slice []T) []T {
	if slice == nil {
		return []T{}
	}
	return slice
}
