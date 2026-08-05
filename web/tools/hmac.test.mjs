import assert from "node:assert/strict";
import test from "node:test";
import {
	b64urlDecode,
	b64urlEncode,
	hasScope,
	packGrant,
	SCOPE_DROP,
	SCOPE_OHAYOU,
	signGrant,
	verifyGrant,
} from "../lib/hmac.js";

const SECRET = "0123456789abcdef0123456789abcdef";
const NOW = 1_754_250_000;

/** The id is 8 raw bytes on the wire; verifyGrant hands it back base64url. */
const ID = new TextEncoder().encode("01234567");
const ID_B64 = b64urlEncode(ID);

function grant(overrides = {}) {
	return {
		scopes: SCOPE_DROP,
		expiry: NOW + 300,
		id: ID,
		account: "someone",
		nick: "someone_",
		channels: ["#chan", "#other"],
		...overrides,
	};
}

/** Re-signs a payload the packer would refuse, so validate() can be reached. */
async function signRaw(bytes, secret = SECRET) {
	const key = await crypto.subtle.importKey(
		"raw",
		new TextEncoder().encode(secret),
		{ name: "HMAC", hash: "SHA-256" },
		false,
		["sign"],
	);
	const full = new Uint8Array(await crypto.subtle.sign("HMAC", key, bytes));
	return `${b64urlEncode(bytes)}.${b64urlEncode(full.subarray(0, 16))}`;
}

async function reason(token, secret = SECRET, now = NOW) {
	const result = await verifyGrant(token, secret, now);
	assert.equal(result.ok, false);
	return result.reason;
}

test("a freshly signed grant verifies", async () => {
	const result = await verifyGrant(
		await signGrant(grant(), SECRET),
		SECRET,
		NOW,
	);
	assert.equal(result.ok, true);
	assert.deepEqual(result.payload, {
		scopes: SCOPE_DROP,
		expiry: NOW + 300,
		id: ID_B64,
		account: "someone",
		nick: "someone_",
		channels: ["#chan", "#other"],
	});
});

test("the byte layout is pinned", async () => {
	// The bot signs the same bytes in go, and internal/web/grant_test.go pins
	// this exact string. If it changes, both sides change together.
	assert.equal(
		await signGrant(grant(), SECRET),
		"AgFoj7w8MDEyMzQ1NjcHc29tZW9uZQhzb21lb25lXwIFI2NoYW4GI290aGVy.zx4o3X9YTT1z-DbcIay-qw",
	);
});

test("the token is short enough to click out of a terminal", async () => {
	// A url that wraps in an irc client is one nobody can follow.
	const token = await signGrant(grant(), SECRET);
	assert.ok(token.length <= 96, `${token.length} characters`);
});

test("a grant signed with another secret is refused", async () => {
	const token = await signGrant(grant(), "not the secret");
	assert.equal(await reason(token), "bad signature");
});

test("swapping the payload under a valid tag is refused", async () => {
	const token = await signGrant(grant(), SECRET);
	const [, tag] = token.split(".");
	const widened = packGrant(
		grant({ channels: ["#chan", "#other", "#secret"] }),
	);
	assert.equal(
		await reason(`${b64urlEncode(widened)}.${tag}`),
		"bad signature",
	);
});

test("a truncated tag is refused", async () => {
	const token = await signGrant(grant(), SECRET);
	assert.equal(await reason(token.slice(0, -1)), "malformed");
});

test("an expired grant is refused", async () => {
	assert.equal(
		await reason(await signGrant(grant({ expiry: NOW - 1 }), SECRET)),
		"expired",
	);
});

test("a grant expiring exactly now is refused", async () => {
	assert.equal(
		await reason(await signGrant(grant({ expiry: NOW }), SECRET)),
		"expired",
	);
});

test("a grant reaching too far into the future is refused", async () => {
	assert.equal(
		await reason(await signGrant(grant({ expiry: NOW + 901 }), SECRET)),
		"expiry too far out",
	);
});

test("malformed tokens are refused before any parsing", async () => {
	const token = await signGrant(grant(), SECRET);
	const [body] = token.split(".");

	assert.equal(await reason(""), "malformed");
	assert.equal(await reason(body), "malformed");
	assert.equal(await reason(`${token}.extra`), "malformed");
	assert.equal(await reason(null), "malformed");
	assert.equal(await reason("".padEnd(2000, "a")), "malformed");
	assert.equal(await reason(`${body}.not base64`), "malformed");
});

test("a grant from another version is refused", async () => {
	const body = packGrant(grant());
	body[0] = 3;
	assert.equal(
		await reason(await signRaw(body)),
		"unreadable payload: grant version 3",
	);
});

test("a truncated or padded payload is refused", async () => {
	const body = packGrant(grant());

	for (let n = 0; n < body.length; n++) {
		const short = await signRaw(body.subarray(0, n));
		const why = await reason(short);
		assert.ok(why.startsWith("unreadable payload"), `${n} bytes gave ${why}`);
	}

	const padded = new Uint8Array([...body, 0]);
	assert.equal(
		await reason(await signRaw(padded)),
		"unreadable payload: trailing bytes in grant",
	);
});

test("verifying without a secret is refused", async () => {
	const token = await signGrant(grant(), SECRET);
	assert.equal(await reason(token, ""), "no secret");
});

test("a signed but nonsensical payload is refused", async () => {
	const cases = [
		[grant({ scopes: 0 }), "no scopes"],
		[grant({ scopes: 0x80 }), "unknown scope"],
		[grant({ channels: [] }), "no channels"],
		[grant({ channels: ["chan"] }), "bad channel"],
		[grant({ channels: ["#with space"] }), "bad channel"],
		[grant({ channels: ["#with,comma"] }), "bad channel"],
		[grant({ channels: [`#${"x".repeat(50)}`] }), "bad channel"],
	];

	for (const [payload, expected] of cases) {
		const token = await signRaw(packGrant(payload));
		assert.equal(
			await reason(token),
			expected,
			JSON.stringify(payload.channels),
		);
	}
});

test("the packer refuses names it cannot length-prefix", () => {
	assert.throws(() => packGrant(grant({ account: "" })));
	assert.throws(() => packGrant(grant({ account: "x".repeat(65) })));
	assert.throws(() => packGrant(grant({ channels: Array(33).fill("#chan") })));
	assert.throws(() => packGrant(grant({ id: new Uint8Array(4) })));
});

test("scopes are checked as a set, not a value", async () => {
	const both = await verifyGrant(
		await signGrant(grant({ scopes: SCOPE_DROP | SCOPE_OHAYOU }), SECRET),
		SECRET,
		NOW,
	);
	assert.equal(both.ok, true);
	assert.equal(hasScope(both.payload, SCOPE_DROP), true);
	assert.equal(hasScope(both.payload, SCOPE_OHAYOU), true);
	assert.equal(hasScope(both.payload, SCOPE_DROP | SCOPE_OHAYOU), true);

	const dropOnly = await verifyGrant(
		await signGrant(grant(), SECRET),
		SECRET,
		NOW,
	);
	assert.equal(hasScope(dropOnly.payload, SCOPE_OHAYOU), false);
	assert.equal(hasScope(dropOnly.payload, SCOPE_DROP | SCOPE_OHAYOU), false);
});

test("a multibyte name survives the round trip", async () => {
	const result = await verifyGrant(
		await signGrant(grant({ nick: "ｄｅｅｒ" }), SECRET),
		SECRET,
		NOW,
	);
	assert.equal(result.ok, true);
	assert.equal(result.payload.nick, "ｄｅｅｒ");
});

test("base64url round-trips and rejects the standard alphabet", () => {
	const bytes = new Uint8Array([0, 1, 62, 63, 127, 128, 254, 255]);
	assert.deepEqual(b64urlDecode(b64urlEncode(bytes)), bytes);
	assert.equal(b64urlEncode(bytes).includes("="), false);

	assert.equal(b64urlDecode("a+b/c"), null);
	assert.equal(b64urlDecode("YQ=="), null);
	assert.equal(b64urlDecode("!"), null);
	assert.equal(b64urlDecode(undefined), null);
	assert.deepEqual(b64urlDecode(""), new Uint8Array(0));
});
