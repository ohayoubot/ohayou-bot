/*
 * Upload sessions. A grant is redeemed once (see functions/drop/api/session.js)
 * and exchanged for one of these.
 *
 * The cookie carries 32 random bytes; the table stores their sha-256, so a dump
 * of upload_session is not a set of live sessions. Unlike clientKey in http.js
 * there is no salt: that hashes low-entropy ips, this hashes a value already
 * past brute force.
 *
 * Timestamps are milliseconds, matching save_log. The grant's own expiry is in
 * seconds, because that is its wire format.
 */

import { b64urlEncode } from "./hmac.js";

const COOKIE = "__Host-drop";

const TTL = 12 * 3600_000;

/**
 * __Host- makes the browser enforce host-only: no Domain, so the cookie can
 * never be sent to the bucket's hostname, which is the one place uploaded bytes
 * are served from. The prefix also requires Path=/, so it rides along on
 * gallery requests too. Nothing else on the origin reads it, and Path was never
 * a security boundary.
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
 * Creates a session and returns the Set-Cookie value for it. channels comes
 * from the verified grant and bounds where the uploader may post.
 */
export async function issueSession(env, { account, nick, channels }) {
	const id = b64urlEncode(crypto.getRandomValues(new Uint8Array(32)));
	const now = Date.now();

	await env.DB.batch([
		env.DB.prepare("DELETE FROM upload_session WHERE expires < ?1").bind(now),
		env.DB.prepare(
			`INSERT INTO upload_session (id_hash, account, nick, channels, created, expires)
       VALUES (?1, ?2, ?3, ?4, ?5, ?6)`,
		).bind(
			await hash(id),
			account,
			nick,
			JSON.stringify(channels),
			now,
			now + TTL,
		),
	]);

	return cookie(id, Math.floor(TTL / 1000));
}

/**
 * Returns {account, nick, channels} or null. Expiry is decided by the query,
 * so a session cannot outlive its row by way of a clock read here.
 */
export async function readSession(request, env) {
	const id = readCookie(request);
	if (!id) return null;

	const row = await env.DB.prepare(
		"SELECT account, nick, channels FROM upload_session WHERE id_hash = ?1 AND expires > ?2",
	)
		.bind(await hash(id), Date.now())
		.first();

	if (!row) return null;

	try {
		return {
			account: row.account,
			nick: row.nick,
			channels: JSON.parse(row.channels),
		};
	} catch {
		return null;
	}
}

/** Drops the row and returns a Set-Cookie that expires the browser's copy. */
export async function clearSession(request, env) {
	const id = readCookie(request);
	if (id) {
		await env.DB.prepare("DELETE FROM upload_session WHERE id_hash = ?1")
			.bind(await hash(id))
			.run();
	}
	return cookie("", 0);
}
