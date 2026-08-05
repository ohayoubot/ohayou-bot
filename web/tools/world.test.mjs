/*
 * The world endpoint serves the tier the bot publishes for everyone, so the
 * thing worth checking is that an unnamed plot stays unnamed even if a row
 * somehow carries a nick.
 */

import assert from "node:assert/strict";
import test from "node:test";
import { onRequestGet } from "../lib/ohayou/world.js";

function row(overrides = {}) {
	return {
		id: "Mallow",
		nick: "mallow",
		named: 1,
		acres: 6,
		land: '{"cat":25}',
		wealth: "industrialist",
		rations: 120,
		...overrides,
	};
}

/** A GAME binding answering with the planted rows. */
function game(rows, published = { updated: 1_700_000_000_000 }) {
	return {
		GAME: {
			prepare: () => ({
				bind: () => ({
					all: async () => ({ results: rows }),
					first: async () => published,
				}),
				all: async () => ({ results: rows }),
				first: async () => published,
			}),
		},
	};
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
		acres: 6,
		land: { cat: 25 },
		wealth: "industrialist",
		rations: 120,
	});
});

// The bot does not publish a nick for an unnamed plot. If one ever appeared in
// a row, this endpoint is the last place that can decline to repeat it.
test("an unnamed plot is served without a nick", async () => {
	const { body } = await world(
		game([row({ id: "opaque", nick: "mallow", named: 0 })]),
	);

	assert.equal(body.plots[0].named, false);
	assert.equal(body.plots[0].nick, "");
	assert.equal(JSON.stringify(body).includes("mallow"), false);
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
