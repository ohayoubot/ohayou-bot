/* Nothing unasked-for reaches the queue, and nobody without a session reaches
   it at all. */

import assert from "node:assert/strict";
import test from "node:test";
import { SCOPE_DROP, SCOPE_OHAYOU } from "../lib/hmac.js";
import { onRequestPost } from "../lib/ohayou/command.js";

/** A GAME binding recording what was written, with a claim that succeeds
    unless the test says otherwise. */
function game({ claimed = true } = {}) {
	const writes = [];
	const statement = (sql) => ({
		sql,
		bind: (...params) => ({
			sql,
			params,
			run: async () => {
				writes.push({ sql, params });
				return { meta: { changes: 1 } };
			},
		}),
	});
	return {
		writes,
		GAME: {
			prepare: statement,
			batch: async () => [{}, { meta: { changes: claimed ? 1 : 0 } }],
		},
	};
}

function session(scopes, account = "Mallow") {
	return {
		DB: {
			prepare: () => ({
				bind: () => ({
					first: async () =>
						scopes === null
							? null
							: { account, nick: "mallow", channels: '["#chan"]', scopes },
				}),
			}),
		},
	};
}

async function post(body, { scopes = SCOPE_OHAYOU, env, account } = {}) {
	const context = env ?? { ...game(), ...session(scopes, account) };
	const request = new Request("https://hemera.day/ohayou/api/command", {
		method: "POST",
		headers: {
			"content-type": "application/json",
			origin: "https://hemera.day",
			cookie: "__Host-ohayou=abc",
		},
		body: JSON.stringify(body),
	});
	const response = await onRequestPost({ request, env: context });
	return { response, body: await response.json(), env: context };
}

test("a flag is queued", async () => {
	const { response, body, env } = await post({
		kind: "flag",
		value: "senordeer",
	});

	assert.equal(response.status, 200);
	assert.equal(body.status, "queued");

	const [write] = env.writes;
	assert.match(write.sql, /INSERT INTO command /);
	// The account comes from the session, never from the body.
	assert.deepEqual(write.params.slice(1), ["Mallow", "flag", "senordeer"]);
});

test("visibility is queued", async () => {
	const { response, env } = await post({ kind: "territory", value: "on" });

	assert.equal(response.status, 200);
	assert.deepEqual(env.writes[0].params.slice(1), [
		"Mallow",
		"territory",
		"on",
	]);
});

// A body claiming another account must not be believed.
test("a request cannot be made on somebody else's behalf", async () => {
	const { env } = await post({
		kind: "flag",
		value: "senordeer",
		account: "Deerly",
	});

	assert.equal(env.writes[0].params[1], "Mallow");
});

test("without a session nothing is queued", async () => {
	const env = { ...game(), ...session(null) };
	const { response } = await post({ kind: "flag", value: "x" }, { env });

	assert.equal(response.status, 401);
	assert.equal(env.writes.length, 0);
});

// A drop link must not change somebody's territory.
test("a session without the ohayou scope is refused", async () => {
	const { response, env } = await post(
		{ kind: "flag", value: "x" },
		{ scopes: SCOPE_DROP },
	);

	assert.equal(response.status, 401);
	assert.equal(env.writes.length, 0);
});

// Nothing that spends or earns is on the list.
test("a kind nobody taught it is refused", async () => {
	for (const body of [
		{ kind: "buy", value: "cat" },
		{ kind: "steal", value: "deerly" },
		{ kind: "ohayous", value: "1000" },
		{ kind: "", value: "" },
		{ value: "no kind" },
	]) {
		const { response, env } = await post(body);
		assert.equal(response.status, 400, JSON.stringify(body));
		assert.equal(env.writes.length, 0);
	}
});

test("a value the kind does not take is refused", async () => {
	for (const body of [
		{ kind: "territory", value: "maybe" },
		{ kind: "territory", value: true },
		{ kind: "flag", value: 7 },
		{ kind: "flag", value: "x".repeat(49) },
	]) {
		const { response, env } = await post(body);
		assert.equal(response.status, 400, JSON.stringify(body));
		assert.equal(env.writes.length, 0);
	}
});

// Taking a flag down is an empty value, not a missing one.
test("an empty flag is a valid request", async () => {
	const { response } = await post({ kind: "flag", value: "" });
	assert.equal(response.status, 200);
});

test("asking too often is refused before anything is queued", async () => {
	const env = { ...game({ claimed: false }), ...session(SCOPE_OHAYOU) };
	const { response } = await post({ kind: "flag", value: "x" }, { env });

	assert.equal(response.status, 429);
	assert.equal(env.writes.length, 0);
});

test("without the game database nothing is queued", async () => {
	const { response } = await post(
		{ kind: "flag", value: "x" },
		{ env: { ...session(SCOPE_OHAYOU) } },
	);
	assert.equal(response.status, 503);
});
