/* Just a Smidge — rendering logic. No framework, no build step. */
(function () {
  "use strict";

  // ---- helpers -------------------------------------------------------------

  function byId(id) {
    return PROFILES.find(function (p) { return p.id === id; });
  }

  function popularityScore(p) {
    var s = p.stats || {};
    return (s.likes || 0) + (s.superLikes || 0) * 3 + (s.matches || 0) * 2;
  }

  // Sorted by popularity, but the star is force-pinned to the front.
  function rankedProfiles() {
    var rest = PROFILES.filter(function (p) { return p.id !== CONFIG.star; })
      .sort(function (a, b) { return popularityScore(b) - popularityScore(a); });
    var star = byId(CONFIG.star);
    return star ? [star].concat(rest) : rest;
  }

  function fmt(n) {
    return (n || 0).toLocaleString();
  }

  function el(tag, cls, html) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (html != null) e.innerHTML = html;
    return e;
  }

  function escapeHtml(s) {
    return String(s == null ? "" : s)
      .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function check(verified) {
    return verified ? ' <span class="verified" title="Verified">✓</span>' : "";
  }

  function cover(p) {
    return (p.photos && p.photos[0]) || "assets/img/placeholder-1.svg";
  }

  // ---- shared chrome -------------------------------------------------------

  function applyBranding() {
    document.title = CONFIG.appName;
    var n = document.querySelectorAll("[data-app-name]");
    n.forEach(function (e) { e.textContent = CONFIG.appName; });
    var t = document.querySelectorAll("[data-tagline]");
    t.forEach(function (e) { e.textContent = CONFIG.tagline; });
  }

  // ---- home / leaderboard --------------------------------------------------

  function renderHome() {
    var list = document.getElementById("leaderboard");
    if (!list) return;

    var heading = document.getElementById("leaderboard-heading");
    if (heading) heading.textContent = CONFIG.leaderboardHeading || "Most Popular";

    var ranked = rankedProfiles();
    ranked.forEach(function (p, i) {
      var rank = i + 1;
      var isStar = p.id === CONFIG.star;
      var card = el("a", "card" + (isStar ? " card--star" : ""));
      card.href = "profile.html?id=" + encodeURIComponent(p.id);

      var badge = isStar
        ? '<span class="rank rank--gold">#1 · Most Wanted</span>'
        : '<span class="rank">#' + rank + "</span>";

      card.innerHTML =
        '<div class="card__photo" style="background-image:url(\'' +
        escapeHtml(cover(p)) + '\')">' + badge + "</div>" +
        '<div class="card__body">' +
        '<h3 class="card__name">' + escapeHtml(p.name) +
        (p.age ? ", " + escapeHtml(p.age) : "") + check(p.verified) + "</h3>" +
        '<p class="card__tagline">' + escapeHtml(p.tagline || "") + "</p>" +
        '<div class="card__stats">' +
        '<span title="Likes">❤️ ' + fmt(p.stats && p.stats.likes) + "</span>" +
        '<span title="Super Likes">⭐ ' + fmt(p.stats && p.stats.superLikes) + "</span>" +
        '<span title="Matches">💬 ' + fmt(p.stats && p.stats.matches) + "</span>" +
        "</div></div>";

      list.appendChild(card);
    });
  }

  // ---- profile detail ------------------------------------------------------

  function getParam(name) {
    return new URLSearchParams(window.location.search).get(name);
  }

  function renderProfile() {
    var root = document.getElementById("profile");
    if (!root) return;

    var id = getParam("id") || CONFIG.star;
    var p = byId(id);
    if (!p) {
      root.innerHTML = '<p class="empty">No such profile. <a href="index.html">Back to the app</a></p>';
      return;
    }

    document.title = p.name + " · " + CONFIG.appName;

    var photos = (p.photos && p.photos.length ? p.photos : ["assets/img/placeholder-1.svg"]);
    var gallery = photos.map(function (src) {
      return '<div class="gallery__slide" style="background-image:url(\'' +
        escapeHtml(src) + '\')"></div>';
    }).join("");

    var tags = (p.tags || []).map(function (t) {
      return '<span class="chip">' + escapeHtml(t) + "</span>";
    }).join("");

    var prompts = (p.prompts || []).map(function (pr) {
      return '<div class="prompt"><div class="prompt__q">' + escapeHtml(pr.q) +
        '</div><div class="prompt__a">' + escapeHtml(pr.a) + "</div></div>";
    }).join("");

    var isStar = p.id === CONFIG.star;

    root.innerHTML =
      '<div class="gallery">' + gallery + "</div>" +
      '<div class="profile__body">' +
      (isStar ? '<div class="banner">👑 #1 Most Popular · The undisputed champion of ' +
        escapeHtml(CONFIG.appName) + "</div>" : "") +
      '<h1 class="profile__name">' + escapeHtml(p.name) +
      (p.age ? ", " + escapeHtml(p.age) : "") + check(p.verified) + "</h1>" +
      (p.location ? '<p class="profile__loc">📍 ' + escapeHtml(p.location) + "</p>" : "") +
      (p.tagline ? '<p class="profile__tagline">' + escapeHtml(p.tagline) + "</p>" : "") +
      '<div class="profile__stats">' +
      '<div><strong>' + fmt(p.stats && p.stats.likes) + "</strong><span>Likes</span></div>" +
      '<div><strong>' + fmt(p.stats && p.stats.superLikes) + "</strong><span>Super Likes</span></div>" +
      '<div><strong>' + fmt(p.stats && p.stats.matches) + "</strong><span>Matches</span></div>" +
      "</div>" +
      (p.bio ? '<p class="profile__bio">' + escapeHtml(p.bio) + "</p>" : "") +
      (tags ? '<div class="chips">' + tags + "</div>" : "") +
      (prompts ? '<div class="prompts">' + prompts + "</div>" : "") +
      '<div class="actions">' +
      '<button class="btn btn--nope" type="button" data-action="nope">✕ Nope</button>' +
      '<button class="btn btn--super" type="button" data-action="super">⭐ Super Like</button>' +
      '<button class="btn btn--like" type="button" data-action="like">❤️ Like</button>' +
      "</div>" +
      '<p class="toast" id="toast" hidden></p>' +
      "</div>";

    wireActions(root, p);
  }

  // The swipe buttons are a gag: liking the star always "matches".
  function wireActions(root, p) {
    var toast = root.querySelector("#toast");
    var isStar = p.id === CONFIG.star;
    function say(msg) {
      if (!toast) return;
      toast.textContent = msg;
      toast.hidden = false;
    }
    root.querySelectorAll("[data-action]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        var a = btn.getAttribute("data-action");
        if (a === "nope") {
          say(isStar ? "You can't actually say no to " + p.name + ". Nice try." : "Noped. Brutal.");
        } else if (a === "super") {
          say("⭐ It's a match! (It was always going to be a match.)");
        } else {
          say(isStar ? "❤️ It's a match! Of course it is." : "❤️ Liked!");
        }
      });
    });
  }

  // ---- boot ----------------------------------------------------------------

  document.addEventListener("DOMContentLoaded", function () {
    applyBranding();
    renderHome();
    renderProfile();
  });
})();
