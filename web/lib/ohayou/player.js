/*
 * GET /ohayou/p/<nick>            one player's plot, as a page
 * GET /ohayou/api/card/<nick>     the same plot, as an svg
 *
 * Rendered here rather than in the browser: the first thing to fetch a
 * permalink is usually a link preview, which reads meta tags out of the html it
 * is handed and will not run a script.
 *
 * Only named plots have a page. The query matches named = 1 rather than
 * filtering after, so it cannot be forgotten.
 */

import {
	decodeParam,
	escapeHTML as esc,
	fail,
	guard,
	parseColumn,
} from "../http.js";
import { card } from "./card.js";

const MAX_NICK = 48;

function place(env) {
	return { channel: env.IRC_CHANNEL, network: env.IRC_NETWORK };
}

async function lookup(env, name) {
	if (!env.GAME) return null;

	const plot = await env.GAME.prepare(
		`SELECT id, nick, flag, acres, land, wealth, rations
     FROM plot WHERE named = 1 AND nick = ?1 COLLATE NOCASE`,
	)
		.bind(name)
		.first();

	if (!plot) return null;
	plot.land = parseColumn(plot.land, {});

	// The gallery is the other database; a worker holding both asks each once.
	let flag = null;
	if (plot.flag && env.DB) {
		const deer = await env.DB.prepare(
			"SELECT kinskode FROM deer WHERE deer = ?1",
		)
			.bind(plot.flag)
			.first();
		flag = deer?.kinskode ?? null;
	}
	return { plot, flag };
}

function wanted(params) {
	const name = decodeParam(params.name);
	if (name === null) return null;
	const trimmed = name.replace(/\.svg$/, "").trim();
	return trimmed && trimmed.length <= MAX_NICK ? trimmed : null;
}

export const onRequestGetCard = guard(async ({ params, env }) => {
	const name = wanted(params);
	if (!name) return fail(400, "no name given");

	const found = await lookup(env, name);
	if (!found) return fail(404, "no plot by that name");

	return new Response(card(found.plot, found.flag, place(env)), {
		headers: {
			"content-type": "image/svg+xml; charset=utf-8",
			"cache-control": "public, max-age=300",
			"content-security-policy":
				"default-src 'none'; style-src 'unsafe-inline'",
			"x-content-type-options": "nosniff",
		},
	});
});

export const onRequestGetPage = guard(async ({ request, params, env }) => {
	const name = wanted(params);
	if (!name) return fail(400, "no name given");

	const found = await lookup(env, name);
	if (!found) {
		return new Response(missing(), {
			status: 404,
			headers: { "content-type": "text/html; charset=utf-8" },
		});
	}

	const url = new URL(request.url);
	const image = `${url.origin}/ohayou/api/card/${encodeURIComponent(
		found.plot.nick,
	)}`;

	return new Response(page(found.plot, image, url.href, place(env)), {
		headers: {
			"content-type": "text/html; charset=utf-8",
			"cache-control": "public, max-age=300",
		},
	});
});

function page(plot, image, href, { channel, network }) {
	const title = `${plot.nick}'s territory`;
	const where = [channel, network].filter(Boolean).join(" on ");
	const summary = `${plot.acres} ${
		plot.acres === 1 ? "acre" : "acres"
	}, ${plot.wealth}, ${plot.rations} rations collected${
		where ? `. Played in ${where}.` : "."
	}`;

	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${esc(title)}</title>
<meta name="description" content="${esc(summary)}">
<meta name="color-scheme" content="dark light">
<meta property="og:type" content="profile">
<meta property="og:title" content="${esc(title)}">
<meta property="og:description" content="${esc(summary)}">
<meta property="og:image" content="${esc(image)}">
<meta property="og:url" content="${esc(href)}">
<meta name="twitter:card" content="summary_large_image">
<link rel="stylesheet" href="/deerkins/style.css">
<link rel="stylesheet" href="/ohayou/style.css">
<link rel="icon" href="/deerkins/favicon.svg" type="image/svg+xml">
</head>
<body>
<header class="topbar">
  <h1>${esc(plot.nick)}</h1>
  <p class="tagline">${esc(summary)}</p>
</header>
<main>
  <section>
    <img src="${esc(image)}" alt="${esc(title)}" width="1200" height="630"
         style="width:100%;height:auto;border-radius:10px">
  </section>
  <section>
    <h2>What this is</h2>
    <p>
      An IRC bot hands out one ration a day to whoever says hello to it. This is
      what ${esc(plot.nick)} has done with theirs${where ? `, in ${esc(where)}` : ""}.
    </p>
    <p>Say <code>!ohayou</code> there and you get a plot of your own.</p>
    <p><a href="/ohayou/">See everyone's land</a></p>
  </section>
</main>
</body>
</html>`;
}

function missing() {
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>No plot by that name</title>
<meta name="color-scheme" content="dark light">
<link rel="stylesheet" href="/deerkins/style.css">
</head>
<body>
<header class="topbar"><h1>No plot by that name</h1></header>
<main><section>
  <p>
    Either nobody by that name plays, or they have not put their name to their
    land. Only a plot whose owner said it was theirs has a page.
  </p>
  <p><a href="/ohayou/">See everyone's land</a></p>
</section></main>
</body>
</html>`;
}
