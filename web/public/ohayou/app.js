/*
 * The world, as one landscape rather than a row of cards.
 *
 * Each plot is a block of acre tiles, and the blocks are packed together so the
 * map reads as settled ground: a big holding is visibly bigger, and small ones
 * fill in around it. Where a plot lands is the browser's business (grid packing
 * does it), but its size and what is on it come from the bot.
 *
 * Nothing here works out a game rule. Which acre a building sits on is this
 * page's choice; the counts beside it are authoritative.
 */

const $ = (sel) => document.querySelector(sel);

let plots = [];
let mine = null;
/** Set when a countdown is on screen, so it can tick without a reload. */
let ticking = null;

async function start() {
	const [world, session] = await Promise.all([load("/ohayou/api/world"), me()]);
	if (!world) {
		$("#totals").textContent = "The map is not answering just now.";
		return;
	}

	plots = world.plots;
	mine = session?.account ?? null;
	$("#totals").replaceChildren(...describe(world.totals, world.updated));

	if (plots.length === 0) {
		$("#empty").hidden = false;
		$("#detail").hidden = true;
		return;
	}

	$("#world").replaceChildren(...plots.map(block));
	const own = plots.find((p) => p.named && p.id === mine);
	show(own ?? plots[0]);

	if (session) await standing();
}

/* ---- your own standing ---- */

/**
 * What only you may see: the exact figures, and what is still counting down.
 * Served from a different endpoint with a different rule, so nothing here can
 * be reached by pointing at somebody else's plot.
 */
async function standing() {
	const yours = await load("/ohayou/api/me");
	if (!yours) return;

	if (yours.status === "unclaimed") {
		// On the map but unnamed: the claim section already explains how.
		return;
	}

	const panels = [
		figure("Ohayous", yours.ohayous, `${yours.cumulative} earned in all`),
		yours.vault && vault(yours.vault),
		figure("Defence", yours.defense, describeArmour(yours.equipped)),
		counts("Metals", yours.metals),
		counts("Items", yours.items),
		running(yours.running),
		probation(yours.probation),
	].filter(Boolean);

	$("#yours").replaceChildren(...panels);
	$("#standing").hidden = false;
	$("#claim").hidden = true;
}

function panel(title, ...body) {
	const el = document.createElement("article");
	el.className = "panel";
	const head = document.createElement("h3");
	head.textContent = title;
	el.append(head, ...body);
	return el;
}

function figure(title, value, sub) {
	const n = document.createElement("div");
	n.className = "figure";
	n.textContent = value;

	const parts = [n];
	if (sub) {
		const s = document.createElement("div");
		s.className = "sub";
		s.textContent = sub;
		parts.push(s);
	}
	return panel(title, ...parts);
}

function vault(v) {
	const el = figure(`Vault, level ${v.level}`, v.ohayous, `of ${v.cap}`);
	const meter = document.createElement("div");
	meter.className = "meter";
	const bar = document.createElement("span");
	bar.style.width = `${Math.min(100, (v.ohayous / v.cap) * 100)}%`;
	meter.append(bar);
	el.append(meter);
	return el;
}

function describeArmour(equipped) {
	const worn = Object.values(equipped);
	return worn.length ? worn.sort().join(", ") : "nothing equipped";
}

function counts(title, held) {
	const names = Object.keys(held).sort();
	if (names.length === 0) return null;

	const list = document.createElement("ul");
	list.className = "rows";
	list.replaceChildren(
		...names.map((name) => {
			const row = document.createElement("li");
			const n = document.createElement("b");
			n.textContent = held[name];
			row.append(name, n);
			return row;
		}),
	);
	return panel(title, list);
}

/** The thing the site can tell you that a channel line cannot: how long left. */
function running(runs) {
	if (!runs || runs.length === 0) return null;

	const list = document.createElement("ul");
	list.className = "rows";
	const tick = () => {
		list.replaceChildren(
			...runs.map((run) => {
				const row = document.createElement("li");
				const left = document.createElement("b");
				left.className = "countdown";
				left.textContent = until(run.due * 1000 - Date.now());
				row.append(labelOf(run.kind), left);
				return row;
			}),
		);
	};
	tick();

	clearInterval(ticking);
	ticking = setInterval(tick, 1000);
	return panel("Running", list);
}

function probation(at) {
	if (!at || at * 1000 <= Date.now()) return null;

	const el = figure(
		"On probation",
		until(at * 1000 - Date.now()),
		"left to serve",
	);
	el.classList.add("warn");
	return el;
}

/** Friendly names for the activities. Falls back to the bot's own word, so an
    activity added to the game still reads as something. */
function labelOf(kind) {
	return (
		{ mining: "quarry", pumping: "oilwell", breeding: "cattery" }[kind] ?? kind
	);
}

function until(ms) {
	if (ms <= 0) return "done";
	const seconds = Math.floor(ms / 1000);
	const hours = Math.floor(seconds / 3600);
	const minutes = Math.floor((seconds % 3600) / 60);
	if (hours > 0) return `${hours}h ${String(minutes).padStart(2, "0")}m`;
	if (minutes > 0)
		return `${minutes}m ${String(seconds % 60).padStart(2, "0")}s`;
	return `${seconds}s`;
}

async function load(url) {
	try {
		const res = await fetch(url);
		return res.ok ? await res.json() : null;
	} catch {
		return null;
	}
}

/** Only to know which plot is yours. A signed-out visitor sees the same map. */
async function me() {
	return load("/api/session");
}

/* ---- the map ---- */

/**
 * A plot's block is as square as its acreage allows, so the map is made of
 * settlements rather than stripes. Grid packing places them; a dense flow
 * lets a small plot fill a gap a large one left.
 */
function block(plot) {
	const acres = Math.max(1, plot.acres);
	const wide = Math.ceil(Math.sqrt(acres));
	const tall = Math.ceil(acres / wide);

	const el = document.createElement("button");
	el.type = "button";
	el.className = "plot";
	el.setAttribute("role", "listitem");
	if (!plot.named) el.classList.add("unnamed");
	if (mine && plot.named && plot.id === mine) el.classList.add("mine");

	el.style.setProperty("--wide", wide);
	el.style.setProperty("--tall", tall);
	el.setAttribute(
		"aria-label",
		`${plot.named ? plot.nick : "an unclaimed plot"}, ${acres} ${plural(
			acres,
			"acre",
		)}, ${plot.wealth}`,
	);

	// The tiles themselves: filled ones first, so a holding reads as built up
	// from one corner rather than speckled.
	const filled = fill(plot, wide * tall);
	for (const item of filled) {
		const tile = document.createElement("span");
		tile.className = item ? "acre built" : "acre";
		if (item) tile.style.setProperty("--hue", hueOf(item));
		el.append(tile);
	}

	const point = () => show(plot);
	el.addEventListener("mouseenter", point);
	el.addEventListener("focus", point);
	el.addEventListener("click", point);
	return el;
}

/**
 * What sits on each tile of the block. Buildings are dealt out in name order so
 * the same holding always draws the same way; tiles past the plot's acreage are
 * the corner the square leaves over, and stay blank.
 */
function fill(plot, tiles) {
	const out = [];
	for (const [item, count] of Object.entries(plot.land).sort()) {
		for (let i = 0; i < count && out.length < plot.acres; i++) out.push(item);
	}
	while (out.length < plot.acres) out.push(null);
	while (out.length < tiles) out.push(undefined);
	return out.slice(0, tiles);
}

/* ---- who a plot belongs to ---- */

function show(plot) {
	const panel = $("#detail");
	const name = document.createElement("h3");
	name.textContent = plot.named ? plot.nick : "An unclaimed plot";
	if (!plot.named) name.className = "anon";

	const facts = document.createElement("p");
	facts.className = "facts";
	facts.textContent = `${plot.acres} ${plural(plot.acres, "acre")} · ${
		plot.wealth
	} · ${plot.rations} ${plural(plot.rations, "ration")} collected`;

	const parts = [name, facts];
	if (plot.named) {
		const list = legend(plot.land);
		parts.push(list ?? hint("Nothing built on it yet."));
	} else {
		parts.push(
			hint(
				"Whoever holds this has not put their name to it. Say !territory on to name yours.",
			),
		);
	}
	panel.replaceChildren(...parts);
}

function hint(text) {
	const p = document.createElement("p");
	p.className = "hint";
	p.textContent = text;
	return p;
}

function legend(land) {
	const names = Object.keys(land).sort();
	if (names.length === 0) return null;

	const list = document.createElement("ul");
	list.className = "legend";
	list.replaceChildren(
		...names.map((item) => {
			const row = document.createElement("li");
			const swatch = document.createElement("span");
			swatch.className = "swatch";
			swatch.style.setProperty("--hue", hueOf(item));

			const count = document.createElement("b");
			count.textContent = land[item];
			row.append(swatch, `${item} `, count);
			return row;
		}),
	);
	return list;
}

/* ---- odds and ends ---- */

function describe(totals, updated) {
	const stat = (value, label) => {
		const el = document.createElement("span");
		el.className = "stat";
		const n = document.createElement("b");
		n.textContent = value;
		el.append(n, ` ${label}`);
		return el;
	};

	const when = document.createElement("span");
	when.className = "when";
	when.textContent = updated
		? `published ${ago(Date.now() - updated)}`
		: "nothing published yet";

	return [
		stat(totals.players, plural(totals.players, "player")),
		stat(totals.acres, plural(totals.acres, "acre")),
		stat(totals.named, "named"),
		when,
	];
}

function plural(n, word) {
	return n === 1 ? word : `${word}s`;
}

function ago(ms) {
	const minutes = Math.floor(ms / 60000);
	if (minutes < 1) return "just now";
	if (minutes < 60) return `${minutes} ${plural(minutes, "minute")} ago`;
	const hours = Math.floor(minutes / 60);
	if (hours < 24) return `${hours} ${plural(hours, "hour")} ago`;
	const days = Math.floor(hours / 24);
	return `${days} ${plural(days, "day")} ago`;
}

/**
 * A hue per item name. Derived rather than listed, so an item added to the game
 * gets a colour without this page being taught its name. The lightness and
 * chroma live in the stylesheet, which is what keeps a hundred hues looking
 * like one palette.
 */
function hueOf(item) {
	let hash = 0;
	for (let i = 0; i < item.length; i++) {
		hash = (hash * 31 + item.charCodeAt(i)) >>> 0;
	}
	// Spread by the golden angle so neighbouring names are not neighbouring
	// colours, and adjacent tiles stay told apart.
	return (hash * 137.508) % 360;
}

start();
