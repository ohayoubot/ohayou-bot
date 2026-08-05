/* The one endpoint that serves a balance and a vault. Mostly who it refuses. */

import assert from "node:assert/strict";
import test from "node:test";
import { SCOPE_DROP, SCOPE_OHAYOU } from "../lib/hmac.js";
import { onRequestGet } from "../lib/ohayou/me.js";

function stored(overrides = {}) {
	return {
		nick: "mallow",
		ohayous: 4200,
		cumulative: 30000,
		items: '{"cat":25}',
		metals: '{"iron":40}',
		equipped: '{"head":"helmet"}',
		defense: 48,
		vault: '{"level":2,"ohayous":9000,"cap":10000}',
		probation: 0,
		fortune: "a fine day",
		running: '[{"kind":"mining","due":1785930353}]',
		...overrides,
	};
}

/** Records what the private query was bound to, so a test can prove it is the
    session's account and nothing else. */
function game(row) {
	const bound = [];
	return {
		bound,
		GAME: {
			prepare: (sql) => ({
				bind: (...params) => {
					bound.push({ sql, params });
					return { first: async () => row };
				},
			}),
		},
	};
}

function session(scopes, account = "Mallow") {
	return {
		DB: {
			prepare: () => ({
				bind: () => ({
					first: async () => ({
						account,
						nick: "mallow",
						channels: '["#chan"]',
						scopes,
					}),
				}),
			}),
		},
	};
}

function request(cookie = "__Host-ohayou=abc") {
	return new Request("https://hemera.day/ohayou/api/me", {
		headers: cookie ? { cookie } : {},
	});
}

async function get(env, req = request()) {
	const response = await onRequestGet({ request: req, env });
	return { response, body: await response.json() };
}

test("your own standing comes back whole", async () => {
	const env = { ...game(stored()), ...session(SCOPE_OHAYOU) };
	const { response, body } = await get(env);

	assert.equal(response.status, 200);
	assert.equal(body.status, "standing");
	assert.equal(body.ohayous, 4200);
	assert.deepEqual(body.vault, { level: 2, ohayous: 9000, cap: 10000 });
	assert.deepEqual(body.running, [{ kind: "mining", due: 1785930353 }]);
	assert.deepEqual(body.metals, { iron: 40 });
});

// Nobody without a session sees a balance.
test("without a session there is nothing", async () => {
	const env = { ...game(stored()), ...session(SCOPE_OHAYOU) };
	const { response, body } = await get(env, request(""));

	assert.equal(response.status, 401);
	assert.equal(JSON.stringify(body).includes("4200"), false);
});

// A drop link must not open a vault.
test("a session without the ohayou scope is refused", async () => {
	const env = { ...game(stored()), ...session(SCOPE_DROP) };
	const { response, body } = await get(env);

	assert.equal(response.status, 401);
	assert.equal(JSON.stringify(body).includes("4200"), false);
});

// No parameter names an account: the query binds the session's.
test("the query can only ask for the session's own account", async () => {
	const env = { ...game(stored()), ...session(SCOPE_OHAYOU, "Mallow") };
	await get(
		env,
		new Request("https://hemera.day/ohayou/api/me?account=Deerly", {
			headers: { cookie: "__Host-ohayou=abc" },
		}),
	);

	const [query] = env.bound;
	assert.deepEqual(query.params, ["Mallow"]);
	assert.equal(query.sql.includes("Deerly"), false);
});

// A player who never named their plot has no private row.
test("an unclaimed plot says so rather than failing", async () => {
	const env = { ...game(null), ...session(SCOPE_OHAYOU) };
	const { response, body } = await get(env);

	assert.equal(response.status, 200);
	assert.equal(body.status, "unclaimed");
});

// A shared cache holding this is the one way it reaches anybody else.
test("a standing is never cached", async () => {
	const env = { ...game(stored()), ...session(SCOPE_OHAYOU) };
	const { response } = await get(env);

	assert.match(response.headers.get("cache-control"), /no-store/);
});

test("a row with unreadable json still answers", async () => {
	const env = {
		...game(stored({ items: "not json", vault: null, running: "{" })),
		...session(SCOPE_OHAYOU),
	};
	const { response, body } = await get(env);

	assert.equal(response.status, 200);
	assert.deepEqual(body.items, {});
	assert.equal(body.vault, null);
	assert.deepEqual(body.running, []);
});

test("without the game database it says so", async () => {
	const { response } = await get({ ...session(SCOPE_OHAYOU) });
	assert.equal(response.status, 503);
});
