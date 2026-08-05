/* Sprites, the acre catalogue and the packing behind the world map. */

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { KINS_CHARS, MAX_COLS, MAX_ROWS } from "../public/deerkins/kins.js";
import { ACRELIMIT, acresFor } from "../public/ohayou/catalog.js";
import { layout, usage, VERGE, worldLayout } from "../public/ohayou/plot.js";
import {
	SPRITE_SIZE,
	SPRITES,
	spriteFor,
	spriteURL,
	toRects,
	UNKNOWN,
} from "../public/ohayou/sprites.js";

const items = JSON.parse(
	readFileSync(new URL("../../data/items.json", import.meta.url)),
);

function plot(overrides = {}) {
	return {
		id: "Mallow",
		nick: "mallow",
		named: true,
		flag: "",
		acres: 9,
		land: { cat: 20, quarry: 1 },
		wealth: "settler",
		rations: 30,
		...overrides,
	};
}

/* ---- the catalogue against the game's own ---- */

// The acre limits decide how much land a holding covers. The game owns them.
test("every item the game has, the site can draw", () => {
	for (const item of items) {
		assert.ok(
			SPRITES[item.name],
			`${item.name} has no sprite; add one to sprites.js`,
		);
	}
});

test("the acre limits match data/items.json", () => {
	const theirs = Object.fromEntries(
		items
			.filter((i) => (i.acrelimit ?? 0) > 0)
			.map((i) => [i.name, i.acrelimit]),
	);
	assert.deepEqual(ACRELIMIT, theirs);
});

/* ---- sprites ---- */

test("every sprite is legal kinskode of the right size", () => {
	for (const [name, art] of [
		...Object.entries(SPRITES),
		["unknown", UNKNOWN],
	]) {
		const rows = art.trim().split("\n");
		assert.equal(rows.length, SPRITE_SIZE, `${name} is ${rows.length} rows`);
		assert.ok(rows.length <= MAX_ROWS && SPRITE_SIZE <= MAX_COLS);

		for (const row of rows) {
			assert.equal(row.length, SPRITE_SIZE, `${name}: "${row}"`);
			for (const ch of row) {
				assert.ok(KINS_CHARS.includes(ch), `${name} has ${JSON.stringify(ch)}`);
			}
		}
		assert.match(art, /[^_\n]/, `${name} is blank`);
	}
});

test("an item nobody has drawn still draws something", () => {
	assert.equal(spriteFor("no such item"), UNKNOWN);
});

// The card inlines these, and the card is checked for having no url in it.
test("sprite rects carry a colour and nothing else", () => {
	const rects = toRects(SPRITES.cat);
	assert.match(rects, /^<rect /);
	assert.equal(rects.includes("href"), false);
	assert.equal(rects.includes("_"), false);
});

test("a sprite url is a self-contained data url", () => {
	const url = spriteURL("cat");
	assert.match(url, /^data:image\/svg\+xml,/);
	assert.equal(url, spriteURL("cat"), "not cached");
	assert.equal(decodeURIComponent(url).includes("http"), true); // the xmlns
	assert.equal(
		/https?:\/\/(?!www\.w3\.org)/.test(decodeURIComponent(url)),
		false,
	);
});

/* ---- acres ---- */

test("a holding covers land at its own density", () => {
	assert.equal(acresFor("cat", 20), 1);
	assert.equal(acresFor("cat", 21), 2);
	assert.equal(acresFor("workshop", 5), 1);
	assert.equal(acresFor("refinery", 3), 3);
	assert.equal(acresFor("burger", 9), 0, "food takes no land");
});

test("a plot lays out one entry per acre", () => {
	const { acres, tiles, wide, tall } = layout(plot({ acres: 9 }));

	assert.equal(acres, 9);
	assert.equal(tiles.length, 9);
	assert.ok(wide * tall >= 9);

	const filled = tiles.filter(Boolean);
	assert.equal(filled.length, 2, "20 cats is one acre, the quarry another");
	assert.deepEqual(filled.map((t) => t.item).sort(), ["cat", "quarry"]);
	assert.equal(filled.find((t) => t.item === "cat").n, 20);
});

test("a holding bigger than the land it stands on is clipped to it", () => {
	const { tiles } = layout(plot({ acres: 2, land: { refinery: 9 } }));
	assert.equal(tiles.length, 2);
	assert.equal(tiles.filter(Boolean).length, 2);
});

test("the same plot lays out the same way twice", () => {
	const a = layout(plot({ acres: 16 }));
	const b = layout(plot({ acres: 16 }));
	assert.deepEqual(a.tiles, b.tiles);
});

test("worked and spare acres add up", () => {
	const { acres, built, spare } = usage(plot({ acres: 9 }));
	assert.equal(acres, 9);
	assert.equal(built, 2);
	assert.equal(spare, 7);
});

/* ---- the world ---- */

const many = (n) =>
	Array.from({ length: n }, (_, i) =>
		plot({ id: `p${i}`, acres: 1 + ((i * 7) % 24), land: { cat: i * 3 } }),
	);

test("no two parcels overlap", () => {
	const { parcels } = worldLayout(many(60));

	for (let i = 0; i < parcels.length; i++) {
		for (let j = i + 1; j < parcels.length; j++) {
			const a = parcels[i];
			const b = parcels[j];
			const apart =
				a.x + a.w <= b.x ||
				b.x + b.w <= a.x ||
				a.y + a.h <= b.y ||
				b.y + b.h <= a.y;
			assert.ok(apart, `${a.plot.id} overlaps ${b.plot.id}`);
		}
	}
});

test("every parcel is inside the ground the world claims to need", () => {
	const world = worldLayout(many(40));
	for (const p of world.parcels) {
		assert.ok(p.x >= VERGE - 1 && p.y >= 0, p.plot.id);
		assert.ok(p.x + p.w <= world.width, `${p.plot.id} runs off the east edge`);
		assert.ok(
			p.y + p.h <= world.height,
			`${p.plot.id} runs off the south edge`,
		);
	}
});

// Neighbours stay neighbours between publishes, which is the whole point of
// packing in the order the api returns rather than by size.
test("the same plots in the same order land in the same places", () => {
	const once = worldLayout(many(30)).parcels.map((p) => [p.plot.id, p.x, p.y]);
	const again = worldLayout(many(30)).parcels.map((p) => [p.plot.id, p.x, p.y]);
	assert.deepEqual(once, again);
});

test("a world with one plot is still a world", () => {
	const world = worldLayout([plot({ acres: 1, land: {} })]);
	assert.equal(world.parcels.length, 1);
	assert.ok(world.width > 0 && world.height > 0);
});
