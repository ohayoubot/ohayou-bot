import assert from "node:assert/strict";
import test from "node:test";
import { SCOPE_DROP, SCOPE_OHAYOU } from "../lib/hmac.js";
import {
	clearSession,
	issueSession,
	readCookie,
	readSession,
	renewSession,
	requireScope,
} from "../lib/session.js";

const COOKIE = "__Host-ohayou";

const TTL = 30 * 24 * 3600_000;

function request(cookie) {
	return new Request("https://hemera.day/drop/", {
		headers: cookie ? { cookie } : {},
	});
}

/** Enough of the D1 binding for issue, read, renew and clear: records what it
    was asked to run, and answers with whatever row the caller planted. */
function db(row = null, changes = 1) {
	const calls = [];
	const statement = (sql) => ({
		bind: (...params) => {
			calls.push({ sql, params });
			return statement(sql);
		},
		run: async () => ({ meta: { changes } }),
		first: async () => row,
	});
	return {
		calls,
		DB: { prepare: statement, batch: async () => [] },
	};
}

/** A stored row as readSession will find it, fresh out of the box. */
function stored(overrides = {}) {
	return {
		account: "someone",
		nick: "someone_",
		channels: JSON.stringify(["#chan"]),
		scopes: SCOPE_DROP,
		expires: Date.now() + TTL,
		...overrides,
	};
}

function attributes(header) {
	return new Set(header.split(";").map((part) => part.trim()));
}

test("the cookie is read out of a crowded header", () => {
	assert.equal(readCookie(request(`${COOKIE}=abc`)), "abc");
	assert.equal(readCookie(request(`other=1; ${COOKIE}=abc; last=2`)), "abc");
	assert.equal(readCookie(request(`  ${COOKIE}  =  abc  `)), "abc");
});

test("a missing or foreign cookie reads as null", () => {
	assert.equal(readCookie(request()), null);
	assert.equal(readCookie(request("other=1")), null);
	assert.equal(readCookie(request("nonsense")), null);
	// A prefix of the name is not the name.
	assert.equal(readCookie(request("__Host-ohayous=abc")), null);
	assert.equal(readCookie(request("ohayou=abc")), null);
	// Names this cookie used to have are not the name either.
	assert.equal(readCookie(request("__Host-drop=abc")), null);
	assert.equal(readCookie(request("__Host-hemera=abc")), null);
});

test("a session cookie is host-only, script-proof and lasts a month", async () => {
	const env = db();
	const header = await issueSession(env, {
		account: "someone",
		nick: "someone_",
		channels: ["#chan"],
		scopes: SCOPE_DROP,
	});

	const parts = attributes(header);
	assert.ok(parts.has("HttpOnly"));
	assert.ok(parts.has("Secure"));
	assert.ok(parts.has("SameSite=Lax"));
	assert.ok(parts.has("Path=/"));
	assert.ok(parts.has(`Max-Age=${TTL / 1000}`));
	// __Host- is void if either of these appears.
	assert.equal(header.includes("Domain="), false);
	assert.ok(header.startsWith(`${COOKIE}=`));
});

test("the cookie value is not the stored id", async () => {
	const env = db();
	const header = await issueSession(env, {
		account: "someone",
		nick: "someone_",
		channels: ["#chan"],
		scopes: SCOPE_DROP,
	});

	const value = header.slice(`${COOKIE}=`.length, header.indexOf(";"));
	const [, insert] = env.calls;
	assert.equal(insert.params[0].length, 64); // sha-256, hex
	assert.notEqual(insert.params[0], value);
});

test("two sessions do not collide", async () => {
	const of = async () => {
		const env = db();
		await issueSession(env, {
			account: "a",
			nick: "a",
			channels: ["#chan"],
			scopes: SCOPE_DROP,
		});
		return env.calls[1].params[0];
	};
	assert.notEqual(await of(), await of());
});

test("the channels and scopes are stored as given", async () => {
	const env = db();
	await issueSession(env, {
		account: "someone",
		nick: "someone_",
		channels: ["#chan", "#other"],
		scopes: SCOPE_DROP | SCOPE_OHAYOU,
	});

	const [, insert] = env.calls;
	assert.deepEqual(JSON.parse(insert.params[3]), ["#chan", "#other"]);
	assert.equal(insert.params[4], SCOPE_DROP | SCOPE_OHAYOU);
});

test("a stored session reads back whole", async () => {
	const row = stored();
	const env = db(row);
	const session = await readSession(request(`${COOKIE}=abc`), env);

	assert.deepEqual(session, {
		account: "someone",
		nick: "someone_",
		channels: ["#chan"],
		scopes: SCOPE_DROP,
		expires: row.expires,
	});
});

// The point of storing the scopes: a cookie minted for one part of the site
// must not reach another.
test("a scope the session does not carry is refused", async () => {
	const env = db(stored());
	const req = request(`${COOKIE}=abc`);

	assert.ok(await requireScope(req, env, SCOPE_DROP));
	assert.equal(await requireScope(req, env, SCOPE_OHAYOU), null);
	assert.equal(await requireScope(req, env, SCOPE_DROP | SCOPE_OHAYOU), null);
});

test("a session carrying both scopes reaches both", async () => {
	const env = db(stored({ scopes: SCOPE_DROP | SCOPE_OHAYOU }));
	const req = request(`${COOKIE}=abc`);

	assert.ok(await requireScope(req, env, SCOPE_DROP));
	assert.ok(await requireScope(req, env, SCOPE_OHAYOU));
	assert.ok(await requireScope(req, env, SCOPE_DROP | SCOPE_OHAYOU));
});

// A row written before scopes existed carries none, so it opens nothing.
test("a session with no scopes reaches nothing", async () => {
	const env = db(stored({ scopes: 0 }));
	const req = request(`${COOKIE}=abc`);

	assert.ok(await readSession(req, env));
	assert.equal(await requireScope(req, env, SCOPE_DROP), null);
});

test("no cookie is no session, whatever the scope", async () => {
	const env = db(stored());
	assert.equal(await readSession(request(), env), null);
	assert.equal(await requireScope(request(), env, SCOPE_DROP), null);
});

/* ---- renewal ---- */

test("a session with most of its life left is not written to", async () => {
	const env = db(stored());
	const session = await readSession(request(`${COOKIE}=abc`), env);

	assert.equal(
		await renewSession(request(`${COOKIE}=abc`), env, session),
		null,
	);
	assert.equal(env.calls.length, 1); // the read, and nothing else
});

test("a session past a third of its life slides both halves", async () => {
	// Two-thirds spent, so a third of the TTL is left.
	const env = db(stored({ expires: Date.now() + TTL / 3 }));
	const session = await readSession(request(`${COOKIE}=abc`), env);
	const header = await renewSession(request(`${COOKIE}=abc`), env, session);

	assert.ok(attributes(header).has(`Max-Age=${TTL / 1000}`));
	assert.ok(header.startsWith(`${COOKIE}=abc;`));

	const update = env.calls[1];
	assert.match(update.sql, /UPDATE session SET expires/);
	// The new expiry is a fresh TTL, not an extension of the old one.
	assert.ok(update.params[0] > Date.now() + TTL - 5000);
});

// The UPDATE re-checks the expiry, so a race that lost is not a live cookie.
test("a session that expired between the read and the write is not renewed", async () => {
	const env = db(stored({ expires: Date.now() + 1 }), 0);
	const session = await readSession(request(`${COOKIE}=abc`), env);

	assert.equal(
		await renewSession(request(`${COOKIE}=abc`), env, session),
		null,
	);
});

test("there is nothing to renew without a session", async () => {
	const env = db();
	assert.equal(await renewSession(request(`${COOKIE}=abc`), env, null), null);
	assert.equal(env.calls.length, 0);
});

test("clearing expires the browser copy and drops the row", async () => {
	const env = db();
	const header = await clearSession(request(`${COOKIE}=abc`), env);

	assert.ok(attributes(header).has("Max-Age=0"));
	assert.equal(header.startsWith(`${COOKIE}=;`), true);
	assert.equal(env.calls.length, 1);
	assert.match(env.calls[0].sql, /DELETE FROM session/);
});

test("clearing without a cookie touches nothing", async () => {
	const env = db();
	const header = await clearSession(request(), env);

	assert.ok(attributes(header).has("Max-Age=0"));
	assert.equal(env.calls.length, 0);
});
