/*
 * Sessions for the whole site. A grant is redeemed once (see
 * lib/drop/session.js) and exchanged for one of these.
 *
 * The cookie carries 32 random bytes; the table stores their sha-256, so a dump
 * of session is not a set of live sessions. Unlike clientKey in http.js there
 * is no salt: that hashes low-entropy ips, this hashes a value already past
 * brute force.
 *
 * The scopes are copied from the verified grant, so a cookie only reaches the
 * parts of the site its link was minted for. One session serves every plugin,
 * and each asks for its own scope.
 *
 * Timestamps are milliseconds, matching save_log. The grant's own expiry is in
 * seconds, because that is its wire format.
 */

import { b64urlEncode } from "./hmac.js";

const COOKIE = "__Host-ohayou";

/** A month. Signing in means going back to irc for a link, so make it count. */
const TTL = 30 * 24 * 3600_000;

/** How much of the TTL is spent before a read slides it: a write now and
    again, not one per page load. */
const RENEW_AFTER = TTL / 3;

/**
 * __Host- makes the browser enforce host-only: no Domain, so the cookie is
 * never sent to the bucket's hostname, which is where uploaded bytes are
 * served from. The prefix also requires Path=/, so it rides along on every
 * plugin's pages; the scopes are the boundary, not the path.
 *
 * Secure rules out http, including `wrangler pages dev`. Browsers make an
 * exception for localhost; there is deliberately no env switch to relax this,
 * since a mistake in one would be silent in production.
 */
function cookie(value, maxAge) {
	return `${COOKIE}=${value}; HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=${maxAge}`;
}

async function hash(value) {
	const digest = await crypto.subtle.digest(
		"SHA-256",
		new TextEncoder().encode(value),
	);
	return [...new Uint8Array(digest)]
		.map((b) => b.toString(16).padStart(2, "0"))
		.join("");
}

/** Returns the cookie's value, or null. Exported for the tests. */
export function readCookie(request, name = COOKIE) {
	const header = request.headers.get("cookie");
	if (!header) return null;

	for (const pair of header.split(";")) {
		const at = pair.indexOf("=");
		if (at < 0) continue;
		if (pair.slice(0, at).trim() === name) return pair.slice(at + 1).trim();
	}
	return null;
}

/**
 * Creates a session and returns the Set-Cookie value for it. channels and
 * scopes both come from the verified grant: channels bound where the holder may
 * post, scopes bound what they may do, and the browser can widen neither.
 */
export async function issueSession(env, { account, nick, channels, scopes }) {
	const id = b64urlEncode(crypto.getRandomValues(new Uint8Array(32)));
	const now = Date.now();

	await env.DB.batch([
		env.DB.prepare("DELETE FROM session WHERE expires < ?1").bind(now),
		env.DB.prepare(
			`INSERT INTO session (id_hash, account, nick, channels, scopes, created, expires)
       VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)`,
		).bind(
			await hash(id),
			account,
			nick,
			JSON.stringify(channels),
			scopes,
			now,
			now + TTL,
		),
	]);

	return cookie(id, Math.floor(TTL / 1000));
}

/**
 * Returns {account, nick, channels, scopes, expires} or null. The query decides
 * the expiry, so a session cannot outlive its row by way of a clock read here;
 * expires comes back for renewSession to judge.
 */
export async function readSession(request, env) {
	const id = readCookie(request);
	if (!id) return null;

	const row = await env.DB.prepare(
		"SELECT account, nick, channels, scopes, expires FROM session WHERE id_hash = ?1 AND expires > ?2",
	)
		.bind(await hash(id), Date.now())
		.first();

	if (!row) return null;

	try {
		return {
			account: row.account,
			nick: row.nick,
			channels: JSON.parse(row.channels),
			scopes: row.scopes,
			expires: row.expires,
		};
	} catch {
		return null;
	}
}

/**
 * Slides a session's expiry and returns a Set-Cookie sliding the browser's copy
 * with it, or null when it is too young to bother. Both halves have to move:
 * the row says the session is alive, the cookie says the browser still sends
 * it. The UPDATE re-checks the expiry rather than trusting the read it was
 * handed, so one that ran out in between is not resurrected.
 */
export async function renewSession(request, env, session) {
	if (!session || session.expires - Date.now() > TTL - RENEW_AFTER) return null;

	const id = readCookie(request);
	if (!id) return null;

	const now = Date.now();
	const { meta } = await env.DB.prepare(
		"UPDATE session SET expires = ?1 WHERE id_hash = ?2 AND expires > ?3",
	)
		.bind(now + TTL, await hash(id), now)
		.run();

	return meta.changes === 1 ? cookie(id, Math.floor(TTL / 1000)) : null;
}

/**
 * Returns the session when it carries every scope asked of it, otherwise null.
 * A handler calling this cannot be reached by a cookie minted for another part
 * of the site.
 */
export async function requireScope(request, env, wanted) {
	const session = await readSession(request, env);
	if (!session) return null;
	return (session.scopes & wanted) === wanted ? session : null;
}

/** Drops the row and returns a Set-Cookie that expires the browser's copy. */
export async function clearSession(request, env) {
	const id = readCookie(request);
	if (id) {
		await env.DB.prepare("DELETE FROM session WHERE id_hash = ?1")
			.bind(await hash(id))
			.run();
	}
	return cookie("", 0);
}
