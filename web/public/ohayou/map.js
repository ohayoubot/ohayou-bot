/*
 * The world, drawn as one landmass.
 *
 * Svg in tile units: one unit is one acre, everywhere, which is what makes the
 * map comparable rather than decorative. Ground is emitted as merged runs of
 * tiles; every building is one <image> pointing at a cached sprite url, so a
 * thousand acres cost a thousand nodes rather than a hundred thousand.
 */

import { normalise, toDataURL } from "../deerkins/kins.js";
import { VERGE, worldLayout } from "./plot.js";
import { spriteURL } from "./sprites.js";

const SVG = "http://www.w3.org/2000/svg";

/** Sea around the coast, in tiles. */
const MARGIN = 3;

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

	const pasture = el("pattern", {
		id: "pasture",
		width: 2,
		height: 2,
		patternUnits: "userSpaceOnUse",
	});
	pasture.append(el("rect", { width: 2, height: 2, class: "graze" }));
	pasture.append(el("path", { d: "M.4 1.5 v-.4 M1.5 .7 v-.4", class: "tuft" }));
	defs.append(pasture);

	return defs;
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

	g.append(
		el("rect", {
			class: "field",
			x: 0,
			y: 0,
			width: w,
			height: h,
			rx: 0.2,
			fill: plot.named ? "url(#pasture)" : null,
		}),
	);

	tiles.forEach((tile, i) => {
		if (!tile || i >= acres) return;
		const cx = i % w;
		const cy = Math.floor(i / w);
		g.append(
			el("rect", { class: "worked", x: cx, y: cy, width: 1, height: 1 }),
		);
		for (const sprite of cluster(tile, cx, cy)) g.append(sprite);
	});

	// After the buildings: a boundary a sprite can overrun is not a boundary.
	g.append(
		el("rect", { class: "hedge", x: 0, y: 0, width: w, height: h, rx: 0.2 }),
	);
	if (plot.named) {
		g.append(
			el("rect", { class: "strip", x: 0, y: h - 0.3, width: w, height: 0.3 }),
		);
	}

	const banner = flag(plot, flags, w);
	if (banner) g.append(banner);

	if (plot.named) {
		const label = el("text", { class: "nameplate", x: w / 2, y: h + 0.95 });
		label.textContent = plot.nick;
		g.append(label);
	}

	const pick = () => onPick?.(plot);
	g.addEventListener("pointerenter", pick);
	g.addEventListener("focus", pick);
	g.addEventListener("click", pick);
	return g;
}

/**
 * The buildings on one acre. One of a thing fills the acre; several share it.
 * Never smaller than a quarter tile: past that the sprite stops being one.
 */
function cluster(tile, cx, cy) {
	const side = tile.n === 1 ? 1 : 2;
	const size = 1 / side;
	const href = spriteURL(tile.item);
	const out = [];

	for (let i = 0; i < Math.min(tile.n, side * side); i++) {
		out.push(
			el("image", {
				href,
				x: (cx + (i % side) * size).toFixed(3),
				y: (cy + Math.floor(i / side) * size).toFixed(3),
				width: size,
				height: size,
			}),
		);
	}
	return out;
}

/** The plot's deer, on a pole at the near corner. */
function flag(plot, flags, w) {
	const code = plot.named && plot.flag && flags[plot.flag];
	if (!code) return null;

	const g = el("g", {
		class: "flagpole",
		transform: `translate(${w - 0.5} -2.6)`,
	});
	g.append(
		el("rect", { class: "pole", x: -0.08, y: 0, width: 0.16, height: 3.1 }),
	);
	g.append(
		el("image", {
			href: toDataURL(normalise(code), `flag:${plot.flag}`),
			x: -2.1,
			y: 0.1,
			width: 2,
			height: 2,
			preserveAspectRatio: "xMidYMid meet",
		}),
	);
	return g;
}

function describe(plot, acres) {
	const who = plot.named ? plot.nick : "an unclaimed plot";
	const size = `${acres} ${acres === 1 ? "acre" : "acres"}`;
	return plot.named ? `${who}, ${size}, ${plot.wealth}` : `${who}, ${size}`;
}
