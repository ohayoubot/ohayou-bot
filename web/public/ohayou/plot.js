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
 * Buildings gather rather than scatter: acres are dealt nearest a seeded corner
 * first, so a holding reads as a steading with its fields around it instead of
 * icons sprinkled over a rectangle. Items go in name order, so each kind lands
 * in its own band and a holding always draws the same way. Which acre a thing
 * sits on is not a game rule.
 */
export function layout(plot) {
	const acres = Math.max(1, plot.acres);
	const wide = Math.ceil(Math.sqrt(acres));
	const tall = Math.ceil(acres / wide);

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

	const order = settle(acres, wide, tall, hash(plot.id));
	const tiles = new Array(acres).fill(null);
	built.forEach((tile, i) => {
		tiles[order[i]] = tile;
	});

	return { acres, wide, tall, tiles };
}

/**
 * Acre indices, nearest a seeded corner first. Squared distance rather than
 * rows, so the built land grows as a quarter-circle and its edge against the
 * fields is a curve. Ties break on the index, which makes the order total and
 * the same in every renderer.
 */
function settle(acres, wide, tall, seed) {
	const ax = seed & 1 ? wide - 1 : 0;
	const ay = seed & 2 ? tall - 1 : 0;

	const near = (i) => {
		const dx = (i % wide) - ax;
		const dy = Math.floor(i / wide) - ay;
		return dx * dx + dy * dy;
	};

	return Array.from({ length: acres }, (_, i) => i).sort(
		(a, b) => near(a) - near(b) || a - b,
	);
}

/** Acres to a field. Bigger than one, so a holding is a patchwork of fields
    rather than a different crop on every acre. */
const FIELD = 2;

/**
 * Which of the three crops the acre at x,y is under. Deterministic and shared,
 * so the map and the card farm the same land the same way.
 */
export function fieldOf(id, x, y) {
	return hash(`${id}:${Math.floor(x / FIELD)}:${Math.floor(y / FIELD)}`) % 3;
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
