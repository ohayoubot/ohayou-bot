/* Sprites, the acre catalogue and the packing behind the world map. */

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import {
	KINS_CHARS,
	MAX_COLS,
	MAX_ROWS,
	toRects,
} from "../public/deerkins/kins.js";
import { ACRELIMIT, acresFor, groundFor } from "../public/ohayou/catalog.js";
import {
	fieldOf,
	layout,
	usage,
	VERGE,
	worldLayout,
} from "../public/ohayou/plot.js";
import {
	SPRITE_SIZE,
	SPRITES,
	spriteFor,
	spriteURL,
	UNKNOWN,
} from "../public/ohayou/sprites.js";
import { BANDS, bandColour, TERRAIN } from "../public/ohayou/terrain.js";

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

/* ---- the colours css and the worker both draw with ---- */

const css = readFileSync(
	new URL("../public/ohayou/style.css", import.meta.url),
	"utf8",
);

// The map is painted by css and the card by a worker. They have to agree.
test("style.css repeats the terrain colours exactly", () => {
	for (const [name, hex] of Object.entries(TERRAIN)) {
		const declared = css.match(new RegExp(`--${name}:\\s*(#[0-9a-f]{6})`, "i"));
		assert.ok(declared, `--${name} is not in style.css`);
		assert.equal(declared[1].toLowerCase(), hex, `--${name}`);
	}
});

test("every wealth band has a colour and a rule", () => {
	for (const band of BANDS) {
		assert.match(bandColour(band), /^#[0-9a-f]{6}$/i, band);
		assert.ok(css.includes(`[data-band="${band}"]`), `${band} has no css rule`);
	}
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

/*
 * Buildings gather into a steading in one corner. Scattering them evenly is
 * what made a large holding read as icons sprinkled over a rectangle, so this
 * is a look worth keeping rather than an implementation detail.
 */
test("a holding's buildings gather in one corner", () => {
	const big = plot({
		acres: 100,
		land: { cat: 200, quarry: 6, workshop: 20, refinery: 4 },
	});
	const { tiles, wide } = layout(big);

	const at = tiles.flatMap((t, i) =>
		t ? [[i % wide, Math.floor(i / wide)]] : [],
	);
	assert.ok(at.length > 8, "not enough built acres to judge");

	// 24 acres of buildings on a 10 by 10 holding: a steading of about five
	// tiles a side, not a sprinkling across all ten.
	const xs = at.map(([x]) => x);
	const ys = at.map(([, y]) => y);
	const spanX = Math.max(...xs) - Math.min(...xs) + 1;
	const spanY = Math.max(...ys) - Math.min(...ys) + 1;
	const room = Math.ceil(Math.sqrt(at.length)) + 2;
	assert.ok(spanX <= room, `buildings span ${spanX} columns, wanted ${room}`);
	assert.ok(spanY <= room, `buildings span ${spanY} rows, wanted ${room}`);
	assert.ok(spanX < wide && spanY < wide, "the steading fills the holding");

	const held = new Set(at.map(([x, y]) => `${x},${y}`));
	const touching = at.filter(([x, y]) =>
		[
			[1, 0],
			[-1, 0],
			[0, 1],
			[0, -1],
		].some(([dx, dy]) => held.has(`${x + dx},${y + dy}`)),
	);
	assert.equal(touching.length, at.length, "a building stands on its own");
});

test("the crops are stable, in range, and a field is more than an acre", () => {
	const of = (x, y) => fieldOf("Mallow", x, y);

	for (let y = 0; y < 6; y++) {
		for (let x = 0; x < 6; x++) {
			assert.ok(Number.isInteger(of(x, y)) && of(x, y) >= 0 && of(x, y) < 3);
			assert.equal(of(x, y), fieldOf("Mallow", x, y), "not deterministic");
		}
	}
	// A field is a block, so its acres agree with each other.
	assert.equal(of(0, 0), of(1, 1));
	assert.equal(of(2, 2), of(3, 3));

	// And a different holding farms its land differently.
	const across = (id) =>
		[0, 2, 4, 6].flatMap((y) => [0, 2, 4, 6].map((x) => fieldOf(id, x, y)));
	assert.notDeepEqual(across("Mallow"), across("someone-else"));
});

// The ground under a thing is what stops the map reading as icons on a lawn.
test("everything that takes land has ground to stand on", () => {
	for (const item of Object.keys(ACRELIMIT)) {
		const ground = groundFor(item);
		assert.ok(
			TERRAIN[ground],
			`${item} stands on ${ground}, which has no colour`,
		);
	}
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

// The api orders by id for this reason; the packer only preserves what it is
// given.
test("a parcel keeps its place when its holder gets richer", () => {
	const before = worldLayout(many(20)).parcels.map((p) => [
		p.plot.id,
		p.x,
		p.y,
	]);

	const richer = many(20).map((p) => ({ ...p, rations: p.rations + 5 }));
	const after = worldLayout(richer).parcels.map((p) => [p.plot.id, p.x, p.y]);

	assert.deepEqual(after, before);
});

test("a world with one plot is still a world", () => {
	const world = worldLayout([plot({ acres: 1, land: {} })]);
	assert.equal(world.parcels.length, 1);
	assert.ok(world.width > 0 && world.height > 0);
});
