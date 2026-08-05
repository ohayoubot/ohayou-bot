import assert from "node:assert/strict";
import test from "node:test";
import { clearSession, issueSession, readCookie } from "../lib/session.js";

const COOKIE = "__Host-drop";

function request(cookie) {
	return new Request("https://hemera.day/drop/", {
		headers: cookie ? { cookie } : {},
	});
}

/** Enough of the D1 binding for issue and clear: records what it was asked to
    run and answers nothing. */
function db() {
	const calls = [];
	const statement = (sql) => ({
		bind: (...params) => {
			calls.push({ sql, params });
			return statement(sql);
		},
		run: async () => ({}),
	});
	return {
		calls,
		DB: { prepare: statement, batch: async () => [] },
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
	assert.equal(readCookie(request("__Host-drops=abc")), null);
	assert.equal(readCookie(request("drop=abc")), null);
});

test("a session cookie is host-only, script-proof and short-lived", async () => {
	const env = db();
	const header = await issueSession(env, {
		account: "someone",
		nick: "someone_",
		channels: ["#chan"],
	});

	const parts = attributes(header);
	assert.ok(parts.has("HttpOnly"));
	assert.ok(parts.has("Secure"));
	assert.ok(parts.has("SameSite=Lax"));
	assert.ok(parts.has("Path=/"));
	assert.ok(parts.has("Max-Age=43200"));
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
	});

	const value = header.slice(`${COOKIE}=`.length, header.indexOf(";"));
	const [, insert] = env.calls;
	assert.equal(insert.params[0].length, 64); // sha-256, hex
	assert.notEqual(insert.params[0], value);
});

test("two sessions do not collide", async () => {
	const of = async () => {
		const env = db();
		await issueSession(env, { account: "a", nick: "a", channels: ["#chan"] });
		return env.calls[1].params[0];
	};
	assert.notEqual(await of(), await of());
});

test("the channel list is stored as given", async () => {
	const env = db();
	await issueSession(env, {
		account: "someone",
		nick: "someone_",
		channels: ["#chan", "#other"],
	});
	assert.deepEqual(JSON.parse(env.calls[1].params[3]), ["#chan", "#other"]);
});

test("clearing expires the browser copy and drops the row", async () => {
	const env = db();
	const header = await clearSession(request(`${COOKIE}=abc`), env);

	assert.ok(attributes(header).has("Max-Age=0"));
	assert.equal(header.startsWith(`${COOKIE}=;`), true);
	assert.equal(env.calls.length, 1);
	assert.match(env.calls[0].sql, /DELETE FROM upload_session/);
});

test("clearing without a cookie touches nothing", async () => {
	const env = db();
	const header = await clearSession(request(), env);

	assert.ok(attributes(header).has("Max-Age=0"));
	assert.equal(env.calls.length, 0);
});
