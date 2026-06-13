# The Big Render

*A development diary, told the way Raymond Chandler would have told it — if Chandler had ever shipped a terminal markdown browser instead of drinking his lunch. Every case below really happened. The dates are in the commit log. The rest is just weather.*

---

## I. The job comes in

It was the fifth of May, and the kind of empty repository that makes a man reach for a CLI. No files. No history. Just a cursor blinking at me like a one-eyed cat that knew something I didn't.

The job was simple, the way they always say it's simple right before it costs you a weekend. Point the thing at a folder full of markdown and make it readable. Fill the screen. Make the links go somewhere. I'd built worse for worse reasons.

So I laid down the scaffold and wired up the entrypoint, and the first thing it did was double-cross me. Auto-open went diving headfirst into the deepest subdirectory and landed on the last leaf in the alphabet, like a drunk looking for his keys under the one streetlamp that worked. I taught it manners. First top-level file or nothing. You don't go rummaging in a stranger's basement on the first date.

By nightfall the links worked. You could cycle them with `n` and `p`, follow one with `Enter`, back out with `Esc`. I gave the whole thing a coat of house style — three coats, actually, because the first two looked like a tax form and the second like a tax form that had been to art school. The third one looked like a page. I called it minimalism and went home.

The watcher I added last. A little `fsnotify` stoolie that tapped me on the shoulder whenever a file changed on disk. Best-effort, I told it. If you fall down on the job, the browser keeps running. I don't carry dead weight.

## II. The vault keeps its secrets

Two days later the wikilinks walked in. `[[Like this]]`, double-bracketed, the kind of reference that only means something if you know where the bodies are buried.

So I built a place to bury them. Called it the vault. It walked the whole root, indexed every outgoing reference, and learned to answer one question better than most witnesses: *who points back here?* Backlinks. The reverse of a thing always tells you more than the thing.

I built the diagnostics first — before the feature, before the glory. A ring buffer, a JSON-line logger, a footer that flashed a warning and forgot it three seconds later, like a man who's seen too much. Everybody told me to build the feature first. Everybody's wrong about that. When the wikilink resolver started missing on a name, the logger was already sitting there with its hat on, ready to tell me exactly who'd lied.

Then came the modals. The first one swung open and I made a rule on the spot, the kind of rule you make once and keep forever: one modal at a time. They swap, they don't stack. A man with two open dialogs is a man who's lost the thread. `?` was the one exception — the help screen, too polite to shove another modal out of the way mid-job.

The picker came in around then too, riding the first six pull requests. `^p`. A way to open any file in the joint without crawling the tree on your hands and knees. I didn't know yet how much that little door would matter. You never do.

## III. The long goodbye to the side pane

The ninth was a bloodbath of housekeeping. I carved the model into four pieces because a god-object is just a suspect with too many alibis. I split the watcher, deduped two wikilink parsers into one honest package, and swept the docs until they matched the floor plan.

Then the links got dressed up. Dotted underline, URL hidden — you saw the name, not the ugly address it lived at. I tried OSC 8 hyperlinks, the fancy terminal kind. Looked sharp for about an hour. Then I caught it picking the pocket of my mouse hit-testing, lifting clicks clean off the BubbleZone tracker. I put two in it and reverted the commit. Some upgrades are just downgrades in a better suit.

But the twelfth. The twelfth was when the dame walked in.

She called herself the finder. Recency-ranked, fuzzy-filtered, type a few letters and she'd surface the file you wanted before you'd finished wanting it. Hybrid score — modification time and visit history, with mtime winning the close calls. I gave her her own little package, `internal/recent`, and a place to keep her secrets in atomic JSON so a crash couldn't catch her mid-sentence.

And she changed everything. I'd been running a two-pane joint — tree on the left, content on the right, the way everybody does it. But she was better at navigation than the tree ever was. So I did the hard thing. I took the side pane out back. Hid it by default, then folded it into a modal you opened with `^b` when you actually needed it. Content filled the screen now. The finder was the front door. The tree was just a room you could visit.

It's an ugly business, retiring the thing you built first. But you don't keep a partner around out of sentiment. Not in this line of work.

## IV. The day everything shipped

The thirteenth came in like nine cases stacked on one desk.

First the code files. Somebody wanted to read a `.go` or a `.py` in the same window, so I ran them through Chroma, painted them in 256 colors, bolted on a line-number gutter, and soft-wrapped the long lines. Chroma left a phantom gutter row hanging off the bottom — a reset escape with nobody behind it. I'd seen that trick before. Suppressed it and moved on. The SGR state kept dropping its coat at every wrap boundary, so I made it carry its own colors across the line break. You want a thing done right, you make it remember what it was wearing.

Then the embeds. `![[file.go#L10-L20]]` — pull a slice of source straight into the page, fenced and guttered, live-synced so editing the source updated the view. Four follow-up fixes came knocking after that one merged, the way they do. Skip the fenced blocks. Honor the no-scroll sentinel. Don't lose the reader's place on Esc. I paid them all. You always pay the follow-ups; the only question is whether you pay now or at 2 a.m.

The wrapped-link cursor I wrote test-first — a failing test, then the fix, the honest way. Made the highlight reopen on every wrapped row, the way `less` and `vim` do it, so a link that ran across three lines stayed lit the whole way.

And then, because a man's work ought to be worth something, I cut the first release. A `--version` flag, a GoReleaser pipeline that fired on any `v*` tag, archives for every platform that mattered. v0.1.2. The CHANGELOG started committing itself. I'd built a thing that could ship itself out the door while I slept. That's either progress or the beginning of the end, and in this town they look the same.

## V. Search, and the broken ones

Search came mid-month, and I built it clean — a pure scanner with no idea the TUI even existed. Fan out across every file, skip the binaries, cancel the whole run the instant the next keystroke landed. Debounced at 150 milliseconds so it wouldn't chase every twitch. Type a query, get hits ranked by recency, hit `Enter`, land on the exact line. Five bugs jumped me the day after it shipped — stale hits, a backspace that didn't, a repaint that wouldn't. I cleaned them up one at a time. v0.2.0.

The twenty-fifth was quieter. I taught it to count its own broken links and post the tally in the footer. The unresolved wikilinks I left as plain text with a little `?` — visible, but I kept them out of the link cycler. A link you can't follow has no business pretending it leads somewhere. I've followed enough dead leads to know the difference.

## VI. Two dialects and a stubborn table

June first, and the keybindings got political.

Some folks wanted vim. Some wanted the modern stuff, the VS Code idioms, the browser muscle memory. I wasn't going to referee that fight, so I built a factory — `pagerKeys` for the old school, `modernKeys` for the rest — and let a TOML config pick the winner. The dispatch code never learned which dialect was in play. It just asked *does this key match* and did as it was told. That's the secret to surviving in this business: don't make the muscle care about the politics.

The tables had been truncating long cells mid-word for weeks, chopping content off at the knee. Turned out the old Glamour was hard-coding inline mode on every cell. The new version, 0.10.0, finally let them wrap — but it came with strings: a new footer that re-emitted every link, separator characters that didn't match my own. I pinned the separators, killed the footer, kept the wrap. You take the upgrade and you frisk it for surprises. v0.3.0.

## VII. Drag, copy, and a look in the mirror

June twelfth. The mouse learned to grab.

Press in the content pane, drag, release — and the selected text went to the clipboard. The trick was telling a drag from a click: the first motion event made it a drag, so a press-and-release with no travel still followed the link under your finger. I copied two ways at once — the OS clipboard *and* an OSC 52 escape — because OSC 52 alone couldn't reach macOS Terminal.app, and the OS clipboard alone couldn't reach across an SSH line. Belt and suspenders. In this town you don't bet your copy on one wire.

Then `y` to copy the current file's path, in both dialects, surfaced in the help sheet so nobody had to guess.

And then I did the thing nobody pays you to do. I sat down and reviewed the whole joint through a domain-driven lens. Wrote it all out — the path-resolution logic triplicated across three packages, the model still carrying too much, the highlight markers duplicated in two places. I didn't fix it that day. I just wrote down where the bodies were. v0.4.0 shipped, and I went home with a list.

## VIII. Cleaning up loose ends

The thirteenth I worked the list. Four refactors, four pull requests, each one cutting along a single reason to change. Pulled the path resolution into one honest helper. Centralized the highlight markers. Split the navigation intent out of the status line. Carved up the render file so each piece had exactly one reason to wake me at night.

That's where it stands. The content fills the screen. The links go somewhere. The finder opens the door, the search finds the line, the vault remembers who pointed where. There's still work out there — block references, a configurable vault root, the cases that haven't walked in yet.

They always walk in eventually. That's the thing about a codebase. It's never finished. It just gets quiet for a while, the rain easing off the window, the cursor blinking, waiting for the next one to come through the door with a problem and a deadline and that same tired look that says *I heard you were good at this.*

I poured myself two fingers of `go test ./...`, watched it all come back green, and called it a night.

*— fin*
