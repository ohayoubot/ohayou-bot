/*
 * The registry page: draw the world, read a deed when one is pointed at, and
 * fill in the signed-in file from the private endpoint.
 */

import { normalise, toDataURL } from "../deerkins/kins.js";
import { nav } from "../nav.js";
import { nameOf } from "./catalog.js";
import { drawWorld } from "./map.js";
import { usage } from "./plot.js";
import { spriteURL } from "./sprites.js";
import { BANDS } from "./terrain.js";

const $ = (sel) => document.querySelector(sel);

let flags = {};
let mine = null;
let ticking = null;

async function start() {
	nav("#sitenav", "/ohayou/");
	key();

	const [world, session] = await Promise.all([load("/ohayou/api/world"), me()]);
	if (!world) {
		$("#loading").textContent = "The registry is not answering just now.";
		return;
	}

	flags = world.flags ?? {};
	mine = session?.account ?? null;
	ledger(world.totals, world.updated);

	if (world.plots.length === 0) {
		$("#loading").textContent = "Nothing has been claimed yet.";
		return;
	}

	const { svg, focus } = drawWorld(world.plots, {
		flags,
		mine,
		onPick: deed,
	});
	$("#mapscroll").replaceChildren(svg);
	zoom(svg);

	const own = world.plots.find((p) => p.named && p.id === mine);
	deed(own ?? world.plots[0]);
	if (own) {
		const find = $("#findme");
		find.hidden = false;
		find.addEventListener("click", () => focus(own.id));
		focus(own.id);
	}

	if (session) {
		$("#signedout").hidden = true;
		await file(own);
	}
}

/**
 * Scales the map inside its scroller. A world of a hundred parcels is a texture
 * at a fit width; the buildings are only legible closer in.
 */
function zoom(svg) {
	const steps = [1, 1.5, 2, 3, 4];
	let at = 0;

	const apply = () => {
		svg.style.width = `${steps[at] * 100}%`;
		$("#zoomlevel").textContent = `${steps[at]}\u00d7`;
		$("#zoomout").disabled = at === 0;
		$("#zoomin").disabled = at === steps.length - 1;
	};

	$("#zoomin").addEventListener("click", () => {
		at = Math.min(steps.length - 1, at + 1);
		apply();
	});
	$("#zoomout").addEventListener("click", () => {
		at = Math.max(0, at - 1);
		apply();
	});
	apply();
}

/* ---- totals ---- */

function ledger(totals, updated) {
	const entry = (label, value, cls = "") => {
		const div = document.createElement("div");
		if (cls) div.className = cls;
		const dt = document.createElement("dt");
		dt.textContent = label;
		const dd = document.createElement("dd");
		dd.textContent = value;
		div.append(dd, dt);
		return div;
	};

	$("#ledger").replaceChildren(
		entry("acres surveyed", totals.acres.toLocaleString()),
		entry("holders", totals.players.toLocaleString()),
		entry("parcels filed", totals.named.toLocaleString()),
		entry(
			"last survey",
			updated ? ago(Date.now() - updated) : "never",
			"filed",
		),
	);
}

function key() {
	$("#bands").replaceChildren(
		...BANDS.map((band) => {
			const li = document.createElement("li");
			const swatch = document.createElement("i");
			li.dataset.band = band;
			li.append(swatch, band);
			return li;
		}),
	);
}

/* ---- one parcel's deed ---- */

function deed(plot) {
	const panel = $("#deed");
	const parts = [];

	const name = document.createElement("h3");
	name.textContent = plot.named ? plot.nick : "Unfiled parcel";
	if (!plot.named) name.className = "anon";
	parts.push(name);

	if (plot.named) {
		const band = document.createElement("div");
		band.className = "band";
		band.textContent = plot.wealth;
		parts.push(band);
	}

	const deer = flagOf(plot);
	if (deer) parts.push(deer);

	const { acres, built, spare } = usage(plot);
	const facts = document.createElement("dl");
	facts.className = "facts";
	for (const [label, value] of [
		["acres", acres.toLocaleString()],
		["worked", `${built} of ${acres}`],
		["spare", spare],
		["rations drawn", plot.rations.toLocaleString()],
	]) {
		const dt = document.createElement("dt");
		dt.textContent = label;
		const dd = document.createElement("dd");
		dd.textContent = value;
		facts.append(dt, dd);
	}
	parts.push(facts);

	if (plot.named) {
		parts.push(holdings(plot.land) ?? hint("Nothing built on it yet."));
	} else {
		parts.push(
			hint(
				"Held by somebody who has not filed a name. The registry publishes the acreage and nothing else.",
			),
		);
	}

	panel.replaceChildren(...parts);
}

/** An item list with the item drawn beside each row. */
function holdings(land) {
	const names = Object.keys(land ?? {}).sort();
	if (names.length === 0) return null;

	const list = document.createElement("ul");
	list.className = "holdings";
	list.replaceChildren(
		...names.map((item) => {
			const li = document.createElement("li");

			const img = document.createElement("img");
			img.src = spriteURL(item);
			img.alt = "";
			img.width = 22;
			img.height = 22;

			const label = document.createElement("span");
			label.textContent = nameOf(item, land[item]);

			const n = document.createElement("b");
			n.textContent = land[item].toLocaleString();

			li.append(img, label, n);
			return li;
		}),
	);
	return list;
}

function flagOf(plot) {
	const code = plot.named && plot.flag && flags[plot.flag];
	if (!code) return null;

	const fig = document.createElement("figure");
	fig.className = "deer";
	const img = document.createElement("img");
	img.src = toDataURL(normalise(code), `flag:${plot.flag}`);
	img.alt = `the deer named ${plot.flag}`;
	const cap = document.createElement("figcaption");
	cap.textContent = `flying ${plot.flag}`;
	fig.append(img, cap);
	return fig;
}

/* ---- the signed-in file ---- */

async function file(own) {
	const yours = await load("/ohayou/api/me");
	if (!yours || yours.status === "unclaimed") return;

	const panels = [
		figure(
			"Ohayous on hand",
			yours.ohayous,
			`${yours.cumulative} drawn in all`,
		),
		yours.vault && vault(yours.vault),
		figure("Defence", yours.defense, armour(yours.equipped)),
		running(yours.running),
		probation(yours.probation),
		counts("Metals", yours.metals),
		stock(yours.items),
		controls(own),
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
	n.textContent = Number(value).toLocaleString();

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
	const el = figure(`Vault, level ${v.level}`, v.ohayous, `of ${v.cap} held`);
	const meter = document.createElement("div");
	meter.className = "meter";
	const bar = document.createElement("span");
	bar.style.width = `${Math.min(100, (v.ohayous / v.cap) * 100)}%`;
	meter.append(bar);
	el.append(meter);
	return el;
}

function armour(equipped) {
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
			n.textContent = held[name].toLocaleString();
			row.append(name, n);
			return row;
		}),
	);
	return panel(title, list);
}

/** Everything owned, drawn. */
function stock(items) {
	const list = holdings(items);
	if (!list) return null;
	const el = panel("Everything you own", list);
	el.classList.add("wide");
	return el;
}

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
	return panel("Working", list);
}

function probation(at) {
	if (!at || at * 1000 <= Date.now()) return null;

	const el = document.createElement("article");
	el.className = "panel warn";
	const head = document.createElement("h3");
	head.textContent = "Bound over";
	const n = document.createElement("div");
	n.className = "figure";
	n.textContent = until(at * 1000 - Date.now());
	const sub = document.createElement("div");
	sub.className = "sub";
	sub.textContent = "left to serve";
	el.append(head, n, sub);
	return el;
}

/** Queues a request. The bot polls, applies it, and the map redraws next survey. */
function controls(own) {
	const flag = document.createElement("input");
	flag.type = "text";
	flag.maxLength = 48;
	flag.placeholder = "a deer's name";
	flag.value = own?.flag ?? "";
	flag.setAttribute("aria-label", "the deer to fly over your parcel");

	const say = document.createElement("p");
	say.className = "hint";

	const send = async (kind, value) => {
		say.textContent = "Filing…";
		const res = await fetch("/ohayou/api/command", {
			method: "POST",
			headers: { "content-type": "application/json" },
			body: JSON.stringify({ kind, value }),
		}).catch(() => null);

		say.textContent = res?.ok
			? "Filed. The bot picks it up within a minute."
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
	down.textContent = "Withdraw my name";
	down.addEventListener("click", () => send("territory", "off"));

	const el = panel("Your flag", row, down, say);
	el.classList.add("wide");
	return el;
}

/* ---- odds and ends ---- */

function labelOf(kind) {
	return (
		{ mining: "quarry", pumping: "oil well", breeding: "cattery" }[kind] ?? kind
	);
}

function hint(text) {
	const p = document.createElement("p");
	p.className = "hint";
	p.textContent = text;
	return p;
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

function ago(ms) {
	const minutes = Math.floor(ms / 60000);
	if (minutes < 1) return "just now";
	if (minutes < 60) return `${minutes}m ago`;
	const hours = Math.floor(minutes / 60);
	if (hours < 24) return `${hours}h ago`;
	return `${Math.floor(hours / 24)}d ago`;
}

async function load(url) {
	try {
		const res = await fetch(url);
		return res.ok ? await res.json() : null;
	} catch {
		return null;
	}
}

async function me() {
	return load("/api/session");
}

start();
