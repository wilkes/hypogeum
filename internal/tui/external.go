package tui

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
)

// externalOpener launches a URL in the platform's default handler.
// Returning an error surfaces in the status bar; the caller doesn't
// distinguish between launch failures and the URL being rejected.
type externalOpener func(rawURL string) error

// openExternalURL is the default externalOpener. It validates the URL
// scheme (only http and https) and execs the platform-appropriate
// command. It does NOT wait for the spawned process — exec.Cmd.Start()
// returns once the OS has accepted the launch, and the browser keeps
// running independently. The user is back in hypogeum within a frame.
//
// Schemes other than http/https are deliberately rejected. Most
// desktop environments will happily hand `javascript:` or `data:` URLs
// to the default browser, which executes them; markdown content the
// user is viewing should not be able to trigger that.
func openExternalURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme %q not supported (http/https only)", u.Scheme)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", rawURL)
	default:
		// Linux, BSDs, anything else with xdg-utils.
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}
