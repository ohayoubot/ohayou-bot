/*
 * GET /ohayou/api/world   every plot on the map
 *
 * Public: this is the tier the bot publishes for everyone, named or not. A
 * plot with named=false carries an opaque id, no nick and no buildings, so
 * there is nothing here to withhold and no session to check.
 *
 * The private tier is a different endpoint with a different rule.
 *
 * A plot may fly a deer from the gallery. The gallery is the other database, so
 * this resolves the names in a second query and hands back the art with the
 * plot: D1 cannot join across databases, but a worker holding both bindings can
 * ask each of them once.
 */

import { guard, json } from "../http.js";

/** The whole world in one answer. It is one row per player who has ever
    ohayou'd, which is not a number that needs paging yet. */
const LIMIT = 500;

/** Distinct deer to look up in one go. Far more than anyone will fly. */
const MAX_FLAGS = 100;

export const onRequestGet = guard(async ({ env }) => {
	if (!env.GAME) return json({ status: "world", plots: [], totals: empty() });

	const { results } = await env.GAME.prepare(
		`SELECT id, nick, named, flag, acres, land, wealth, rations
     FROM plot ORDER BY rations DESC, id LIMIT ?1`,
	)
		.bind(LIMIT)
		.all();

	const plots = results.map((row) => ({
		id: row.id,
		// An unnamed plot carries neither a nick nor a flag: a chosen picture is
		// as good as a name. The bot does not publish one, and this declines to
		// repeat it if a row ever did.
		nick: row.named ? row.nick : "",
		named: row.named === 1,
		flag: row.named ? row.flag : "",
		acres: row.acres,
		land: parse(row.land),
		wealth: row.wealth,
		rations: row.rations,
	}));

	const flags = await banners(env, plots);

	const published = await env.GAME.prepare(
		"SELECT updated FROM publish WHERE plugin = 'ohayou' AND table_name = 'plot'",
	).first();

	return json(
		{
			status: "world",
			plots,
			flags,
			totals: totals(plots),
			// When the bot last said anything, so a page can admit to being stale.
			updated: published?.updated ?? 0,
		},
		// Short: the bot publishes every couple of minutes, and a map that lags
		// a minute behind is fine, while one that lags an hour looks broken.
		{ headers: { "cache-control": "public, max-age=30" } },
	);
});

/**
 * The art for every flag flown, by deer name. Returned beside the plots rather
 * than inside them so a deer flown by twenty people is sent once.
 *
 * A name that matches nothing is simply absent: somebody may have picked a deer
 * that was later renamed, and a plot with no banner is a smaller problem than a
 * map that will not draw.
 */
async function banners(env, plots) {
	if (!env.DB) return {};

	const names = [...new Set(plots.map((p) => p.flag).filter(Boolean))].slice(
		0,
		MAX_FLAGS,
	);
	if (names.length === 0) return {};

	const holes = names.map((_, i) => `?${i + 1}`).join(", ");
	const { results } = await env.DB.prepare(
		`SELECT deer, kinskode FROM deer WHERE deer IN (${holes})`,
	)
		.bind(...names)
		.all();

	const out = {};
	for (const row of results) out[row.deer] = row.kinskode;
	return out;
}

/** Derived here rather than stored, so the totals cannot disagree with the
    rows they are totals of. */
function totals(plots) {
	const out = empty();
	for (const plot of plots) {
		out.players++;
		out.acres += plot.acres;
		if (plot.named) out.named++;
	}
	return out;
}

function empty() {
	return { players: 0, named: 0, acres: 0 };
}

/** A row this side did not write is not worth crashing over. */
function parse(raw) {
	try {
		const value = JSON.parse(raw);
		return value && typeof value === "object" && !Array.isArray(value)
			? value
			: {};
	} catch {
		return {};
	}
}
