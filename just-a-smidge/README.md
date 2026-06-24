# Just a Smidge 🔥

A purely-for-laughs **gag dating app** — a static website you fill with your own
photos and bios, rigged so one friend is permanently the #1 "Most Wanted" person
on the app. Great as a birthday roast / inside-joke gift.

> Keep it kind: this is a roast *with* your friends, not at anyone's expense.
> Only use photos of people who are in on the joke.

## What it is

- 100% static — plain HTML/CSS/JS, **no build step, no dependencies**.
- All content lives in **one file**: [`assets/data.js`](assets/data.js).
- A leaderboard homepage + a tappable profile page per person.
- The friend you choose is **force-pinned to #1** no matter the numbers.

## Run it

Just open `index.html` in a browser. For the gallery/links to behave exactly
like in production, serve the folder over HTTP:

```sh
cd just-a-smidge
python3 -m http.server 8000
# then visit http://localhost:8000
```

## Make it yours (the only file you edit)

Open [`assets/data.js`](assets/data.js):

1. **Pick the star.** Set `CONFIG.star` to the `id` of your friend's profile.
   They'll always be #1.
2. **Add photos.** Drop images into `assets/img/` and list their paths in each
   profile's `photos` array (first photo is the cover). Placeholders are
   included so it looks complete until you do.
3. **Write the profiles.** Each person is one object in the `PROFILES` array —
   copy a block to add more. Fields: `name`, `age`, `location`, `tagline`,
   `bio`, `tags`, `prompts` (the Q/A cards), `stats` (likes/superLikes/matches),
   and `verified`.
4. **Theme the copy.** `CONFIG.appName`, `CONFIG.tagline`, and
   `CONFIG.leaderboardHeading` set the branding shown across the site.

That's it — save and reload.

## Deploy (free)

Any static host works since there's no backend:

- **GitHub Pages** — push and enable Pages on the folder/branch.
- **Netlify / Vercel / Cloudflare Pages** — drag-and-drop the `just-a-smidge`
  folder, no config needed.

## Layout

```
just-a-smidge/
  index.html          leaderboard homepage
  profile.html        single profile (reads ?id= from the URL)
  assets/
    data.js           ← all content + config (edit this)
    app.js            rendering logic
    styles.css        styling
    img/              photos + placeholder art
```
