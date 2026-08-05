/*
 * Grant tokens. The irc bot mints one per upload request and sends it to the
 * user in a private message; api/drop/session redeems it for a cookie.
 *
 *   v1.<payload>.<signature>
 *
 * payload    base64url of the json below
 * signature  base64url of HMAC-SHA256 over the literal "v1.<payload>"
 *
 *   a  services account name, the identity everything else keys on
 *   n  nick at mint time, display only
 *   c  channels the upload may be posted to
 *   e  expiry, unix seconds
 *   j  unique id, redeemable once
 *
 * The secret is keyed as its own utf-8 bytes. It is written as hex but is not
 * decoded from hex; the bot must key on []byte(secret) to match.
 *
 * This format is implemented twice, here and in the bot's go. Neither side may
 * change without the other. tools/hmac.test.mjs pins it.
 */

const VERSION = "v1";

/** Longest a mint may reach into the future. Bounds the damage from a bot with
    a broken clock; the bot itself mints far shorter grants. */
const MAX_TTL = 900;

/** No legitimate token comes close. Bounds work before any parsing. */
const MAX_TOKEN = 1024;

const MAX_NAME = 64;
const MAX_CHANNELS = 32;

/** RFC 1459 caps a channel name at 50 including the prefix. */
const CHANNEL = /^#[^\s,\x07\x00-\x1f]{1,49}$/;

export function b64urlEncode(bytes) {
	let binary = "";
	for (const byte of bytes) binary += String.fromCharCode(byte);
	return btoa(binary)
		.replace(/\+/g, "-")
		.replace(/\//g, "_")
		.replace(/=+$/, "");
}

/** Returns null rather than throwing, and rejects the standard alphabet's
    "+" and "/" so one encoding maps to one token. */
export function b64urlDecode(text) {
	if (typeof text !== "string" || !/^[A-Za-z0-9_-]*$/.test(text)) return null;
	try {
		const binary = atob(text.replace(/-/g, "+").replace(/_/g, "/"));
		const bytes = new Uint8Array(binary.length);
		for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
		return bytes;
	} catch {
		return null;
	}
}

async function key(secret, usages) {
	return crypto.subtle.importKey(
		"raw",
		new TextEncoder().encode(secret),
		{ name: "HMAC", hash: "SHA-256" },
		false,
		usages,
	);
}

/** Only the bot mints in production. Kept here so the format has one
    definition and the tests can exercise it. */
export async function signGrant(payload, secret) {
	const signed = `${VERSION}.${b64urlEncode(
		new TextEncoder().encode(JSON.stringify(payload)),
	)}`;
	const signature = await crypto.subtle.sign(
		"HMAC",
		await key(secret, ["sign"]),
		new TextEncoder().encode(signed),
	);
	return `${signed}.${b64urlEncode(new Uint8Array(signature))}`;
}

/**
 * Returns {ok: true, payload} or {ok: false, reason}. The reason is for the
 * log; callers answer every failure the same way, so a probe cannot tell a
 * spent grant from a forged one.
 */
export async function verifyGrant(
	token,
	secret,
	now = Math.floor(Date.now() / 1000),
) {
	if (typeof token !== "string" || token.length > MAX_TOKEN)
		return bad("malformed");
	if (typeof secret !== "string" || secret === "") return bad("no secret");

	const parts = token.split(".");
	if (parts.length !== 3) return bad("malformed");

	const [version, encoded, signature] = parts;
	if (version !== VERSION) return bad("bad version");

	const raw = b64urlDecode(signature);
	if (raw === null) return bad("malformed");

	// subtle.verify compares the digests itself, in constant time.
	const valid = await crypto.subtle.verify(
		"HMAC",
		await key(secret, ["verify"]),
		raw,
		new TextEncoder().encode(`${version}.${encoded}`),
	);
	if (!valid) return bad("bad signature");

	const decoded = b64urlDecode(encoded);
	if (decoded === null) return bad("malformed");

	let payload;
	try {
		payload = JSON.parse(new TextDecoder().decode(decoded));
	} catch {
		return bad("unreadable payload");
	}

	const invalid = validate(payload);
	if (invalid) return bad(invalid);

	if (payload.e <= now) return bad("expired");
	if (payload.e > now + MAX_TTL) return bad("expiry too far out");

	return { ok: true, payload };
}

/** Runs on signed input, so this catches a bot bug rather than an attacker. */
function validate(payload) {
	if (payload === null || typeof payload !== "object" || Array.isArray(payload))
		return "payload is not an object";
	if (!name(payload.a)) return "bad account";
	if (!name(payload.n)) return "bad nick";
	if (!name(payload.j)) return "bad id";
	if (!Number.isSafeInteger(payload.e)) return "bad expiry";
	if (!Array.isArray(payload.c) || payload.c.length === 0) return "no channels";
	if (payload.c.length > MAX_CHANNELS) return "too many channels";
	if (!payload.c.every((c) => typeof c === "string" && CHANNEL.test(c)))
		return "bad channel";
	return null;
}

function name(value) {
	return (
		typeof value === "string" && value.length > 0 && value.length <= MAX_NAME
	);
}

function bad(reason) {
	return { ok: false, reason };
}
