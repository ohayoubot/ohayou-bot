/*
 * The world map: every plot as a block of acre tiles, packed into one grid so
 * the page reads as ground rather than a list. Where a block lands is the
 * browser's business; its size and contents come from the bot.
 */

import { hexOf, normalise, TRANSPARENT } from "../deerkins/kins.js";

import { hueOf, layout } from "./plot.js";

const $ = (sel) => document.querySelector(sel);

let plots = [];
let flags = {};
let mine = null;
/** The countdown interval, cleared before a new one replaces it. */
let ticking = null;

async function start() {
	const [world, session] = await Promise.all([load("/ohayou/api/world"), me()]);
	if (!world) {
		$("#totals").textContent = "The map is not answering just now.";
		return;
	}

	plots = world.plots;
	flags = world.flags ?? {};
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

/** The private tier: a different endpoint, with a different rule. */
async function standing() {
	const yours = await load("/ohayou/api/me");
	if (!yours) return;

	if (yours.status === "unclaimed") {
		// On the map but unnamed: the claim section already explains how.
		return;
	}

	const panels = [
		controls(yours),
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

/**
 * Queues a request. Nothing here changes the game: the bot polls, applies it,
 * and the map redraws when it next publishes.
 */
function controls(yours) {
	const flag = document.createElement("input");
	flag.type = "text";
	flag.maxLength = 48;
	flag.placeholder = "a deer's name";
	flag.value = plots.find((p) => p.named && p.id === yours.account)?.flag ?? "";
	flag.setAttribute("aria-label", "the deer to fly over your plot");

	const say = document.createElement("p");
	say.className = "hint";

	const send = async (kind, value) => {
		say.textContent = "Asking…";
		const res = await fetch("/ohayou/api/command", {
			method: "POST",
			headers: { "content-type": "application/json" },
			body: JSON.stringify({ kind, value }),
		}).catch(() => null);

		say.textContent = res?.ok
			? "Asked. The bot will pick it up in a moment."
			: "The bot did not take that.";
	};

	const fly = document.createElement("button");
	fly.type = "button";
	fly.textContent = "Fly it";
	fly.addEventListener("click", () => send("flag", flag.value.trim()));

	const row = document.createElement("div");
	row.className = "ask";
	row.append(flag, fly);

	const down = document.createElement("button");
	down.type = "button";
	down.className = "quiet";
	down.textContent = "Take my name off the map";
	down.addEventListener("click", () => send("territory", "off"));

	return panel("Your flag", row, down, say);
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

/** Counts down from the due time, so a four-hour run ticks without polling. */
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

/** Display names for the activities, falling back to the bot's own word. */
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

/** Only to know which plot is yours; a signed-out visitor sees the same map. */
async function me() {
	return load("/api/session");
}

/* ---- the map ---- */

/** A plot as a block of acre tiles, sized by the holding. */
function block(plot) {
	const { acres, wide, tall, tiles } = layout(plot);

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

	// wide by tall is square, so the last row may run past the acreage. Those
	// cells are not land and are left out.
	for (let i = 0; i < wide * tall; i++) {
		if (i >= acres) break;
		const tile = document.createElement("span");
		tile.className = tiles[i] ? "acre built" : "acre";
		if (tiles[i]) tile.style.setProperty("--hue", hueOf(tiles[i]));
		el.append(tile);
	}

	const point = () => show(plot);
	el.addEventListener("mouseenter", point);
	el.addEventListener("focus", point);
	el.addEventListener("click", point);
	return el;
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

	const parts = [name];
	const banner = flag(plot.flag);
	if (banner) parts.push(banner);
	parts.push(facts);
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

/**
 * A plot's deer, drawn from the gallery's own kinskode: one character per cell,
 * in the sixteen colours IRC has. This is the one place the map borrows the
 * gallery's palette, because it is the gallery's picture.
 */
function flag(name) {
	const code = name && flags[name];
	if (!code) return null;

	const rows = normalise(code).split("\n");
	const wide = Math.max(...rows.map((r) => r.length));

	const art = document.createElement("div");
	art.className = "banner";
	art.style.setProperty("--cols", wide);
	art.title = name;
	art.setAttribute("role", "img");
	art.setAttribute("aria-label", `the deer named ${name}`);

	for (const row of rows) {
		for (let x = 0; x < wide; x++) {
			const cell = document.createElement("span");
			const char = row[x] ?? TRANSPARENT;
			const hex = char === TRANSPARENT ? null : hexOf(char);
			if (hex) cell.style.background = hex;
			art.append(cell);
		}
	}

	const wrap = document.createElement("figure");
	wrap.className = "flag";
	const caption = document.createElement("figcaption");
	caption.textContent = name;
	wrap.append(art, caption);
	return wrap;
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

start();
