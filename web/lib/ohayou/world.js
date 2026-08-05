/*
 * GET /ohayou/api/world   every plot on the map
 *
 * Public. The bot publishes a plot for every player; an unnamed one carries an
 * opaque id, no nick and no buildings, so there is nothing here to withhold.
 *
 * A named plot may fly a deer. The gallery is the other database and D1 cannot
 * join across two, so the art is fetched in a second query and returned beside
 * the plots rather than inside them: a deer twenty people fly is sent once.
 */

import { guard, json, parseColumn } from "../http.js";

/** One row per player who has ever ohayou'd, which does not need paging yet. */
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
		// The bot publishes neither for an unnamed plot; this declines to repeat
		// one if a row ever carried it.
		nick: row.named ? row.nick : "",
		named: row.named === 1,
		flag: row.named ? row.flag : "",
		acres: row.acres,
		land: parseColumn(row.land, {}),
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
			updated: published?.updated ?? 0,
		},
		// The bot publishes every couple of minutes.
		{ headers: { "cache-control": "public, max-age=30" } },
	);
});

/**
 * The art for every flag flown, by deer name. A name matching nothing is
 * absent: a deer may have been renamed since somebody picked it.
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

/** Counted from the rows, so the totals cannot disagree with them. */
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
