/*
 * The world, drawn as one landmass.
 *
 * Svg in tile units: one unit is one acre, everywhere, which is what makes the
 * map comparable rather than decorative. Ground is emitted as merged runs of
 * tiles; every building is one <image> pointing at a cached sprite url, so a
 * thousand acres cost a thousand nodes rather than a hundred thousand.
 */

import { normalise, toDataURL } from "../deerkins/kins.js";
import { groundFor } from "./catalog.js";
import { fieldOf, VERGE, worldLayout } from "./plot.js";
import { spriteURL } from "./sprites.js";

const SVG = "http://www.w3.org/2000/svg";

/** What a parcel is called when its holder asked not to be named. */
export const ANONYMOUS = "Anonymous";

/** Sea around the coast, in tiles. */
const MARGIN = 3;

/**
 * Nameplate height in tiles. Set here rather than in css because fit() below
 * measures against it: a label sized by one and clipped by the other would
 * overrun its neighbours.
 */
const NAMEPLATE = 0.55;

const el = (name, attrs = {}) => {
	const node = document.createElementNS(SVG, name);
	for (const [k, v] of Object.entries(attrs)) node.setAttribute(k, v);
	return node;
};

/**
 * Draws plots and returns { svg, focus }, where focus(id) centres a plot and
 * marks it. onPick is called with a plot when one is pointed at or focused.
 */
export function drawWorld(plots, { flags = {}, mine = null, onPick } = {}) {
	const world = worldLayout(plots);
	const w = world.width + MARGIN * 2;
	const h = world.height + MARGIN * 2;

	const svg = el("svg", {
		viewBox: `0 0 ${w} ${h}`,
		class: "worldmap",
		role: "list",
		"aria-label": "Every territory",
		preserveAspectRatio: "xMidYMid meet",
	});
	svg.append(
		defs(),
		el("rect", { x: 0, y: 0, width: w, height: h, fill: "url(#sea)" }),
	);

	const land = ground(world, MARGIN);
	svg.append(land);

	const byID = new Map();
	for (const parcel of world.parcels) {
		const node = drawParcel(parcel, { flags, mine, onPick, offset: MARGIN });
		byID.set(parcel.plot.id, node);
		svg.append(node);
	}

	return {
		svg,
		focus(id) {
			const node = byID.get(id);
			if (!node) return false;
			for (const other of byID.values()) other.classList.remove("picked");
			node.classList.add("picked");
			node.scrollIntoView({ block: "center", inline: "center" });
			return true;
		},
	};
}

function defs() {
	const defs = el("defs");

	const sea = el("pattern", {
		id: "sea",
		width: 4,
		height: 4,
		patternUnits: "userSpaceOnUse",
	});
	sea.append(el("rect", { width: 4, height: 4, class: "sea" }));
	sea.append(el("path", { d: "M0 2.5 q1 -.6 2 0 t2 0", class: "swell" }));
	defs.append(sea);

	const grass = el("pattern", {
		id: "grass",
		width: 3,
		height: 3,
		patternUnits: "userSpaceOnUse",
	});
	grass.append(el("rect", { width: 3, height: 3, class: "turf" }));
	grass.append(
		el("path", { d: "M.6 2.2 v-.5 M2.2 1 v-.5 M1.4 2.6 v-.4", class: "tuft" }),
	);
	defs.append(grass);

	// Most parcels are unfiled, so bare ground is the common case and a flat
	// rectangle of it is most of the map.
	const fallow = el("pattern", {
		id: "fallow",
		width: 2,
		height: 2,
		patternUnits: "userSpaceOnUse",
	});
	fallow.append(el("rect", { width: 2, height: 2, class: "bare" }));
	fallow.append(
		el("path", { d: "M.5 1.4 h.3 M1.4 .5 h.3 M1.1 1.7 h.2", class: "clod" }),
	);
	defs.append(fallow);

	const pasture = el("pattern", {
		id: "pasture",
		width: 2,
		height: 2,
		patternUnits: "userSpaceOnUse",
	});
	pasture.append(el("rect", { width: 2, height: 2, class: "graze" }));
	pasture.append(el("path", { d: "M.4 1.5 v-.4 M1.5 .7 v-.4", class: "tuft" }));
	defs.append(pasture);

	// The three crops. A parcel of any size is a patchwork of them, which is
	// what stops a large holding being one flat green rectangle. Each is one
	// acre so a run of acres tiles it seamlessly, and the rows are drawn in the
	// pattern rather than per acre: a four hundred acre plot costs the same as
	// a one acre one.
	defs.append(
		crop("crop-a", "M.18 0 v1 M.5 0 v1 M.82 0 v1"),
		crop("crop-b", "M0 .18 h1 M0 .5 h1 M0 .82 h1"),
		crop("crop-c", "M.25 .25 h.1 M.7 .4 h.1 M.4 .78 h.1 M.12 .62 h.1"),
	);

	for (const name of ["pen", "yard", "spoil"]) defs.append(worked(name));

	return defs;
}

/** One acre of a crop: a ground colour and the rows worked into it. */
function crop(name, rows) {
	const pattern = el("pattern", {
		id: name,
		width: 1,
		height: 1,
		patternUnits: "userSpaceOnUse",
	});
	pattern.append(el("rect", { width: 1, height: 1, class: `ground ${name}` }));
	pattern.append(el("path", { d: rows, class: `furrow ${name}` }));
	return pattern;
}

/** The ground a building stands on: hardstanding, a pen, or spoil. */
function worked(name) {
	const pattern = el("pattern", {
		id: name,
		width: 1,
		height: 1,
		patternUnits: "userSpaceOnUse",
	});
	pattern.append(el("rect", { width: 1, height: 1, class: `ground ${name}` }));
	pattern.append(
		el("path", {
			d: "M.15 .2 h.12 M.62 .35 h.12 M.35 .72 h.12 M.8 .8 h.1",
			class: `grit ${name}`,
		}),
	);
	return pattern;
}

/**
 * The landmass: every tile within VERGE of a parcel, a jittered fringe so the
 * coast is not a rectangle, and no inland sea where the packer left a gap.
 * Merged along rows into as few rects as it can.
 */
function ground(world, offset) {
	const cells = new Set();
	const key = (x, y) => `${x},${y}`;

	for (const p of world.parcels) {
		for (let y = p.y - VERGE; y < p.y + p.h + VERGE; y++) {
			for (let x = p.x - VERGE; x < p.x + p.w + VERGE; x++)
				cells.add(key(x, y));
		}
	}

	// A deterministic fringe: the same world always has the same coastline.
	for (const cell of [...cells]) {
		const [x, y] = cell.split(",").map(Number);
		for (const [dx, dy] of [
			[1, 0],
			[-1, 0],
			[0, 1],
			[0, -1],
		]) {
			if (noise(x + dx, y + dy) > 0.45) cells.add(key(x + dx, y + dy));
		}
	}

	fillLakes(cells, key);

	const g = el("g", { class: "land" });
	const rows = new Map();
	for (const cell of cells) {
		const [x, y] = cell.split(",").map(Number);
		if (!rows.has(y)) rows.set(y, []);
		rows.get(y).push(x);
	}

	for (const [y, xs] of [...rows].sort((a, b) => a[0] - b[0])) {
		xs.sort((a, b) => a - b);
		let start = xs[0];
		let last = xs[0];
		const flush = () => {
			g.append(
				el("rect", {
					x: start + offset,
					y: y + offset,
					width: last - start + 1,
					height: 1,
					fill: "url(#grass)",
				}),
			);
		};
		for (const x of xs.slice(1)) {
			if (x === last + 1) {
				last = x;
				continue;
			}
			flush();
			start = x;
			last = x;
		}
		flush();
	}
	return g;
}

/**
 * Turns any water the packer enclosed into land. Flood fills the sea from
 * outside the bounding box; whatever it never reached was a hole.
 */
function fillLakes(cells, key) {
	let minX = Number.POSITIVE_INFINITY;
	let minY = Number.POSITIVE_INFINITY;
	let maxX = Number.NEGATIVE_INFINITY;
	let maxY = Number.NEGATIVE_INFINITY;
	for (const cell of cells) {
		const [x, y] = cell.split(",").map(Number);
		minX = Math.min(minX, x);
		minY = Math.min(minY, y);
		maxX = Math.max(maxX, x);
		maxY = Math.max(maxY, y);
	}
	if (!Number.isFinite(minX)) return;

	const sea = new Set();
	const queue = [[minX - 1, minY - 1]];
	while (queue.length) {
		const [x, y] = queue.pop();
		if (x < minX - 1 || x > maxX + 1 || y < minY - 1 || y > maxY + 1) continue;
		const k = key(x, y);
		if (sea.has(k) || cells.has(k)) continue;
		sea.add(k);
		queue.push([x + 1, y], [x - 1, y], [x, y + 1], [x, y - 1]);
	}

	for (let y = minY; y <= maxY; y++) {
		for (let x = minX; x <= maxX; x++) {
			const k = key(x, y);
			if (!sea.has(k)) cells.add(k);
		}
	}
}

/** Stable value in [0,1) for a tile. */
function noise(x, y) {
	let n = (x * 73856093) ^ (y * 19349663);
	n = (n ^ (n >>> 13)) >>> 0;
	return ((n * 1274126177) >>> 0) / 4294967296;
}

function drawParcel(parcel, { flags, mine, onPick, offset }) {
	const { plot, tiles, w, h, acres } = parcel;
	const x = parcel.x + offset;
	const y = parcel.y + offset;

	const g = el("g", {
		class: "parcel",
		tabindex: 0,
		role: "listitem",
		transform: `translate(${x} ${y})`,
	});
	g.classList.toggle("unnamed", !plot.named);
	g.classList.toggle("mine", Boolean(mine) && plot.named && plot.id === mine);
	g.setAttribute("aria-label", describe(plot, acres));
	if (plot.named) g.dataset.band = plot.wealth;

	// The fence is drawn against the parcel rather than against the map: a
	// stroke that reads as a hedge around a hundred acres is a stripe across a
	// single one. Everything else that outlines the parcel follows it.
	g.style.setProperty("--fence", fence(w, h));

	// Bare ground under everything, so a partial last row is not a hole.
	g.append(
		el("rect", {
			class: "field",
			x: 0,
			y: 0,
			width: w,
			height: h,
			fill: plot.named ? "url(#pasture)" : "url(#fallow)",
		}),
	);

	// A parcel nobody named is left as it is: bare acreage, and no reading of
	// what is on it. Only a named one is farmed.
	if (plot.named)
		for (const patch of fields(plot, tiles, w, acres)) g.append(patch);

	tiles.forEach((tile, i) => {
		if (!tile || i >= acres) return;
		const cx = i % w;
		const cy = Math.floor(i / w);
		g.append(
			el("rect", {
				class: "worked",
				x: cx,
				y: cy,
				width: 1,
				height: 1,
				fill: `url(#${groundFor(tile.item)})`,
			}),
		);
		for (const sprite of cluster(tile, cx, cy)) g.append(sprite);
	});

	// Outside the boundary rather than astride it: half a stroke over a hundred
	// acres is nothing, over a single one it is a lid. After the buildings,
	// since a boundary a sprite can overrun is not a boundary.
	const thick = fence(w, h);
	g.append(
		el("rect", {
			class: "hedge",
			x: -thick / 2,
			y: -thick / 2,
			width: w + thick,
			height: h + thick,
		}),
	);

	// Wealth is the strip along the foot, which the key repeats. Sized against
	// the parcel like the fence: a fixed strip is a third of a single acre and
	// a hairline across four hundred.
	if (plot.named) {
		const deep = thick * 1.7;
		g.append(
			el("rect", { class: "strip", x: 0, y: h - deep, width: w, height: deep }),
		);
	}

	const banner = flag(plot, flags, w, h);
	if (banner) g.append(banner);

	{
		const label = el("text", {
			class: "nameplate",
			x: w / 2,
			y: h + 0.8,
			"font-size": NAMEPLATE,
		});
		label.textContent = fit(plot.named ? plot.nick : ANONYMOUS, w + VERGE * 2);
		g.append(label);
	}

	const pick = () => onPick?.(plot);
	g.addEventListener("pointerenter", pick);
	g.addEventListener("focus", pick);
	g.addEventListener("click", pick);
	return g;
}

/** How thick a parcel's boundary is, in tiles: a share of its short side, and
    never so thin it vanishes nor so thick it frames a single acre. */
function fence(w, h) {
	return Math.min(0.18, Math.max(0.04, Math.min(w, h) * 0.04));
}

/**
 * The unworked acres, as merged runs of the crop each is under. Runs rather
 * than one rect per acre: the pattern already draws the rows, so a four hundred
 * acre plot costs a handful of nodes.
 */
function fields(plot, tiles, w, acres) {
	const out = [];
	const rows = Math.ceil(acres / w);

	for (let y = 0; y < rows; y++) {
		let from = 0;
		let crop = -1;

		const flush = (to) => {
			if (crop < 0) return;
			out.push(
				el("rect", {
					class: "crop",
					x: from,
					y,
					width: to - from,
					height: 1,
					fill: `url(#crop-${"abc"[crop]})`,
				}),
			);
		};

		for (let x = 0; x <= w; x++) {
			const i = y * w + x;
			// Past the acreage, or built on: either way the run ends here.
			const under =
				x < w && i < acres && !tiles[i] ? fieldOf(plot.id, x, y) : -1;
			if (under === crop) continue;
			flush(x);
			from = x;
			crop = under;
		}
	}
	return out;
}

/**
 * The things on one acre. One of a thing fills the acre; a handful share it,
 * and a full acre is drawn three by three. Never smaller than a third of a
 * tile: past that the sprite stops being one, so a denser acre reads as a full
 * one rather than as a smaller crowd.
 */
function cluster(tile, cx, cy) {
	const side = tile.n === 1 ? 1 : tile.n <= 4 ? 2 : 3;
	const href = spriteURL(tile.item);

	// Scaled by the group rather than by each image: an <image> given a
	// fractional width is one Chromium declines to raster at all.
	const g = el("g", {
		class: "stand",
		transform: `translate(${cx} ${cy}) scale(${(1 / side).toFixed(4)})`,
	});
	for (let i = 0; i < Math.min(tile.n, side * side); i++) {
		g.append(
			el("image", {
				href,
				x: i % side,
				y: Math.floor(i / side),
				width: 1,
				height: 1,
			}),
		);
	}
	return [g];
}

/**
 * The plot's deer, on a pole inside the parcel's top corner. Sized against the
 * parcel: a two tile banner over a one acre holding is a banner with a holding
 * under it.
 */
function flag(plot, flags, w, h) {
	const code = plot.named && plot.flag && flags[plot.flag];
	if (!code) return null;

	const size = Math.min(1.8, Math.max(0.9, Math.min(w, h) * 0.7));
	const pole = size * 1.25;

	const g = el("g", {
		class: "flagpole",
		transform: `translate(${w - 0.15} ${-pole})`,
	});
	g.append(
		el("rect", {
			class: "pole",
			x: -size * 0.05,
			y: 0,
			width: size * 0.1,
			height: pole + 0.1,
		}),
	);
	// Backed, because a deer is drawn in whatever sixteen colours its author
	// chose and some of them are the colour of the sea behind it.
	g.append(
		el("rect", {
			class: "banner",
			x: -size,
			y: 0.05,
			width: size,
			height: size,
		}),
	);
	g.append(
		el("image", {
			href: toDataURL(normalise(code), `flag:${plot.flag}`),
			x: -size,
			y: 0.05,
			width: size,
			height: size,
			preserveAspectRatio: "xMidYMid meet",
		}),
	);
	return g;
}

/**
 * A nick clipped to the tiles it has to sit in, verges included. Most parcels
 * are one acre wide and most nicks are not, so without this every label runs
 * over its neighbours.
 */
function fit(nick, tiles) {
	// Monospace advance is about 0.6em.
	const room = Math.max(3, Math.floor(tiles / (NAMEPLATE * 0.6)));
	return nick.length > room ? `${nick.slice(0, room - 1)}\u2026` : nick;
}

function describe(plot, acres) {
	const who = plot.named ? plot.nick : ANONYMOUS;
	const size = `${acres} ${acres === 1 ? "acre" : "acres"}`;
	return plot.named ? `${who}, ${size}, ${plot.wealth}` : `${who}, ${size}`;
}
