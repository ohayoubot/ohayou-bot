import assert from "node:assert/strict";
import test from "node:test";
import {
	CELL_ASPECT,
	fitGrid,
	imageToKinskode,
	nearest,
	sample,
} from "../public/deerkins/image.js";
import {
	MAX_COLS,
	MAX_ROWS,
	measure,
	PALETTE,
	validate,
} from "../public/deerkins/kins.js";

/** An ImageData-alike. `fill` returns [r,g,b,a] for a pixel. */
function image(width, height, fill) {
	const data = new Uint8ClampedArray(width * height * 4);
	for (let y = 0; y < height; y++) {
		for (let x = 0; x < width; x++) {
			const [r, g, b, a] = fill(x, y);
			const p = (y * width + x) * 4;
			data[p] = r;
			data[p + 1] = g;
			data[p + 2] = b;
			data[p + 3] = a ?? 255;
		}
	}
	return { data, width, height };
}

const solid = (w, h, rgb) => image(w, h, () => rgb);
const kinsOf = (name) => PALETTE.find((c) => c.name === name).kins;
const nameOf = (kins) => PALETTE.find((c) => c.kins === kins).name;

/** The palette colour names a kinskode uses, ignoring transparency. */
const used = (kins) =>
	[...new Set(kins.replace(/[\n_]/g, ""))].map(nameOf).sort();

const NEUTRAL = new Set(["black", "grey", "silver", "white"]);

test("fitGrid keeps the image shape given non-square cells", () => {
	// The grid's usable area is 40 * 5 by 30 * 8, so it is taller than it is
	// wide in cell counts but nearly square once drawn.
	const wide = fitGrid(1000, 100);
	assert.equal(wide.cols, MAX_COLS);
	assert.equal(wide.rows, Math.round((MAX_COLS * CELL_ASPECT * 100) / 1000));

	const tall = fitGrid(100, 1000);
	assert.equal(tall.rows, MAX_ROWS);
	assert.ok(tall.cols < MAX_COLS);
});

test("fitGrid never exceeds the canvas limits", () => {
	for (const [w, h] of [
		[1, 1],
		[4000, 3000],
		[3000, 4000],
		[10000, 1],
		[1, 10000],
	]) {
		const { cols, rows } = fitGrid(w, h);
		assert.ok(cols >= 1 && cols <= MAX_COLS, `cols ${cols} for ${w}x${h}`);
		assert.ok(rows >= 1 && rows <= MAX_ROWS, `rows ${rows} for ${w}x${h}`);
	}
});

test("fitGrid survives degenerate sizes", () => {
	assert.deepEqual(fitGrid(0, 0), { cols: 1, rows: 1 });
	assert.deepEqual(fitGrid(10, Number.NaN), { cols: 1, rows: 1 });
});

test("sample averages a cell in linear light", () => {
	// Half black, half white. An sRGB average gives 127; averaging in linear
	// light and converting back gives ~188.
	const half = image(2, 1, (x) => (x === 0 ? [0, 0, 0] : [255, 255, 255]));
	const [r] = sample(half, 1, 1);
	assert.ok(Math.abs(r - 0.5) < 1e-9, `linear mean was ${r}`);
});

test("sample reports mean alpha and ignores transparent pixels' colour", () => {
	const img = image(2, 1, (x) => (x === 0 ? [255, 0, 0, 255] : [0, 255, 0, 0]));
	const cell = sample(img, 1, 1);
	assert.equal(cell[3], 0.5);
	assert.ok(cell[0] > 0.9, "kept the opaque red");
	assert.ok(cell[1] < 1e-9, "did not blend in the transparent green");
});

test("sample splits a source evenly across cells", () => {
	const img = image(4, 2, (x) => (x < 2 ? [255, 0, 0] : [0, 0, 255]));
	const cells = sample(img, 2, 1);
	assert.ok(cells[0] > 0.9 && cells[2] < 1e-9, "left cell is red");
	assert.ok(cells[4] < 1e-9 && cells[6] > 0.9, "right cell is blue");
});

test("nearest returns the exact palette entry for a palette colour", () => {
	PALETTE.forEach(({ hex }, i) => {
		const r = Number.parseInt(hex.slice(1, 3), 16);
		const g = Number.parseInt(hex.slice(3, 5), 16);
		const b = Number.parseInt(hex.slice(5, 7), 16);
		assert.equal(nearest(r, g, b), i, hex);
	});
});

test("nearest picks a sane neighbour for an off-palette colour", () => {
	assert.equal(PALETTE[nearest(250, 250, 250)].name, "white");
	assert.equal(PALETTE[nearest(8, 8, 8)].name, "black");
	assert.equal(PALETTE[nearest(200, 20, 20)].name, "red");
});

test("imageToKinskode fills the grid with one flat colour", () => {
	const kins = imageToKinskode(solid(40, 25, [255, 0, 0]));
	const { rows, cols } = measure(kins);
	assert.equal(cols, MAX_COLS);
	assert.ok(rows >= 1 && rows <= MAX_ROWS);

	const unique = new Set(kins.replace(/\n/g, ""));
	assert.deepEqual([...unique], [kinsOf("red")]);
});

test("imageToKinskode leaves transparent areas transparent", () => {
	// Opaque left half, transparent right half.
	const img = image(40, 40, (x) =>
		x < 20 ? [255, 255, 255, 255] : [255, 255, 255, 0],
	);
	const lines = imageToKinskode(img).split("\n");
	for (const line of lines) {
		const half = line.length / 2;
		assert.ok(
			[...line.slice(0, half)].every((ch) => ch === kinsOf("white")),
			line,
		);
		assert.ok(
			[...line.slice(half)].every((ch) => ch === "_"),
			line,
		);
	}
});

test("dithering mixes colours that flat matching would flatten", () => {
	// Halfway between grey (#7F7F7F) and silver (#D2D2D2), so there is real
	// error to spread. A tone that sits on a palette entry would not dither.
	const grey = solid(64, 64, [168, 168, 168]);

	const flat = new Set(
		imageToKinskode(grey, { dither: false }).replace(/\n/g, ""),
	);
	assert.equal(flat.size, 1, "one colour without dithering");

	const dithered = new Set(
		imageToKinskode(grey, { dither: true }).replace(/\n/g, ""),
	);
	assert.ok(dithered.size > 1, "several colours with dithering");
});

test("dithering at zero damping matches no dithering at all", () => {
	const photo = image(200, 200, (x, y) => [x % 256, y % 256, 90]);
	assert.equal(
		imageToKinskode(photo, { dither: true, chroma: 0, lightness: 0 }),
		imageToKinskode(photo, { dither: false }),
	);
});

test("a flat dark area stays neutral rather than speckling", () => {
	// Euclidean Oklab sends these to navy and maroon, the only entries near
	// their lightness. Black and grey are fine; a hue is not.
	for (const rgb of [
		[20, 14, 26],
		[30, 20, 40],
		[20, 20, 20],
		[40, 40, 40],
	]) {
		for (const name of used(imageToKinskode(solid(80, 50, rgb)))) {
			assert.ok(NEUTRAL.has(name), `rgb(${rgb}) used ${name}`);
		}
	}
});

test("black stays a single flat black", () => {
	const kins = imageToKinskode(solid(80, 50, [0, 0, 0]));
	assert.deepEqual([...new Set(kins.replace(/\n/g, ""))], [kinsOf("black")]);
});

test("a pale colour keeps its hue instead of going grey", () => {
	// Nothing in the palette is pastel, so the hue can only arrive as saturated
	// cells mixed among the neutrals.
	for (const [rgb, wanted] of [
		[
			[240, 150, 168],
			["red", "maroon", "orange", "magenta", "purple"],
		],
		[
			[245, 195, 155],
			["orange", "yellow", "red", "maroon"],
		],
		[
			[150, 190, 150],
			["green", "lime", "teal"],
		],
		[
			[120, 140, 190],
			["blue", "navy", "purple"],
		],
	]) {
		const names = used(imageToKinskode(solid(80, 50, rgb), { dither: true }));
		const hues = names.filter((n) => !NEUTRAL.has(n));
		assert.ok(hues.length > 0, `rgb(${rgb}) came out entirely neutral`);
		for (const name of hues) {
			assert.ok(wanted.includes(name), `rgb(${rgb}) reached for ${name}`);
		}
	}
});

test("a washed-out blue does not come out teal", () => {
	// Teal is the only unsaturated entry, so Euclidean a/b distance let it
	// collect desaturated cells of any hue.
	for (const rgb of [
		[150, 160, 178],
		[120, 130, 150],
		[170, 180, 200],
	]) {
		const names = used(imageToKinskode(solid(80, 50, rgb)));
		assert.ok(!names.includes("teal"), `rgb(${rgb}) -> ${names}`);
		assert.ok(!names.includes("cyan"), `rgb(${rgb}) -> ${names}`);
	}
});

test("a gradient in lightness does not flatten to one colour", () => {
	// Shading used to collapse onto whichever entry owned the hue, so a lit
	// green ball came out one flat green.
	const width = 400;
	const ramp = image(width, 250, (x) => {
		const u = x / width;
		return [25 + 120 * u, 80 + 150 * u, 25 + 90 * u];
	});
	const kins = imageToKinskode(ramp);
	const lines = kins.split("\n");
	const half = Math.floor(lines[0].length / 2);
	const dark = new Set(lines.flatMap((l) => [...l.slice(0, half)]));
	const light = new Set(lines.flatMap((l) => [...l.slice(half)]));

	assert.ok(dark.has(kinsOf("green")), `dark end: ${[...dark]}`);
	assert.ok(light.has(kinsOf("lime")), `light end: ${[...light]}`);
});

test("neutral greys never pick up a hue", () => {
	// The regression that motivated CHROMA_WEIGHT: rgb(60,60,60) matched maroon.
	const neutral = new Set(["black", "grey", "silver", "white"]);
	for (let v = 0; v <= 255; v += 5) {
		const got = PALETTE[nearest(v, v, v)].name;
		assert.ok(neutral.has(got), `rgb(${v},${v},${v}) -> ${got}`);
	}
});

test("greens stay green instead of collapsing to grey", () => {
	// The reported bug: a green gradient came out green at one end and grey
	// across the rest. Every step of the ramp must still read as green.
	const greens = [
		[0, 100, 0],
		[0, 147, 0],
		[0, 200, 0],
		[34, 90, 34],
		[100, 200, 100],
		[120, 180, 120],
	];
	for (const rgb of greens) {
		const got = PALETTE[nearest(...rgb)].name;
		assert.ok(["green", "lime", "teal"].includes(got), `rgb(${rgb}) -> ${got}`);
	}
});

test("a green gradient keeps green across its whole width", () => {
	const width = 400;
	const grad = image(width, 250, (x) => {
		const u = x / width;
		return [40 + 180 * u, 90 + 165 * u, 40 + 180 * u];
	});
	const row = imageToKinskode(grad, { dither: true }).split("\n")[2];
	const greens = [kinsOf("green"), kinsOf("lime"), kinsOf("teal")];
	const last = row.split("").findLastIndex((ch) => greens.includes(ch));
	assert.ok(
		last > row.length * 0.6,
		`green ran out at ${last} of ${row.length}: ${row}`,
	);
});

test("imported art passes the server's validator", () => {
	const photo = image(300, 200, (x, y) => [
		(x * 7) % 256,
		(y * 11) % 256,
		(x * y) % 256,
	]);
	for (const dither of [true, false]) {
		assert.equal(validate(imageToKinskode(photo, { dither })), null);
	}
});

test("a one pixel image scales up to a square grid of that colour", () => {
	const kins = imageToKinskode(solid(1, 1, [0, 147, 0]));
	const { cols, rows } = measure(kins);
	assert.equal(cols, MAX_COLS);
	assert.equal(rows, Math.round(MAX_COLS * CELL_ASPECT));
	assert.deepEqual([...new Set(kins.replace(/\n/g, ""))], [kinsOf("green")]);
});
