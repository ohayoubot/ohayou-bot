/*
 * POST /drop/api/upload?channel=%23chan
 *
 * Body is the raw image. Requires the cookie from api/session and the
 * x-drop-upload header. Stores the bytes in r2 and queues one line for the bot
 * to say in the channel.
 */

import { b64urlEncode } from "../../../lib/hmac.js";
import {
	fail,
	guard,
	intVar,
	json,
	rejectForeignOrigin,
} from "../../../lib/http.js";
import { readSession } from "../../../lib/session.js";
import { sniffImage } from "../../../lib/sniff.js";

/** The page re-encodes through a canvas before sending, so anything near this
    is not something the page produced. */
const MAX_BYTES = 5 * 1024 * 1024;

const HOUR = 3600_000;
const DAY = 24 * HOUR;

/** 12 random bytes, 16 base64url characters. Enough that a collision is not
    worth handling in code; upload.key is UNIQUE so one would surface as an
    error rather than as an object quietly overwritten. */
const KEY_BYTES = 12;

export const onRequestPost = guard(async ({ request, env }) => {
	const blocked = rejectForeignOrigin(request);
	if (blocked) return blocked;

	/*
	 * This header is the csrf control. It is not CORS-safelisted, so a browser
	 * must preflight to send it, and no OPTIONS handler exists to answer.
	 * Content-Type is deliberately not checked: it is attacker-controlled and
	 * ignored, and requiring multipart or a form encoding here would put the
	 * endpoint back inside what a cross-origin form can reach.
	 */
	if (request.headers.get("x-drop-upload") !== "1")
		return fail(400, "missing x-drop-upload header");

	if (!env.UPLOADS || !env.PUBLIC_IMAGE_BASE)
		return fail(503, "uploads are not configured");

	const session = await readSession(request, env);
	if (!session) return fail(401, "no session, ask the bot for a new link");

	const channel = pick(
		session.channels,
		new URL(request.url).searchParams.get("channel"),
	);
	if (!channel) return fail(403, "not a channel you may post to");

	// Requiring the length keeps an unbounded body from being read at all. The
	// bytes are measured again below, since this header is the client's claim.
	const declared = Number.parseInt(
		request.headers.get("content-length") ?? "",
		10,
	);
	if (!Number.isFinite(declared)) return fail(411, "send a content-length");
	if (declared > MAX_BYTES) return fail(413, "that image is too big");

	const bytes = new Uint8Array(await request.arrayBuffer());
	if (bytes.length === 0) return fail(400, "empty body");
	if (bytes.length > MAX_BYTES) return fail(413, "that image is too big");

	const kind = sniffImage(bytes);
	if (!kind) return fail(415, "png, jpeg, gif and webp only");

	const rejection = await reserveUpload(env, session.account);
	if (rejection) return rejection;

	const key = `${b64urlEncode(crypto.getRandomValues(new Uint8Array(KEY_BYTES)))}.${kind.ext}`;

	/*
	 * r2 before d1: a failure between the two leaves an object nobody links to,
	 * which costs nothing, rather than a queued line pointing at a 404.
	 *
	 * contentType is the sniffed type, never the client's. The bucket's hostname
	 * still needs a rule adding nosniff; see the readme.
	 */
	await env.UPLOADS.put(key, bytes, {
		httpMetadata: {
			contentType: kind.mime,
			contentDisposition: "inline",
			cacheControl: "public, max-age=31536000, immutable",
		},
	});

	await env.DB.prepare(
		`INSERT INTO upload (ts, account, nick, channel, key, mime, bytes)
     VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)`,
	)
		.bind(
			Date.now(),
			session.account,
			session.nick,
			channel,
			key,
			kind.mime,
			bytes.length,
		)
		.run();

	return json(
		{ status: "queued", channel, url: url(env, key) },
		{ status: 201, headers: { "cache-control": "no-store" } },
	);
});

/**
 * Returns the channel as the bot signed it, not as the client spelled it, so
 * only names the bot vouched for are ever stored. Irc compares channel names
 * without case.
 */
function pick(channels, wanted) {
	if (typeof wanted !== "string") return null;
	return channels.find((c) => c.toLowerCase() === wanted.toLowerCase()) ?? null;
}

function url(env, key) {
	return `${env.PUBLIC_IMAGE_BASE.replace(/\/+$/, "")}/${key}`;
}

/**
 * Claims one upload against the per-account and site-wide hourly limits. Same
 * shape as reserveSlot in the gallery: the INSERT's WHERE does the counting, so
 * a burst cannot all read the same count and all pass. Keyed on the account
 * rather than an ip, since a grant already said who this is.
 */
async function reserveUpload(env, account) {
	const perAccount = intVar(env.UPLOADS_PER_HOUR, 20);
	const siteWide = intVar(env.GLOBAL_UPLOADS_PER_HOUR, 200);
	const now = Date.now();
	const since = now - HOUR;

	const [, claim] = await env.DB.batch([
		env.DB.prepare("DELETE FROM upload_log WHERE ts < ?1").bind(now - DAY),
		env.DB.prepare(
			`INSERT INTO upload_log (account, ts)
       SELECT ?1, ?2
       WHERE (SELECT COUNT(*) FROM upload_log WHERE account = ?1 AND ts > ?3) < ?4
         AND (SELECT COUNT(*) FROM upload_log WHERE ts > ?3) < ?5`,
		).bind(account, now, since, perAccount, siteWide),
	]);

	if (claim.meta.changes === 1) return null;

	const mine = await env.DB.prepare(
		"SELECT COUNT(*) AS n FROM upload_log WHERE account = ?1 AND ts > ?2",
	)
		.bind(account, since)
		.first();

	if ((mine?.n ?? 0) >= perAccount) {
		return json(
			{
				status: "error",
				error: `you have uploaded ${perAccount} images in the last hour, try again later`,
			},
			{
				status: 429,
				headers: { "retry-after": "900", "cache-control": "no-store" },
			},
		);
	}

	return json(
		{
			status: "error",
			error: "uploads are busy right now, try again in a bit",
		},
		{
			status: 429,
			headers: { "retry-after": "600", "cache-control": "no-store" },
		},
	);
}
