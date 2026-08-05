/*
 * GET /ohayou/api/me   your own standing
 *
 * The private tier: exact balances, the vault, equipment, defences and what is
 * still counting down. Served only against a session holding the matching
 * account.
 *
 * There is deliberately no route that takes an account, and no listing
 * endpoint. The only account this can answer for is the one in the cookie, so
 * there is no parameter to tamper with and nothing to enumerate.
 */

import { SCOPE_OHAYOU } from "../hmac.js";
import { fail, guard, json } from "../http.js";
import { requireScope } from "../session.js";

export const onRequestGet = guard(async ({ request, env }) => {
	const session = await requireScope(request, env, SCOPE_OHAYOU);
	if (!session) return fail(401, "no session");

	if (!env.GAME) return fail(503, "the game is not published here");

	const row = await env.GAME.prepare(
		`SELECT nick, ohayous, cumulative, items, metals, equipped, defense,
            vault, probation, fortune, running
     FROM plot_private WHERE account = ?1`,
	)
		.bind(session.account)
		.first();

	// Everyone is on the map, but only those who named their plot have a
	// private row. Not an error: it is what "you have not claimed yours" looks
	// like, and the page says so.
	if (!row) {
		return json(
			{ status: "unclaimed", account: session.account, nick: session.nick },
			{ headers: { "cache-control": "no-store" } },
		);
	}

	return json(
		{
			status: "standing",
			account: session.account,
			nick: row.nick,
			ohayous: row.ohayous,
			cumulative: row.cumulative,
			items: parse(row.items, {}),
			metals: parse(row.metals, {}),
			equipped: parse(row.equipped, {}),
			defense: row.defense,
			vault: parse(row.vault, null),
			probation: row.probation,
			fortune: row.fortune,
			running: parse(row.running, []),
		},
		// Never cached: this is one person's, and a shared cache holding it is
		// the one way it could reach somebody else.
		{ headers: { "cache-control": "no-store, private" } },
	);
});

/** A row this side did not write is not worth crashing over. */
function parse(raw, fallback) {
	if (raw === null || raw === undefined) return fallback;
	try {
		return JSON.parse(raw);
	} catch {
		return fallback;
	}
}
