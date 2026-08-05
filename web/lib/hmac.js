/*
 * Grant tokens. The irc bot mints one per request and sends it to the user in a
 * private message; lib/drop/session.js redeems it for a cookie.
 *
 *   <payload>.<tag>
 *
 * both base64url without padding. The tag is the first 16 bytes of HMAC-SHA256
 * over the payload's bytes.
 *
 * The payload is packed rather than json because the link is read off a
 * terminal irc client, where a url that wraps is one nobody can click:
 *
 *   0       version
 *   1       scopes, a bitmask
 *   2..5    expiry, unix seconds, big endian
 *   6..13   id, 8 random bytes, redeemable once
 *   14..    account, nick, then a count and that many channels,
 *           each one a length byte followed by its utf-8 bytes
 *
 * The secret is keyed as its own utf-8 bytes. It is written as hex but is not
 * decoded from hex; the bot must key on []byte(secret) to match.
 *
 * This format is implemented twice, here and in the bot's internal/web. Neither
 * side may change without the other. tools/hmac.test.mjs pins it against the
 * same vector as grant_test.go.
 */

const VERSION = 2;

/** Longest a mint may reach into the future. Bounds the damage from a bot with
    a broken clock; the bot itself mints far shorter grants. */
const MAX_TTL = 900;

/** No legitimate token comes close. Bounds work before any parsing. */
const MAX_TOKEN = 1024;

const MAX_NAME = 64;
const MAX_CHANNELS = 32;
const ID_BYTES = 8;
const TAG_BYTES = 16;

/** RFC 1459 caps a channel name at 50 including the prefix. */
const CHANNEL = /^#[^\s,\x07\x00-\x1f]{1,49}$/;

/** What a grant lets its holder do. The bit positions are wire format: the
    bot's internal/web has the same list. */
export const SCOPE_DROP = 1 << 0;
export const SCOPE_OHAYOU = 1 << 1;

const KNOWN_SCOPES = SCOPE_DROP | SCOPE_OHAYOU;

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

async function key(secret) {
	return crypto.subtle.importKey(
		"raw",
		new TextEncoder().encode(secret),
		{ name: "HMAC", hash: "SHA-256" },
		false,
		["sign"],
	);
}

/**
 * The tag is truncated, so subtle.verify is no use: it compares a whole digest.
 * Signing and comparing the first TAG_BYTES ourselves is the same check, and
 * sameBytes keeps it constant time.
 */
async function tag(body, secret) {
	const full = await crypto.subtle.sign("HMAC", await key(secret), body);
	return new Uint8Array(full).subarray(0, TAG_BYTES);
}

/** Constant time for equal lengths, which is the only case that reaches it: a
    tag of another length is refused before the comparison. */
function sameBytes(a, b) {
	if (a.length !== b.length) return false;
	let diff = 0;
	for (let i = 0; i < a.length; i++) diff |= a[i] ^ b[i];
	return diff === 0;
}

function packName(out, name) {
	const raw = new TextEncoder().encode(name);
	if (raw.length === 0 || raw.length > MAX_NAME) {
		throw new Error(`name is ${raw.length} bytes, want 1 to ${MAX_NAME}`);
	}
	out.push(raw.length, ...raw);
}

/** Only the bot mints in production. Kept here so the format has one
    definition and the tests can exercise it. */
export function packGrant({ scopes, expiry, id, account, nick, channels }) {
	const out = [VERSION, scopes & 0xff];
	out.push(
		(expiry >>> 24) & 0xff,
		(expiry >>> 16) & 0xff,
		(expiry >>> 8) & 0xff,
		expiry & 0xff,
	);
	if (id.length !== ID_BYTES) throw new Error(`id is not ${ID_BYTES} bytes`);
	out.push(...id);

	packName(out, account);
	packName(out, nick);
	if (channels.length > MAX_CHANNELS) throw new Error("too many channels");
	out.push(channels.length);
	for (const channel of channels) packName(out, channel);

	return new Uint8Array(out);
}

export async function signGrant(grant, secret) {
	const body = packGrant(grant);
	return `${b64urlEncode(body)}.${b64urlEncode(await tag(body, secret))}`;
}

/**
 * Walks the payload, refusing to read past the end. It runs on bytes whose tag
 * already checked out, so a failure is the bot disagreeing with us rather than
 * anything hostile, but a truncated payload must still not read wild.
 */
function unpackGrant(body) {
	let at = 0;
	const take = (n) => {
		if (at + n > body.length) throw new Error("grant ends early");
		const out = body.subarray(at, at + n);
		at += n;
		return out;
	};
	const byte = () => take(1)[0];
	const name = () => {
		const raw = take(byte());
		if (raw.length === 0) throw new Error("empty name in grant");
		return new TextDecoder("utf-8", { fatal: true }).decode(raw);
	};

	const version = byte();
	if (version !== VERSION) throw new Error(`grant version ${version}`);

	const scopes = byte();
	const stamp = take(4);
	const expiry =
		((stamp[0] << 24) >>> 0) + (stamp[1] << 16) + (stamp[2] << 8) + stamp[3];
	const id = b64urlEncode(take(ID_BYTES));
	const account = name();
	const nick = name();

	const count = byte();
	if (count > MAX_CHANNELS) throw new Error(`${count} channels`);
	const channels = [];
	for (let i = 0; i < count; i++) channels.push(name());

	if (at !== body.length) throw new Error("trailing bytes in grant");
	return { scopes, expiry, id, account, nick, channels };
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
	if (parts.length !== 2) return bad("malformed");

	const body = b64urlDecode(parts[0]);
	const given = b64urlDecode(parts[1]);
	if (body === null || given === null) return bad("malformed");
	if (given.length !== TAG_BYTES) return bad("malformed");

	if (!sameBytes(given, await tag(body, secret))) return bad("bad signature");

	let payload;
	try {
		payload = unpackGrant(body);
	} catch (err) {
		return bad(`unreadable payload: ${err.message}`);
	}

	const invalid = validate(payload);
	if (invalid) return bad(invalid);

	if (payload.expiry <= now) return bad("expired");
	if (payload.expiry > now + MAX_TTL) return bad("expiry too far out");

	return { ok: true, payload };
}

/** Runs on signed input, so this catches a bot bug rather than an attacker. */
function validate(payload) {
	if (payload.scopes === 0) return "no scopes";
	if (payload.scopes & ~KNOWN_SCOPES) return "unknown scope";
	if (payload.channels.length === 0) return "no channels";
	if (!payload.channels.every((c) => CHANNEL.test(c))) return "bad channel";
	return null;
}

/** Whether a grant carries every scope asked of it. */
export function hasScope(payload, wanted) {
	return (payload.scopes & wanted) === wanted;
}

function bad(reason) {
	return { ok: false, reason };
}
