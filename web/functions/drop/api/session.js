/*
 * GET    /drop/api/session   who the cookie says you are
 * POST   /drop/api/session   redeem a grant from irc for a cookie
 * DELETE /drop/api/session   give it back
 */

import { verifyGrant } from "../../../lib/hmac.js";
import {
	fail,
	guard,
	json,
	readJson,
	rejectCrossOrigin,
	rejectForeignOrigin,
} from "../../../lib/http.js";
import {
	clearSession,
	issueSession,
	readSession,
} from "../../../lib/session.js";

const NO_STORE = { "cache-control": "no-store" };

export const onRequestGet = guard(async ({ request, env }) => {
	const session = await readSession(request, env);
	if (!session) return fail(401, "no session");

	return json({ status: "session", ...session }, { headers: NO_STORE });
});

export const onRequestPost = guard(async ({ request, env }) => {
	const blocked = rejectCrossOrigin(request);
	if (blocked) return blocked;

	if (!env.UPLOAD_HMAC_SECRET) return fail(503, "uploads are not configured");

	const { body, error } = await readJson(request);
	if (error) return fail(400, error);

	const grant = await verifyGrant(body.token, env.UPLOAD_HMAC_SECRET);
	if (!grant.ok) return refuse(grant.reason);

	if (!(await claim(env, grant.payload))) return refuse("already redeemed");

	const cookie = await issueSession(env, {
		account: grant.payload.a,
		nick: grant.payload.n,
		channels: grant.payload.c,
	});

	return json(
		{
			status: "session",
			account: grant.payload.a,
			nick: grant.payload.n,
			channels: grant.payload.c,
		},
		{ headers: { ...NO_STORE, "set-cookie": cookie } },
	);
});

export const onRequestDelete = guard(async ({ request, env }) => {
	// A cross-origin DELETE always preflights and there is no OPTIONS handler,
	// so this only has to catch a client that skips the preflight.
	const blocked = rejectForeignOrigin(request);
	if (blocked) return blocked;

	const cookie = await clearSession(request, env);
	return json(
		{ status: "ended" },
		{ headers: { ...NO_STORE, "set-cookie": cookie } },
	);
});

/**
 * Records the grant's id, returning whether this is the first time. The insert
 * decides it, so two simultaneous redemptions of one link cannot both win.
 *
 * exp is in seconds, as it is on the wire and in grant_used. Everywhere else in
 * this database time is milliseconds.
 */
async function claim(env, payload) {
	const [, claimed] = await env.DB.batch([
		env.DB.prepare("DELETE FROM grant_used WHERE exp < ?1").bind(
			Math.floor(Date.now() / 1000),
		),
		env.DB.prepare(
			"INSERT OR IGNORE INTO grant_used (jti, exp) VALUES (?1, ?2)",
		).bind(payload.j, payload.e),
	]);

	return claimed.meta.changes === 1;
}

/** One answer for every reason, so a probe cannot tell a spent link from a
    forged one. The reason goes to the log, which is where "my link doesn't
    work" gets diagnosed. */
function refuse(reason) {
	console.log(`grant refused: ${reason}`);
	return fail(401, "that link is expired, already used, or not valid");
}
