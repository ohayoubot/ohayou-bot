/*
 * GET /drop/api/uploads   what this account has dropped lately
 *
 * The dashboard's "your recent drops". Keyed on the session's account, so it
 * can only ever show its own: there is no parameter to point it elsewhere.
 */

import { SCOPE_DROP } from "../hmac.js";
import { fail, guard, json } from "../http.js";
import { requireScope } from "../session.js";

/** Enough to recognise what you sent, not a gallery. */
const LIMIT = 12;

export const onRequestGet = guard(async ({ request, env }) => {
	const session = await requireScope(request, env, SCOPE_DROP);
	if (!session) return fail(401, "no session");

	if (!env.PUBLIC_IMAGE_BASE) return fail(503, "uploads are not configured");

	const { results } = await env.DB.prepare(
		"SELECT ts, channel, key FROM upload WHERE account = ?1 ORDER BY id DESC LIMIT ?2",
	)
		.bind(session.account, LIMIT)
		.all();

	const base = env.PUBLIC_IMAGE_BASE.replace(/\/+$/, "");
	return json(
		{
			status: "uploads",
			uploads: results.map((row) => ({
				ts: row.ts,
				channel: row.channel,
				url: `${base}/${row.key}`,
			})),
		},
		{ headers: { "cache-control": "no-store" } },
	);
});
