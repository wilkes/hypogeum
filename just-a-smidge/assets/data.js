/* =============================================================================
 * Just a Smidge — content file
 * -----------------------------------------------------------------------------
 * This is the ONLY file you need to edit to fill the app with real content.
 * No build step. Just edit, save, and reload the page.
 *
 * Quick start:
 *   1. Set CONFIG.star to the `id` of the friend who should be #1.
 *   2. Drop photos into assets/img/ and reference them in `photos`.
 *   3. Add/edit profiles in the PROFILES array below.
 * ===========================================================================*/

const CONFIG = {
  // Shown in the header / browser tab.
  appName: "Just a Smidge",
  tagline: "The dating app where less is more.",

  // The `id` of the friend who is ALWAYS pinned to #1 most popular,
  // no matter what the numbers say. This is the whole point. :)
  star: "the-star",

  // Flavor copy for the leaderboard heading.
  leaderboardHeading: "🔥 Most Wanted This Week",
};

/* Each profile is one object. Copy a block to add another person.
 *
 *   id         unique slug, used in the URL (profile.html?id=...)
 *   name       display name
 *   age        number (optional, purely cosmetic)
 *   location   short string, e.g. "2 miles away"
 *   tagline    one-liner under the name
 *   bio        longer paragraph
 *   photos     array of image paths (assets/img/...). First is the cover.
 *   tags       short interest chips
 *   prompts    array of { q, a } — the little dating-app prompt cards
 *   stats      { likes, superLikes, matches } — numbers shown on the profile
 *   verified   true shows a blue check
 *
 * Popularity ranking uses stats, BUT the CONFIG.star is force-pinned to #1.
 */
const PROFILES = [
  {
    id: "the-star",
    name: "Your Friend",
    age: 29,
    location: "Right here, somehow",
    tagline: "Booked, busy, and absolutely irresistible.",
    bio:
      "Replace this with the real (lovingly exaggerated) bio. The whole app " +
      "is rigged so this person sits at #1 forever. Add their best/funniest " +
      "photos to the `photos` array and pile on the in-jokes.",
    photos: ["assets/img/placeholder-1.svg"],
    tags: ["Local Legend", "Certified Catch", "5-Star Rated"],
    prompts: [
      { q: "The way to my heart is", a: "Honestly, you're already 90% there." },
      { q: "My most controversial opinion", a: "That I'm only the #1 most popular person on here. Feels low." },
      { q: "We'll get along if", a: "you can keep up. (You can't.)" },
    ],
    stats: { likes: 9999, superLikes: 4040, matches: 1872 },
    verified: true,
  },
  {
    id: "rival-1",
    name: "Sam",
    age: 31,
    location: "5 miles away",
    tagline: "Tried their best. It wasn't enough.",
    bio: "A perfectly nice person who simply was not built different. Swap in a friendly rival.",
    photos: ["assets/img/placeholder-2.svg"],
    tags: ["Tries Hard", "Decent Playlist"],
    prompts: [
      { q: "A green flag I look for", a: "Someone who isn't already #1 on this app. (Impossible to find.)" },
    ],
    stats: { likes: 412, superLikes: 18, matches: 63 },
    verified: false,
  },
  {
    id: "rival-2",
    name: "Jordan",
    age: 27,
    location: "8 miles away",
    tagline: "So close to peaking. So far.",
    bio: "The runner-up's runner-up. Add another good-natured contender here.",
    photos: ["assets/img/placeholder-3.svg"],
    tags: ["Gym Selfie Enthusiast", "Two Dogs"],
    prompts: [
      { q: "My simple pleasures", a: "Being the second-ish most popular person in a 10-mile radius." },
    ],
    stats: { likes: 388, superLikes: 22, matches: 51 },
    verified: false,
  },
  {
    id: "rival-3",
    name: "Riley",
    age: 30,
    location: "12 miles away",
    tagline: "Showed up. Participated. We respect it.",
    bio: "Fourth place and at peace with it. Replace with another friend.",
    photos: ["assets/img/placeholder-4.svg"],
    tags: ["Brunch", "Owns a Kayak"],
    prompts: [
      { q: "Dating me is like", a: "settling, but in a nice way." },
    ],
    stats: { likes: 201, superLikes: 9, matches: 30 },
    verified: false,
  },
];
