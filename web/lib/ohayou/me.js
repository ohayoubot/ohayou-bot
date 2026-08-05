/*
 * GET /ohayou/api/me   your own standing
 *
 * Exact balances, the vault, equipment, defences and what is counting down.
 * There is no route taking an account and no listing endpoint: the query binds
 * the session's account, so there is no parameter to tamper with.
 */

import { SCOPE_OHAYOU } from "../hmac.js";
import { fail, guard, json, parseColumn } from "../http.js";
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

	// Only a named plot has a private row. Not an error: the page says so.
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
			items: parseColumn(row.items, {}),
			metals: parseColumn(row.metals, {}),
			equipped: parseColumn(row.equipped, {}),
			defense: row.defense,
			vault: parseColumn(row.vault, null),
			probation: row.probation,
			fortune: row.fortune,
			running: parseColumn(row.running, []),
		},
		// A shared cache holding this is the one way it reaches anybody else.
		{ headers: { "cache-control": "no-store, private" } },
	);
});
