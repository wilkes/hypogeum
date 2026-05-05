package tree

import "os"

// osReadDir wraps os.ReadDir so we can mock it in tests if needed.
func osReadDir(name string) ([]os.DirEntry, error) {
	return os.ReadDir(name)
}
