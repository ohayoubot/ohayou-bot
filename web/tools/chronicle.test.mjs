/* The chronicle's words, and the feed that serves them. */

import assert from "node:assert/strict";
import test from "node:test";
import { onRequestGet } from "../lib/ohayou/lately.js";
import { ago, phrase, readable } from "../public/ohayou/chronicle.js";

/** Every kind the bot records, with the detail each needs. */
const KINDS = [
	["settle", {}],
	["land", { acres: "4" }],
	["build", { thing: "refinery" }],
	["strike", { metal: "gold" }],
	["steal", { took: "a purse of ohayous" }],
	["caught", {}],
	["cat", {}],
	["flag", { deer: "artbutt" }],
	["double", {}],
];

function event(overrides = {}) {
	return {
		id: 1,
		ts: Math.floor(Date.now() / 1000),
		kind: "cat",
		actor: "alice",
		subject: "",
		detail: {},
		...overrides,
	};
}

test("every kind the bot records has words here", () => {
	for (const [kind, detail] of KINDS) {
		const said = phrase(event({ kind, detail, subject: "bob" }));
		assert.ok(said, `${kind} reads as nothing`);
		assert.match(said, /\.$/, `${kind} does not end a sentence`);
	}
});

// A kind the bot learns before this does costs a line, not a page.
test("a kind with no words is dropped rather than guessed at", () => {
	assert.equal(phrase(event({ kind: "invented-later" })), null);
	assert.deepEqual(readable([event({ kind: "invented-later" })]), []);
});

// The bot empties the name before publishing. This is what the reader does
// with it, and it must not be a blank where a name goes.
test("an unnamed actor reads as Someone", () => {
	const said = phrase(
		event({ actor: "", kind: "build", detail: { thing: "factory" } }),
	);

	assert.match(said, /^Someone raised a factory\.$/);
});

test("an unnamed victim reads as someone", () => {
	const said = phrase(
		event({
			kind: "steal",
			actor: "alice",
			subject: "",
			detail: { took: "a cat" },
		}),
	);

	assert.equal(said, "alice robbed someone of a cat.");
});

// A world event is nobody's doing and must not be attributed.
test("the distributor is not somebody", () => {
	const said = phrase(event({ kind: "double", actor: "" }));

	assert.equal(said.includes("Someone"), false);
});

test("a detail that is missing costs the line, not the page", () => {
	assert.equal(phrase(event({ kind: "build", detail: {} })), null);
	assert.equal(phrase(event({ kind: "strike", detail: {} })), null);
	assert.equal(phrase(event({ kind: "steal", detail: {} })), null);
	assert.equal(
		phrase(event({ kind: "land", detail: { acres: "nope" } })),
		null,
	);
});

// A flag taken down is a flag event with no deer, and reads as one.
test("striking a flag reads without a name", () => {
	assert.equal(
		phrase(event({ kind: "flag", detail: {} })),
		"alice struck their flag.",
	);
});

test("acres agree with themselves", () => {
	assert.equal(
		phrase(event({ kind: "land", detail: { acres: "1" } })),
		"alice bought 1 acre.",
	);
	assert.equal(
		phrase(event({ kind: "land", detail: { acres: "4" } })),
		"alice bought 4 acres.",
	);
});

test("ago reads in one unit", () => {
	const now = 1_700_000_000_000;
	const at = (seconds) => Math.floor(now / 1000) - seconds;

	assert.equal(ago(at(30), now), "just now");
	assert.equal(ago(at(300), now), "5m");
	assert.equal(ago(at(7200), now), "2h");
	assert.equal(ago(at(100_000), now), "yesterday");
	assert.equal(ago(at(400_000), now), "4d");
	// A clock disagreeing with the bot's must not read as the future.
	assert.equal(ago(at(-500), now), "just now");
});

test("readable stops at the limit it is given", () => {
	const many = Array.from({ length: 20 }, (_, i) => event({ id: i }));

	assert.equal(readable(many, 6).length, 6);
	assert.equal(readable(many).length, 20);
	assert.equal(readable(undefined).length, 0);
});

/* ---- the feed ---- */

/** A GAME binding answering with the planted rows. */
function game(rows, published = { updated: 1_700_000_000_000 }) {
	const bound = {
		all: async () => ({ results: rows }),
		first: async () => published,
	};
	return {
		GAME: {
			prepare: () => ({ ...bound, bind: () => bound }),
		},
	};
}

async function lately(env, url = "https://hemera.day/ohayou/api/lately") {
	const response = await onRequestGet({ request: new Request(url), env });
	return { response, body: await response.json() };
}

test("the feed comes back parsed", async () => {
	const { body } = await lately(
		game([
			{
				id: 9,
				ts: 1_700_000_000,
				kind: "build",
				actor: "alice",
				subject: "",
				detail: '{"thing":"refinery"}',
			},
		]),
	);

	assert.equal(body.status, "lately");
	assert.deepEqual(body.events[0], {
		id: 9,
		ts: 1_700_000_000,
		kind: "build",
		actor: "alice",
		subject: "",
		detail: { thing: "refinery" },
	});
	assert.equal(body.updated, 1_700_000_000_000);
});

test("unreadable detail does not take the feed down", async () => {
	const { body } = await lately(
		game([
			{ id: 1, ts: 1, kind: "cat", actor: "alice", subject: "", detail: "{{{" },
		]),
	);

	assert.deepEqual(body.events[0].detail, {});
});

test("a missing binding answers an empty feed", async () => {
	const { body } = await lately({});

	assert.equal(body.status, "lately");
	assert.deepEqual(body.events, []);
});

test("a nick that is not one asks the database nothing", async () => {
	const refusing = {
		GAME: {
			prepare: () => {
				throw new Error("the database was asked");
			},
		},
	};

	for (const nick of ["", "x".repeat(49)]) {
		const { body } = await lately(
			refusing,
			`https://hemera.day/ohayou/api/lately?nick=${encodeURIComponent(nick)}`,
		);
		assert.deepEqual(body.events, [], JSON.stringify(nick));
	}
});

test("the feed may be cached briefly", async () => {
	const { response } = await lately(game([]));

	assert.match(response.headers.get("cache-control"), /max-age=30/);
});
