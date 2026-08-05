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

import { usage } from "../../public/ohayou/plot.js";
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

/**
 * The shell, written out rather than built by a script: the first thing to
 * fetch this is a crawler, and it runs none.
 */
function shell(title, head, body) {
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${esc(title)}</title>
<meta name="color-scheme" content="dark">
${head}
<link rel="stylesheet" href="/site.css">
<link rel="stylesheet" href="/ohayou/style.css">
<link rel="icon" href="/deerkins/favicon.svg" type="image/svg+xml">
</head>
<body>
<header class="shell">
  <div>
    <a class="brand" href="/"><b>hemera</b>.day <span>land office</span></a>
    <nav class="sitenav" aria-label="Sections">
      <a href="/">home</a>
      <a href="/ohayou/">registry</a>
      <a href="/deerkins/">deerkins</a>
      <a href="/drop/">drop</a>
    </nav>
  </div>
</header>
<main class="page">
${body}
</main>
</body>
</html>`;
}

function page(plot, image, href, { channel, network }) {
	const title = `${plot.nick}'s territory`;
	const where = [channel, network].filter(Boolean).join(" on ");
	const { acres, built } = usage(plot);
	const summary = `${acres} ${
		acres === 1 ? "acre" : "acres"
	}, ${plot.wealth}, ${plot.rations} rations drawn${
		where ? `. Filed from ${where}.` : "."
	}`;

	const head = `<meta name="description" content="${esc(summary)}">
<meta property="og:type" content="profile">
<meta property="og:title" content="${esc(title)}">
<meta property="og:description" content="${esc(summary)}">
<meta property="og:image" content="${esc(image)}">
<meta property="og:url" content="${esc(href)}">
<meta name="twitter:card" content="summary_large_image">`;

	const body = `  <section class="certificate">
    <img src="${esc(image)}" alt="${esc(title)}" width="1200" height="630">
  </section>

  <section class="masthead">
    <div>
      <h1>${esc(plot.nick)}</h1>
      <p class="lede">
        Parcel filed with the land office${where ? ` from ${esc(where)}` : ""},
        and surveyed every couple of minutes.
      </p>
    </div>
    <dl class="ledger">
      <div><dd>${acres}</dd><dt>acres held</dt></div>
      <div><dd>${built}</dd><dt>acres worked</dt></div>
      <div><dd>${plot.rations}</dd><dt>rations drawn</dt></div>
      <div class="filed"><dd>${esc(plot.wealth)}</dd><dt>standing</dt></div>
    </dl>
  </section>

  <section class="panel notice">
    <span class="stamp">What this is</span>
    <p>
      An IRC bot hands one ration a day to anyone who says good morning to it.
      Rations buy acres, and acres carry cats, quarries and refineries. Above is
      what ${esc(plot.nick)} has done with theirs.
    </p>
    <p>
      Say <code>!ohayou</code>${
				where ? ` in ${esc(where)}` : ""
			} and the office will find you an acre of your own.
    </p>
    <p><a href="/ohayou/">Every parcel on the register</a></p>
  </section>`;

	return shell(title, head, body);
}

function missing() {
	return shell(
		"No parcel by that name",
		'<meta name="robots" content="noindex">',
		`  <section class="panel notice">
    <span class="stamp">Nothing on file</span>
    <h1>No parcel by that name</h1>
    <p>
      Either nobody by that name plays, or they have not filed their name
      against their land. Only a parcel whose holder asked to be named has a
      page here.
    </p>
    <p><a href="/ohayou/">Every parcel on the register</a></p>
  </section>`,
	);
}
