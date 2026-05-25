package markdown

// Resolver looks up a wikilink target by name and returns the
// absolute path of the file it resolves to.
//
// `internal/vault.Vault` implements this. The interface is defined
// here so this package can be tested with a fake — it does not
// import vault.
type Resolver interface {
	Resolve(fromFile, name, heading, block string) (path string, ok bool)
	ResolveAnchor(path, heading, block string) (line int, ok bool)
}

// nopResolver returns ("", false) for every lookup. Used when no
// resolver is configured — wikilinks then render as broken.
type nopResolver struct{}

func (nopResolver) Resolve(string, string, string, string) (string, bool) {
	return "", false
}

func (nopResolver) ResolveAnchor(string, string, string) (int, bool) {
	return 0, false
}
