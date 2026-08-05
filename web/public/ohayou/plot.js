/*
 * Plot and world geometry, shared by the map, the dashboard and the svg card.
 *
 * Lives under public/ because the browser loads it and the worker imports it,
 * the way the gallery's kins.js is shared.
 */

import { ACRELIMIT, acresFor } from "./catalog.js";

/**
 * Where a plot's acres go.
 *
 * tiles is one entry per acre: null for empty land, otherwise { item, n } where
 * n is how many of that item stand on that acre, capped by its acre limit. wide
 * by tall is the block the plot fills, square bar the remainder.
 *
 * Buildings are dealt out in name order, so a holding always draws the same
 * way. Which acre one sits on is not a game rule.
 */
export function layout(plot) {
	const acres = Math.max(1, plot.acres);
	const wide = Math.ceil(Math.sqrt(acres));

	const built = [];
	for (const [item, count] of Object.entries(plot.land ?? {}).sort()) {
		const limit = ACRELIMIT[item] ?? 1;
		let left = count;
		const spread = acresFor(item, count);
		for (let i = 0; i < spread && built.length < acres; i++) {
			const n = Math.min(limit, left);
			built.push({ item, n });
			left -= n;
		}
	}

	// Dealt across the parcel rather than filling from one corner. Striding by a
	// number coprime with the acreage visits every acre once and spreads them
	// evenly, which a shuffle does not: a shuffle clumps often enough to look
	// like a bug. Where it starts is seeded by the plot, so the same holding
	// draws the same way on every publish and in every renderer.
	const step = stride(acres);
	const from = hash(plot.id) % acres;
	const tiles = new Array(acres).fill(null);
	built.forEach((tile, i) => {
		tiles[(from + i * step) % acres] = tile;
	});

	return { acres, wide, tall: Math.ceil(acres / wide), tiles };
}

/** The golden-ratio step, walked up until it shares no factor with n. */
function stride(n) {
	let step = Math.max(1, Math.round(n * 0.6180339887));
	while (step > 1 && gcd(step, n) !== 1) step--;
	return step;
}

function gcd(a, b) {
	return b === 0 ? a : gcd(b, a % b);
}

function hash(id) {
	let out = 2166136261;
	for (const ch of String(id)) {
		out = Math.imul(out ^ ch.charCodeAt(0), 16777619) >>> 0;
	}
	return out;
}

/** Acres a plot has built on, and acres it has spare. */
export function usage(plot) {
	const built = Object.entries(plot.land ?? {}).reduce(
		(sum, [item, n]) => sum + acresFor(item, n),
		0,
	);
	const acres = Math.max(1, plot.acres);
	return {
		acres,
		built: Math.min(built, acres),
		spare: Math.max(0, acres - built),
	};
}

/* ---- the world ---- */

/** Blank tiles left around a parcel, which the map draws as grass and track. */
export const VERGE = 1;

/**
 * Lays every plot out on one landmass.
 *
 * A skyline packer: each parcel goes at the lowest, then leftmost, position
 * that fits, so a small holding settles into the gap a larger one left rather
 * than starting a new row. Order in decides position out, so a caller passing a
 * stable order keeps neighbours neighbours between publishes.
 *
 * Returns parcels in tile coordinates, and the ground they need.
 */
export function worldLayout(plots, { width } = {}) {
	const shaped = plots.map((plot) => {
		const { wide, tall, tiles, acres } = layout(plot);
		return { plot, tiles, acres, w: wide, h: tall };
	});

	const across = width ?? worldWidth(shaped);
	const skyline = new Array(across).fill(0);
	const parcels = [];

	for (const parcel of shaped) {
		const w = Math.min(across, parcel.w + VERGE * 2);
		const h = parcel.h + VERGE * 2;
		const at = lowestFit(skyline, w);

		parcels.push({ ...parcel, x: at.x + VERGE, y: at.y + VERGE });
		for (let i = at.x; i < at.x + w; i++) skyline[i] = at.y + h;
	}

	return {
		parcels,
		width: across,
		height: Math.max(0, ...skyline),
	};
}

/** The lowest run of w columns, leftmost when several are level. */
function lowestFit(skyline, w) {
	let best = { x: 0, y: Number.POSITIVE_INFINITY };
	for (let x = 0; x + w <= skyline.length; x++) {
		let y = 0;
		for (let i = x; i < x + w; i++) y = Math.max(y, skyline[i]);
		if (y < best.y) best = { x, y };
	}
	return best;
}

/** A landscape rather than a column: roughly twice as wide as it is tall. */
function worldWidth(shaped) {
	const area = shaped.reduce(
		(sum, p) => sum + (p.w + VERGE * 2) * (p.h + VERGE * 2),
		0,
	);
	const widest = Math.max(0, ...shaped.map((p) => p.w + VERGE * 2));
	return Math.max(12, widest, Math.ceil(Math.sqrt(area * 2.2)));
}
