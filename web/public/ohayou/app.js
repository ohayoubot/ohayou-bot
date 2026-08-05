/*
 * The world map. Every plot is a patch of land, sized by how many acres its
 * owner holds and coloured by what is on it.
 *
 * The layout is decorative: which acre a building sits on is this page's
 * choice, not the game's. What is authoritative is the count beside it, which
 * comes from the bot. Nothing here works out a game rule.
 */

const $ = (sel) => document.querySelector(sel);

async function start() {
	const [world, session] = await Promise.all([load("/ohayou/api/world"), me()]);
	if (!world) {
		$("#totals").textContent = "The map is not answering just now.";
		return;
	}

	const { plots, totals, updated } = world;
	$("#totals").replaceChildren(...describe(totals, updated));

	if (plots.length === 0) {
		$("#empty").hidden = false;
		return;
	}
	$("#world").replaceChildren(
		...plots.map((plot) => card(plot, session?.account)),
	);
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

function card(plot, account) {
	const el = document.createElement("article");
	el.className = "plot";
	if (!plot.named) el.classList.add("unnamed");
	if (account && plot.named && plot.id === account) el.classList.add("mine");

	const head = document.createElement("header");
	const name = document.createElement("h3");
	name.textContent = plot.named ? plot.nick : "unclaimed";
	const band = document.createElement("span");
	band.className = "band";
	band.textContent = plot.wealth;
	head.append(name, band);
	el.append(head, acres(plot));

	if (plot.named) {
		const list = legend(plot.land);
		if (list) el.append(list);
	}
	return el;
}

/**
 * One tile per acre, filled with whatever is on the plot. Buildings are dealt
 * out in name order so the same holding always draws the same way; a plot with
 * more buildings than acres simply fills every tile.
 */
function acres(plot) {
	const grid = document.createElement("div");
	grid.className = "acres";

	const fill = [];
	for (const [item, count] of Object.entries(plot.land).sort()) {
		for (let i = 0; i < count && fill.length < plot.acres; i++) {
			fill.push(item);
		}
	}

	for (let i = 0; i < plot.acres; i++) {
		const tile = document.createElement("div");
		tile.className = "acre";
		if (fill[i]) {
			tile.classList.add("built");
			tile.style.setProperty("--hue", hueOf(fill[i]));
			tile.title = fill[i];
		}
		grid.append(tile);
	}
	return grid;
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

/**
 * A hue per item name. Derived rather than listed, so an item added to the
 * game gets a colour without this page being taught its name. The lightness
 * and chroma live in the stylesheet, which is what keeps a hundred hues
 * looking like one palette.
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
