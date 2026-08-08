/*
 * GET    /api/session   who the cookie says you are
 * POST   /api/session   redeem a link from irc for a cookie
 * DELETE /api/session   give it back
 *
 * One session for the whole site. The scopes come from the grant and decide
 * which plugins the cookie reaches, so this does not care which part of the
 * site the link was minted for.
 */

import { verifyGrant } from "../hmac.js";
import {
	fail,
	guard,
	json,
	readJson,
	rejectCrossOrigin,
	rejectForeignOrigin,
} from "../http.js";
import {
	clearSession,
	issueSession,
	readSession,
	renewSession,
} from "../session.js";

const NO_STORE = { "cache-control": "no-store" };

/** What the browser is told about itself. */
function describe(session) {
	return {
		status: "session",
		account: session.account,
		nick: session.nick,
		channels: session.channels,
		scopes: session.scopes,
	};
}

/** Every page asks this on load, so it is where a session gets slid forward. */
export const onRequestGet = guard(async ({ request, env }) => {
	const session = await readSession(request, env);
	if (!session) return fail(401, "no session");

	const renewed = await renewSession(request, env, session);

	return json(describe(session), {
		headers: renewed ? { ...NO_STORE, "set-cookie": renewed } : NO_STORE,
	});
});

export const onRequestPost = guard(async ({ request, env }) => {
	const blocked = rejectCrossOrigin(request);
	if (blocked) return blocked;

	if (!env.OHAYOU_WEB_SECRET) return fail(503, "sign-in is not configured");

	const { body, error } = await readJson(request);
	if (error) return fail(400, error);

	const grant = await verifyGrant(body.token, env.OHAYOU_WEB_SECRET);
	if (!grant.ok) return refuse(grant.reason);

	if (!(await claim(env, grant.payload))) return refuse("already redeemed");

	const session = {
		account: grant.payload.account,
		nick: grant.payload.nick,
		channels: grant.payload.channels,
		scopes: grant.payload.scopes,
	};
	const cookie = await issueSession(env, session);

	return json(describe(session), {
		headers: { ...NO_STORE, "set-cookie": cookie },
	});
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
 * decides it, so two simultaneous redemptions cannot both win. The expiry is in
 * seconds, as on the wire; everywhere else here is milliseconds.
 */
async function claim(env, payload) {
	const [, claimed] = await env.DB.batch([
		env.DB.prepare("DELETE FROM grant_used WHERE exp < ?1").bind(
			Math.floor(Date.now() / 1000),
		),
		env.DB.prepare(
			"INSERT OR IGNORE INTO grant_used (jti, exp) VALUES (?1, ?2)",
		).bind(payload.id, payload.expiry),
	]);

	return claimed.meta.changes === 1;
}

/** One answer for every reason; the log is where a bad link gets diagnosed. */
function refuse(reason) {
	console.log(`grant refused: ${reason}`);
	return fail(401, "that link is expired, already used, or not valid");
}
