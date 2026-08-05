/*
 * GET /ohayou/api/world   every plot on the map
 *
 * Public: this is the tier the bot publishes for everyone, named or not. A
 * plot with named=false carries an opaque id, no nick and no buildings, so
 * there is nothing here to withhold and no session to check.
 *
 * The private tier is a different endpoint with a different rule.
 */

import { guard, json } from "../http.js";

/** The whole world in one answer. It is one row per player who has ever
    ohayou'd, which is not a number that needs paging yet. */
const LIMIT = 500;

export const onRequestGet = guard(async ({ env }) => {
	if (!env.GAME) return json({ status: "world", plots: [], totals: empty() });

	const { results } = await env.GAME.prepare(
		`SELECT id, nick, named, acres, land, wealth, rations
     FROM plot ORDER BY rations DESC, id LIMIT ?1`,
	)
		.bind(LIMIT)
		.all();

	const plots = results.map((row) => ({
		id: row.id,
		nick: row.named ? row.nick : "",
		named: row.named === 1,
		acres: row.acres,
		land: parse(row.land),
		wealth: row.wealth,
		rations: row.rations,
	}));

	const published = await env.GAME.prepare(
		"SELECT updated FROM publish WHERE plugin = 'ohayou' AND table_name = 'plot'",
	).first();

	return json(
		{
			status: "world",
			plots,
			totals: totals(plots),
			// When the bot last said anything, so a page can admit to being stale.
			updated: published?.updated ?? 0,
		},
		// Short: the bot publishes every couple of minutes, and a map that lags
		// a minute behind is fine, while one that lags an hour looks broken.
		{ headers: { "cache-control": "public, max-age=30" } },
	);
});

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
