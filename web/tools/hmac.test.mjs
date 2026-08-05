import assert from "node:assert/strict";
import test from "node:test";
import {
	b64urlDecode,
	b64urlEncode,
	signGrant,
	verifyGrant,
} from "../lib/hmac.js";

const SECRET = "0123456789abcdef0123456789abcdef";
const NOW = 1_754_250_000;

function grant(overrides = {}) {
	return {
		a: "someone",
		n: "someone_",
		c: ["#chan", "#other"],
		e: NOW + 300,
		j: "MDEyMzQ1Njc4OWFiY2RlZg",
		...overrides,
	};
}

/** Swaps in a payload without touching the signature. */
function repayload(token, payload) {
	const [version, , signature] = token.split(".");
	const encoded = b64urlEncode(
		new TextEncoder().encode(JSON.stringify(payload)),
	);
	return `${version}.${encoded}.${signature}`;
}

async function reason(token, secret = SECRET, now = NOW) {
	const result = await verifyGrant(token, secret, now);
	assert.equal(result.ok, false);
	return result.reason;
}

test("a freshly signed grant verifies", async () => {
	const payload = grant();
	const result = await verifyGrant(
		await signGrant(payload, SECRET),
		SECRET,
		NOW,
	);
	assert.equal(result.ok, true);
	assert.deepEqual(result.payload, payload);
});

test("the byte layout is pinned", async () => {
	// The bot signs the same bytes in go. If this changes, both sides change.
	assert.equal(
		await signGrant(grant(), SECRET),
		"v1.eyJhIjoic29tZW9uZSIsIm4iOiJzb21lb25lXyIsImMiOlsiI2NoYW4iLCIjb3RoZXIiXSwiZSI6MTc1NDI1MDMwMCwiaiI6Ik1ERXlNelExTmpjNE9XRmlZMlJsWmcifQ.JCgzlwyJ-xGgERsMw9DEWv91owB4GLvuEUgoGUB8Wpc",
	);
});

test("field order is not part of the contract", async () => {
	// Verification hashes the payload as received, so a bot that emits the
	// fields in another order still interoperates. Only the vector above cares.
	const reordered = {
		j: "x".repeat(8),
		e: NOW + 300,
		c: ["#chan"],
		n: "n",
		a: "a",
	};
	const result = await verifyGrant(
		await signGrant(reordered, SECRET),
		SECRET,
		NOW,
	);
	assert.equal(result.ok, true);
});

test("a grant signed with another secret is refused", async () => {
	const token = await signGrant(grant(), "not the secret");
	assert.equal(await reason(token), "bad signature");
});

test("swapping the payload under a valid signature is refused", async () => {
	const token = await signGrant(grant(), SECRET);
	const widened = repayload(
		token,
		grant({ c: ["#chan", "#other", "#secret"] }),
	);
	assert.equal(await reason(widened), "bad signature");
});

test("a truncated signature is refused", async () => {
	const token = await signGrant(grant(), SECRET);
	assert.equal(await reason(token.slice(0, -1)), "bad signature");
});

test("an expired grant is refused", async () => {
	const token = await signGrant(grant({ e: NOW - 1 }), SECRET);
	assert.equal(await reason(token), "expired");
});

test("a grant expiring exactly now is refused", async () => {
	const token = await signGrant(grant({ e: NOW }), SECRET);
	assert.equal(await reason(token), "expired");
});

test("a grant reaching too far into the future is refused", async () => {
	const token = await signGrant(grant({ e: NOW + 901 }), SECRET);
	assert.equal(await reason(token), "expiry too far out");
});

test("malformed tokens are refused before any parsing", async () => {
	const token = await signGrant(grant(), SECRET);
	const [version, encoded, signature] = token.split(".");

	assert.equal(await reason(""), "malformed");
	assert.equal(await reason(`${version}.${encoded}`), "malformed");
	assert.equal(await reason(`${token}.extra`), "malformed");
	assert.equal(await reason(null), "malformed");
	assert.equal(await reason("v1.".padEnd(2000, "a")), "malformed");
	assert.equal(await reason(`v2.${encoded}.${signature}`), "bad version");
	assert.equal(await reason(`${version}.${encoded}.not base64`), "malformed");
});

test("verifying without a secret is refused", async () => {
	const token = await signGrant(grant(), SECRET);
	assert.equal(await reason(token, ""), "no secret");
});

test("a signed but nonsensical payload is refused", async () => {
	const cases = [
		[{ ...grant(), a: "" }, "bad account"],
		[{ ...grant(), a: "x".repeat(65) }, "bad account"],
		[{ ...grant(), n: 7 }, "bad nick"],
		[{ ...grant(), j: undefined }, "bad id"],
		[{ ...grant(), e: 1.5 }, "bad expiry"],
		[{ ...grant(), c: [] }, "no channels"],
		[{ ...grant(), c: "#chan" }, "no channels"],
		[{ ...grant(), c: Array(33).fill("#chan") }, "too many channels"],
		[{ ...grant(), c: ["chan"] }, "bad channel"],
		[{ ...grant(), c: ["#with space"] }, "bad channel"],
		[{ ...grant(), c: ["#with,comma"] }, "bad channel"],
		[{ ...grant(), c: [`#${"x".repeat(50)}`] }, "bad channel"],
		[{ ...grant(), c: [17] }, "bad channel"],
	];

	for (const [payload, expected] of cases) {
		const token = await signGrant(payload, SECRET);
		assert.equal(await reason(token), expected, JSON.stringify(payload));
	}
});

test("a signed non-object payload is refused", async () => {
	for (const payload of [null, ["#chan"], 42, "grant"]) {
		const token = await signGrant(payload, SECRET);
		assert.equal(await reason(token), "payload is not an object");
	}
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
