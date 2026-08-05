/* The public tier: an unnamed plot stays unnamed even if a row carries a nick. */

import assert from "node:assert/strict";
import test from "node:test";
import { onRequestGet } from "../lib/ohayou/world.js";

function row(overrides = {}) {
	return {
		id: "Mallow",
		nick: "mallow",
		named: 1,
		flag: "senordeer",
		acres: 6,
		land: '{"cat":25}',
		wealth: "industrialist",
		rations: 120,
		...overrides,
	};
}

/** A GAME binding answering with the planted rows, and optionally a gallery
    holding the given deer. */
function game(rows, published = { updated: 1_700_000_000_000 }, deer = null) {
	const answering = (results, first) => ({
		prepare: () => ({
			bind: () => ({
				all: async () => ({ results }),
				first: async () => first,
			}),
			all: async () => ({ results }),
			first: async () => first,
		}),
	});
	const env = { GAME: answering(rows, published) };
	if (deer) env.DB = answering(deer, null);
	return env;
}

async function world(env) {
	const response = await onRequestGet({ env });
	return { response, body: await response.json() };
}

test("a named plot comes back whole", async () => {
	const { body } = await world(game([row()]));

	assert.equal(body.status, "world");
	assert.deepEqual(body.plots[0], {
		id: "Mallow",
		nick: "mallow",
		named: true,
		flag: "senordeer",
		acres: 6,
		land: { cat: 25 },
		wealth: "industrialist",
		rations: 120,
	});
});

// The bot publishes no nick for an unnamed plot; this is the last place that
// can decline to repeat one if a row ever carried it.
test("an unnamed plot is served without a nick or a flag", async () => {
	const { body } = await world(
		game([row({ id: "opaque", nick: "mallow", named: 0, flag: "senordeer" })]),
	);

	assert.equal(body.plots[0].named, false);
	assert.equal(body.plots[0].nick, "");
	assert.equal(body.plots[0].flag, "");
	assert.equal(JSON.stringify(body).includes("mallow"), false);
	assert.equal(JSON.stringify(body).includes("senordeer"), false);
});

// Beside the plots rather than inside them: a deer twenty people fly is sent
// once.
test("a flag's art is resolved from the gallery", async () => {
	const { body } = await world(
		game([row(), row({ id: "b", flag: "senordeer" })], undefined, [
			{ deer: "senordeer", kinskode: "AB\nCD" },
		]),
	);

	assert.deepEqual(body.flags, { senordeer: "AB\nCD" });
});

// A deer may have been renamed since somebody picked it.
test("a flag matching no deer is simply absent", async () => {
	const { body } = await world(game([row({ flag: "gone" })], undefined, []));

	assert.deepEqual(body.flags, {});
	assert.equal(body.plots[0].flag, "gone");
});

test("no gallery binding is not a broken map", async () => {
	const { response, body } = await world(game([row()]));

	assert.equal(response.status, 200);
	assert.deepEqual(body.flags, {});
});

test("totals are counted from the rows, not stored", async () => {
	const { body } = await world(
		game([
			row({ id: "a", acres: 6, named: 1 }),
			row({ id: "b", acres: 2, named: 0 }),
			row({ id: "c", acres: 1, named: 0 }),
		]),
	);

	assert.deepEqual(body.totals, { players: 3, named: 1, acres: 9 });
});

test("an empty world is a world", async () => {
	const { response, body } = await world(game([]));

	assert.equal(response.status, 200);
	assert.deepEqual(body.plots, []);
	assert.deepEqual(body.totals, { players: 0, named: 0, acres: 0 });
});

// A projection nobody has published is not an error page.
test("a missing binding answers an empty map", async () => {
	const { response, body } = await world({});

	assert.equal(response.status, 200);
	assert.deepEqual(body.plots, []);
});

test("unreadable land does not take the map down", async () => {
	const { body } = await world(
		game([row({ land: "not json" }), row({ id: "b", land: "[1,2]" })]),
	);

	assert.deepEqual(body.plots[0].land, {});
	assert.deepEqual(body.plots[1].land, {});
});

test("the map may be cached briefly", async () => {
	const { response } = await world(game([row()]));

	assert.match(response.headers.get("cache-control"), /max-age=\d+/);
});
