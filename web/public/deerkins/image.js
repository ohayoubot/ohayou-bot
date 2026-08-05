/*
 * Turning a dropped image into kinskode.
 *
 * Pure functions over {data, width, height} — the same shape as an ImageData —
 * so the whole pipeline runs under node --test without a DOM. app.js supplies
 * the real ImageData; see readPixels there.
 *
 * The three steps are fit (how many cells), sample (average the source pixels
 * into those cells) and quantise (pick one of the 16 mIRC colours per cell).
 */

import { MAX_COLS, MAX_ROWS, PALETTE, TRANSPARENT } from "./kins.js";

/** A cell is 5/8 as wide as it is tall. Kept in step with .cell in style.css
    and with CELL_W / CELL_H in app.js. */
export const CELL_ASPECT = 5 / 8;

/** A cell this transparent on average stays transparent. */
export const ALPHA_THRESHOLD = 0.5;

/** How much of a cell's error to push into its neighbours. No palette entry is
    pastel, so hue only appears by mixing; damping chroma turns pale subjects
    grey. Lightness is held back a little to keep flat areas flat. */
export const DITHER_CHROMA = 1;
export const DITHER_LIGHTNESS = 0.85;

/** Share of a cell's chroma error surviving at lightness 0, reaching all of it
    at lightness 1. Oklab's a and b are absolute, so without this a near-black
    area builds up error until it spatters navy and maroon. Below 0.3 shaded
    colours go neutral. */
export const DARK_CHROMA_FLOOR = 0.3;

/** What a chroma or hue difference costs against a lightness difference.

    The palette is four neutrals and twelve saturated entries with nothing
    between, so plain Euclidean Oklab crosses the gap and rgb(60,60,60) matches
    maroon. Worst case is a neutral at lightness 0.298, midway between black and
    grey, needing 3.52.

    Hue costs more because a/b distance cannot separate a small hue error from a
    small chroma one, which let teal collect washed-out cells of any hue. Much
    above 13 costs saturation instead, since a neutral pays nothing for hue. */
export const CHROMA_WEIGHT = 4.5;
export const HUE_WEIGHT = 10;

// Averaging has to happen in linear light or every downscale comes out too
// dark. The table is small enough to build at load.
const TO_LINEAR = new Float64Array(256);
for (let i = 0; i < 256; i++) {
	const c = i / 255;
	TO_LINEAR[i] = c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
}

/** Linear sRGB to Oklab. Björn Ottosson's coefficients. */
function toOklab(r, g, b) {
	const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b);
	const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b);
	const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b);
	return [
		0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s,
		1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s,
		0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s,
	];
}

const PALETTE_LAB = PALETTE.map(({ hex }) =>
	toOklab(
		TO_LINEAR[Number.parseInt(hex.slice(1, 3), 16)],
		TO_LINEAR[Number.parseInt(hex.slice(3, 5), 16)],
		TO_LINEAR[Number.parseInt(hex.slice(5, 7), 16)],
	),
);

const PALETTE_CHROMA = PALETTE_LAB.map(([, A, B]) => Math.hypot(A, B));

/**
 * The largest grid within MAX_COLS x MAX_ROWS that keeps the image's shape,
 * given that cells are taller than they are wide.
 */
export function fitGrid(width, height) {
	if (!(width > 0) || !(height > 0)) return { cols: 1, rows: 1 };

	// cols * CELL_ASPECT / rows == width / height
	let cols = MAX_COLS;
	let rows = Math.round((cols * CELL_ASPECT * height) / width);

	if (rows > MAX_ROWS) {
		rows = MAX_ROWS;
		cols = Math.round((rows * width) / (height * CELL_ASPECT));
	}

	return {
		cols: Math.min(MAX_COLS, Math.max(1, cols)),
		rows: Math.min(MAX_ROWS, Math.max(1, rows)),
	};
}

/**
 * Box-average the source into cols x rows cells. Returns a flat Float64Array
 * of [linear r, g, b, mean alpha] per cell.
 *
 * Colour is averaged weighted by alpha and then divided back out, so a cell
 * that is mostly transparent still takes the colour of the pixels that were
 * actually there rather than fading towards black.
 */
export function sample(image, cols, rows) {
	const { data, width, height } = image;
	const out = new Float64Array(cols * rows * 4);

	for (let cy = 0; cy < rows; cy++) {
		const y0 = Math.floor((cy * height) / rows);
		const y1 = Math.max(y0 + 1, Math.floor(((cy + 1) * height) / rows));

		for (let cx = 0; cx < cols; cx++) {
			const x0 = Math.floor((cx * width) / cols);
			const x1 = Math.max(x0 + 1, Math.floor(((cx + 1) * width) / cols));

			let r = 0;
			let g = 0;
			let b = 0;
			let a = 0;
			let n = 0;

			for (let y = y0; y < y1; y++) {
				for (let x = x0; x < x1; x++) {
					const p = (y * width + x) * 4;
					const alpha = data[p + 3] / 255;
					r += TO_LINEAR[data[p]] * alpha;
					g += TO_LINEAR[data[p + 1]] * alpha;
					b += TO_LINEAR[data[p + 2]] * alpha;
					a += alpha;
					n++;
				}
			}

			const i = (cy * cols + cx) * 4;
			out[i] = a > 0 ? r / a : 0;
			out[i + 1] = a > 0 ? g / a : 0;
			out[i + 2] = a > 0 ? b / a : 0;
			out[i + 3] = n > 0 ? a / n : 0;
		}
	}

	return out;
}

/**
 * Index into PALETTE of the closest colour to an Oklab triple. The a/b distance
 * splits into a chroma part and a hue part, priced separately; see
 * CHROMA_WEIGHT.
 */
function nearestLab(L, A, B, weight = CHROMA_WEIGHT, hue = HUE_WEIGHT) {
	const C = Math.hypot(A, B);
	let best = 0;
	let bestD = Number.POSITIVE_INFINITY;

	for (let i = 0; i < PALETTE_LAB.length; i++) {
		const [pL, pA, pB] = PALETTE_LAB[i];
		const dL = L - pL;
		const dA = A - pA;
		const dB = B - pB;
		const dC = C - PALETTE_CHROMA[i];
		const dH = Math.max(0, dA * dA + dB * dB - dC * dC);
		const d = dL * dL + weight * dC * dC + hue * dH;

		if (d < bestD) {
			bestD = d;
			best = i;
		}
	}

	return best;
}

/** Index into PALETTE of the closest colour to an sRGB byte triple. */
export function nearest(r, g, b, weight = CHROMA_WEIGHT, hue = HUE_WEIGHT) {
	const byte = (v) => Math.min(255, Math.max(0, Math.round(v)));
	const [L, A, B] = toOklab(
		TO_LINEAR[byte(r)],
		TO_LINEAR[byte(g)],
		TO_LINEAR[byte(b)],
	);
	return nearestLab(L, A, B, weight, hue);
}

function spread(err, cols, rows, x, y, share, eL, eA, eB) {
	if (x < 0 || x >= cols || y < 0 || y >= rows) return;
	const i = (y * cols + x) * 3;
	err[i] += eL * share;
	err[i + 1] += eA * share;
	err[i + 2] += eB * share;
}

/**
 * Convert an ImageData-shaped object to kinskode.
 *
 * Matching and error diffusion both happen in Oklab. Diffusing in sRGB spreads
 * error unevenly across the tone range, and matching in sRGB is what sent
 * pastel greens to grey; see CHROMA_WEIGHT.
 *
 * Dithering is Floyd-Steinberg, scanning in alternating directions so the error
 * does not streak to one side. Off by default: at 40x30 the cells are too big
 * to blend and the mixing reads as texture. It is there for photographs, which
 * flat matching turns to mud.
 */
export function imageToKinskode(
	image,
	{
		dither = false,
		chroma = DITHER_CHROMA,
		lightness = DITHER_LIGHTNESS,
		floor = DARK_CHROMA_FLOOR,
		weight = CHROMA_WEIGHT,
		hue = HUE_WEIGHT,
	} = {},
) {
	const { cols, rows } = fitGrid(image.width, image.height);
	const cells = sample(image, cols, rows);
	const err = new Float64Array(cols * rows * 3);
	const lines = [];

	for (let y = 0; y < rows; y++) {
		const ltr = y % 2 === 0;
		const line = new Array(cols).fill(TRANSPARENT);

		for (let n = 0; n < cols; n++) {
			const x = ltr ? n : cols - 1 - n;
			const c = (y * cols + x) * 4;
			if (cells[c + 3] < ALPHA_THRESHOLD) continue;

			const e = (y * cols + x) * 3;
			const [L0, A0, B0] = toOklab(cells[c], cells[c + 1], cells[c + 2]);
			const L = L0 + err[e];
			const A = A0 + err[e + 1];
			const B = B0 + err[e + 2];

			const i = nearestLab(L, A, B, weight, hue);
			line[x] = PALETTE[i].kins;
			if (!dither) continue;

			// L0, not L, so the error that landed here does not decide this.
			const lit = floor + (1 - floor) * Math.min(1, Math.max(0, L0));

			const [pL, pA, pB] = PALETTE_LAB[i];
			const eL = (L - pL) * lightness;
			const eA = (A - pA) * chroma * lit;
			const eB = (B - pB) * chroma * lit;
			const step = ltr ? 1 : -1;

			spread(err, cols, rows, x + step, y, 7 / 16, eL, eA, eB);
			spread(err, cols, rows, x - step, y + 1, 3 / 16, eL, eA, eB);
			spread(err, cols, rows, x, y + 1, 5 / 16, eL, eA, eB);
			spread(err, cols, rows, x + step, y + 1, 1 / 16, eL, eA, eB);
		}

		lines.push(line.join(""));
	}

	return lines.join("\n");
}
