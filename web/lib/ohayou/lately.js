/*
 * GET /ohayou/api/lately   the chronicle
 *
 * Public, like the world. The bot decides who is named before it publishes, so
 * there is nothing to withhold here: a row whose actor is empty is one whose
 * actor asked to be off the map.
 *
 * ?nick=<name> narrows it to one player's share, matched at either end of an
 * event. It matches actor and subject rather than filtering after, so a name
 * that was withheld cannot be found by asking for it.
 */

import { guard, json, parseColumn } from "../http.js";

/** What one page of the feed holds. The bot publishes 200. */
const LIMIT = 60;

const MAX_NICK = 48;

export const onRequestGet = guard(async ({ request, env }) => {
	if (!env.GAME) return json({ status: "lately", events: [], updated: 0 });

	const wanted = new URL(request.url).searchParams.get("nick");
	if (wanted !== null && (!wanted || wanted.length > MAX_NICK)) {
		return json({ status: "lately", events: [], updated: 0 });
	}

	const { results } = wanted
		? await env.GAME.prepare(
				`SELECT id, ts, kind, actor, subject, detail FROM event
         WHERE actor = ?1 COLLATE NOCASE OR subject = ?1 COLLATE NOCASE
         ORDER BY id DESC LIMIT ?2`,
			)
				.bind(wanted, LIMIT)
				.all()
		: await env.GAME.prepare(
				`SELECT id, ts, kind, actor, subject, detail FROM event
         ORDER BY id DESC LIMIT ?1`,
			)
				.bind(LIMIT)
				.all();

	const published = await env.GAME.prepare(
		"SELECT updated FROM publish WHERE plugin = 'ohayou' AND table_name = 'event'",
	).first();

	return json(
		{
			status: "lately",
			events: results.map(row),
			updated: published?.updated ?? 0,
		},
		// The bot publishes every couple of minutes.
		{ headers: { "cache-control": "public, max-age=30" } },
	);
});

export function row(record) {
	return {
		id: record.id,
		ts: record.ts,
		kind: record.kind,
		actor: record.actor ?? "",
		subject: record.subject ?? "",
		detail: parseColumn(record.detail, {}),
	};
}
