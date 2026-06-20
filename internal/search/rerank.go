package search

// RerankByRecency reorders hits so files the order function ranks higher
// come first. Hits from the same file keep their input (line) order.
// order receives the unique hit paths (in stable input order) and must
// return some subset reordered most-recent-first; any path it omits
// trails the result in input order. A nil order returns hits unchanged.
func RerankByRecency(order func(paths []string) []string, hits []Hit) []Hit {
	if order == nil || len(hits) == 0 {
		return hits
	}

	// Unique paths in stable input order.
	seen := map[string]bool{}
	var uniquePaths []string
	for _, h := range hits {
		if !seen[h.Path] {
			seen[h.Path] = true
			uniquePaths = append(uniquePaths, h.Path)
		}
	}

	byPath := map[string][]Hit{}
	for _, h := range hits {
		byPath[h.Path] = append(byPath[h.Path], h)
	}

	out := make([]Hit, 0, len(hits))
	emitted := map[string]bool{}
	for _, p := range order(uniquePaths) {
		out = append(out, byPath[p]...)
		emitted[p] = true
	}
	// Paths the order function dropped trail in input order.
	for _, p := range uniquePaths {
		if !emitted[p] {
			out = append(out, byPath[p]...)
		}
	}
	return out
}
