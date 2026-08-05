/* The only way game data reaches the site. Mostly what it refuses. */

import assert from "node:assert/strict";
import test from "node:test";
import { b64urlEncode } from "../lib/hmac.js";
import { onRequestPost, PREFIX } from "../lib/site/ingest.js";

const SECRET = "0123456789abcdef0123456789abcdef";

function now() {
	return Math.floor(Date.now() / 1000);
}

function plot(overrides = {}) {
	return {
		id: "Mallow",
		nick: "mallow",
		named: true,
		flag: "senordeer",
		acres: 6,
		land: { cat: 25, quarry: 1 },
		wealth: "industrialist",
		rations: 120,
		...overrides,
	};
}

function publish(overrides = {}) {
	return {
		plugin: "ohayou",
		table: "plot",
		generation: 1,
		ts: now(),
		rows: [plot()],
		...overrides,
	};
}

async function sign(bytes, secret = SECRET) {
	const key = await crypto.subtle.importKey(
		"raw",
		new TextEncoder().encode(secret),
		{ name: "HMAC", hash: "SHA-256" },
		false,
		["sign"],
	);
	const message = new Uint8Array(PREFIX.length + bytes.length);
	message.set(new TextEncoder().encode(PREFIX), 0);
	message.set(bytes, PREFIX.length);
	const full = new Uint8Array(await crypto.subtle.sign("HMAC", key, message));
	return b64urlEncode(full.subarray(0, 16));
}

/** A D1 binding that records the statements it was given and answers with
    whatever generation the caller planted. */
function db(seen = null) {
	const batches = [];
	const statement = (sql) => ({
		sql,
		params: [],
		bind(...params) {
			return { ...statement(sql), params };
		},
		first: async () => (seen === null ? null : { generation: seen }),
	});
	return {
		batches,
		GAME: {
			prepare: statement,
			batch: async (statements) => {
				batches.push(statements);
				return statements.map(() => ({ meta: { changes: 1 } }));
			},
		},
	};
}

async function post(body, { secret = SECRET, env, signature } = {}) {
	const raw =
		typeof body === "string"
			? new TextEncoder().encode(body)
			: new TextEncoder().encode(JSON.stringify(body));

	const request = new Request("https://hemera.day/api/ingest", {
		method: "POST",
		headers: {
			"x-ingest-signature": signature ?? (await sign(raw, secret)),
			"content-type": "application/json",
		},
		body: raw,
	});

	const context = env ?? { ...db(), UPLOAD_HMAC_SECRET: SECRET };
	const response = await onRequestPost({ request, env: context });
	return { response, body: await response.json(), env: context };
}

test("a signed publish lands", async () => {
	const { response, body, env } = await post(publish());

	assert.equal(response.status, 200);
	assert.deepEqual(body, { status: "published", rows: 1 });

	const [batch] = env.batches;
	assert.match(batch[0].sql, /^DELETE FROM plot$/);
	assert.match(batch[1].sql, /^INSERT INTO plot /);
	assert.match(batch[2].sql, /INSERT INTO publish/);
});

// Stringified here, so the two sides cannot disagree about encoding.
test("json columns are stringified here, not by the bot", async () => {
	const { env } = await post(publish());
	const insert = env.batches[0][1];

	assert.ok(insert.params.includes('{"cat":25,"quarry":1}'));
});

test("an unsigned or wrongly signed publish is refused", async () => {
	for (const options of [
		{ signature: "" },
		{ signature: "not base64" },
		{ signature: b64urlEncode(new Uint8Array(16)) },
		{ secret: "another secret" },
	]) {
		const { response, env } = await post(publish(), options);
		assert.equal(response.status, 400);
		assert.equal(env.batches.length, 0, "it wrote anyway");
	}
});

test("a signature of the wrong length is refused", async () => {
	const { response } = await post(publish(), {
		signature: b64urlEncode(new Uint8Array(32)),
	});
	assert.equal(response.status, 400);
});

// The prefix is what stops a grant being replayed as a publish.
test("a body signed without the domain prefix is refused", async () => {
	const raw = new TextEncoder().encode(JSON.stringify(publish()));
	const key = await crypto.subtle.importKey(
		"raw",
		new TextEncoder().encode(SECRET),
		{ name: "HMAC", hash: "SHA-256" },
		false,
		["sign"],
	);
	const full = new Uint8Array(await crypto.subtle.sign("HMAC", key, raw));

	const { response } = await post(publish(), {
		signature: b64urlEncode(full.subarray(0, 16)),
	});
	assert.equal(response.status, 400);
});

test("a plugin or table nobody taught it is refused", async () => {
	for (const body of [
		publish({ plugin: "deerkins" }),
		publish({ plugin: "" }),
		publish({ table: "session" }),
		publish({ table: "plot; DROP TABLE plot" }),
	]) {
		const { response, env } = await post(body);
		assert.equal(response.status, 400, JSON.stringify(body.table));
		assert.equal(env.batches.length, 0);
	}
});

// A field the bot starts sending must be allowed here before it is stored.
test("a column the table does not have is refused", async () => {
	const { response, env } = await post(
		publish({ rows: [plot({ ohayous: 4200 })] }),
	);
	assert.equal(response.status, 400);
	assert.equal(env.batches.length, 0);
});

test("a row of the wrong shape is refused", async () => {
	for (const row of [
		plot({ id: 7 }),
		plot({ named: 1 }),
		plot({ named: "yes" }),
		plot({ acres: "six" }),
		plot({ acres: 1.5 }),
		plot({ land: undefined }),
		plot({ land: null }),
		{ id: "Mallow" },
		null,
		["Mallow"],
	]) {
		const { response, env } = await post(publish({ rows: [row] }));
		assert.equal(response.status, 400, JSON.stringify(row));
		assert.equal(env.batches.length, 0);
	}
});

// The same row with the identifying half left out.
test("an anonymous plot is accepted", async () => {
	const { response } = await post(
		publish({
			rows: [
				plot({
					id: "1VNgdUZOTZrl",
					nick: "",
					named: false,
					flag: "",
					land: {},
				}),
			],
		}),
	);
	assert.equal(response.status, 200);
});

test("a nullable json column takes null", async () => {
	const { response } = await post(
		publish({
			table: "plot_private",
			rows: [
				{
					account: "Mallow",
					nick: "mallow",
					ohayous: 10,
					cumulative: 20,
					items: {},
					metals: {},
					equipped: {},
					defense: 0,
					vault: null,
					probation: 0,
					fortune: "",
					running: [],
				},
			],
		}),
	);
	assert.equal(response.status, 200);
});

test("a stale generation is not applied", async () => {
	const env = { ...db(5), UPLOAD_HMAC_SECRET: SECRET };
	const { response, body } = await post(publish({ generation: 5 }), { env });

	assert.equal(response.status, 200);
	assert.equal(body.status, "stale");
	assert.equal(env.batches.length, 0, "a replay rewrote the table");
});

test("a newer generation is applied", async () => {
	const env = { ...db(5), UPLOAD_HMAC_SECRET: SECRET };
	const { body } = await post(publish({ generation: 6 }), { env });

	assert.equal(body.status, "published");
});

test("a bad generation or timestamp is refused", async () => {
	for (const body of [
		publish({ generation: 0 }),
		publish({ generation: -1 }),
		publish({ generation: 1.5 }),
		publish({ ts: now() - 600 }),
		publish({ ts: now() + 600 }),
		publish({ ts: "now" }),
	]) {
		const { response } = await post(body);
		assert.equal(
			response.status,
			400,
			JSON.stringify(body.generation ?? body.ts),
		);
	}
});

test("a body that is not an object of rows is refused", async () => {
	for (const body of ["[]", '"publish"', "null", "not json"]) {
		const { response } = await post(body);
		assert.equal(response.status, 400, body);
	}
	const { response } = await post(publish({ rows: "everything" }));
	assert.equal(response.status, 400);
});

test("an empty publish is a publish", async () => {
	// A table nobody consents to appearing in is empty, not unwritten.
	const { response, body, env } = await post(publish({ rows: [] }));

	assert.equal(response.status, 200);
	assert.deepEqual(body, { status: "published", rows: 0 });
	assert.match(env.batches[0][0].sql, /^DELETE FROM plot$/);
});

test("without a secret it publishes nothing", async () => {
	const env = { ...db() };
	const { response } = await post(publish(), { env });

	assert.equal(response.status, 503);
	assert.equal(env.batches.length, 0);
});

test("a plugin with no binding cannot publish", async () => {
	const env = { UPLOAD_HMAC_SECRET: SECRET };
	const { response } = await post(publish(), { env });

	assert.equal(response.status, 400);
});
