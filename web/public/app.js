/*
 * The front door. Signed out it is a pitch; signed in it is your file. Both
 * states share the same hero, so arriving on a link does not swap the page out
 * from under you.
 *
 * Which places appear comes from plugins.json, so one added there appears here
 * without this file knowing its name.
 */

import { normalise, toDataURL } from "./deerkins/kins.js";
import { drawWorld } from "./ohayou/map.js";
import { spriteURL } from "./ohayou/sprites.js";
import { load, places, SCOPES, shell, signin, site } from "./shell.js";

const $ = (sel) => document.querySelector(sel);

let session = null;

async function start() {
	session = await shell({ area: "home", current: "/" });
	const { fromLink } = await signin();

	const [world, place] = await Promise.all([
		load("/ohayou/api/world"),
		site(),
		session ? null : chat(),
	]);

	if (session) signedIn(world);
	else pitch(place, fromLink);

	map(world);
	await tiles(world);
	if (session) await Promise.all([standing(), drops()]);
}

/* ---- the hero ---- */

function pitch(place, fromLink) {
	$("#kicker").textContent = place?.channel
		? `an irc game, played in ${place.channel}`
		: "a game played in a chatroom";
	$("#headline").textContent = "Say good morning. Take an acre.";
	$("#blurb").textContent =
		"A bot sits in a chatroom and hands one ration to anyone who says hello to it. Rations buy land. Land carries cats, quarries, workshops and refineries, and the office draws every acre of it below, one tile to the acre.";
	$("#cta").replaceChildren(
		anchor("Start here", "#start", "btn"),
		anchor("Read the map", "/ohayou/", "btn ghost"),
	);
	$("#start").hidden = false;

	// Arrived on a link that did not work: say so where step three explains it.
	if (fromLink) {
		$("#why").classList.add("bad");
		$("#why").textContent =
			"That link is expired, already used, or not valid. Ask the bot for another.";
	}
}

function signedIn(world) {
	$("#kicker").textContent = "signed in";
	$("#headline").textContent = session.nick;

	const mine = world?.plots?.find((p) => p.named && p.id === session.account);
	$("#blurb").textContent = mine
		? `${mine.acres.toLocaleString()} acres, ${mine.wealth}, ${mine.rations.toLocaleString()} rations drawn since you started.`
		: "Nothing filed against your name on the map yet. Say !ohayou to the bot and it will find you an acre.";

	const buttons = [];
	if (mine) {
		buttons.push(
			anchor("My deed", `/ohayou/p/${encodeURIComponent(mine.nick)}`, "btn"),
		);
	}
	buttons.push(anchor("The whole map", "/ohayou/", "btn ghost"));
	buttons.push(anchor("Draw a deer", "/deerkins/", "btn ghost"));
	$("#cta").replaceChildren(...buttons);
}

/* ---- what you have ---- */

/** The top of your file, so the front door is worth opening when signed in.
    The whole of it, and everything you can change, is on the registry. */
async function standing() {
	if (!(session.scopes & SCOPES.ohayou)) return;

	const yours = await load("/ohayou/api/me");
	if (!yours || yours.status === "unclaimed") return;

	const stats = [
		["ohayous on hand", yours.ohayous.toLocaleString()],
		yours.vault && [
			`vault, level ${yours.vault.level}`,
			yours.vault.ohayous.toLocaleString(),
		],
		["defence", yours.defense.toLocaleString()],
		["ohayous, all time", yours.cumulative.toLocaleString()],
		...yours.running.map((run) => [labelOf(run.kind), until(run.due)]),
	].filter(Boolean);

	$("#stats").replaceChildren(
		...stats.map(([label, value], at) => {
			const box = document.createElement("div");
			// The countdowns go last and read as words, not totals.
			if (at >= stats.length - yours.running.length) box.className = "filed";
			const dd = document.createElement("dd");
			dd.textContent = value;
			const dt = document.createElement("dt");
			dt.textContent = label;
			box.append(dd, dt);
			return box;
		}),
	);

	if (yours.fortune) {
		const line = $("#fortune");
		line.textContent = yours.fortune;
		line.hidden = false;
	}
	$("#standing").hidden = false;
}

function labelOf(kind) {
	return (
		{ mining: "quarry", pumping: "oil well", breeding: "cattery" }[kind] ?? kind
	);
}

/** Rounded to the minute: this is not the page you watch a clock on. */
function until(due) {
	const minutes = Math.ceil((due * 1000 - Date.now()) / 60000);
	if (minutes <= 0) return "done";
	if (minutes < 60) return `${minutes}m`;
	return `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
}

/** A corner of the map, drawn by the map's own renderer so the front page
    cannot show something the map would not. */
function map(world) {
	if (!world?.plots?.length) {
		$("#viewbar").textContent = "Nothing claimed yet. Be the first.";
		return;
	}

	const biggest = [...world.plots]
		.sort((a, b) => b.acres - a.acres)
		.slice(0, 18);
	const { svg } = drawWorld(biggest, { flags: world.flags ?? {} });
	$("#minimap").replaceChildren(svg);

	const totals = document.createElement("span");
	totals.textContent = `${world.totals.players.toLocaleString()} ${
		world.totals.players === 1 ? "holder" : "holders"
	} · ${world.totals.acres.toLocaleString()} acres`;

	$("#viewbar").replaceChildren(
		totals,
		anchor("all of it →", "/ohayou/", "more"),
	);
}

/* ---- where to go ---- */

/** Each tile carries something live, so the list is not three sentences. A
    place the session cannot reach is still listed, marked: a visitor should
    see everything that is here. */
async function tiles(world) {
	const list = $("#places");
	const known = await places();
	if (known.length === 0) return;

	const cards = await Promise.all(known.map((p) => tile(p, world)));
	list.replaceChildren(...cards);
}

async function tile(plugin, world) {
	const item = document.createElement("li");
	item.className = "tile";
	item.dataset.area = plugin.name;

	const link = document.createElement("a");
	link.href = plugin.path;
	link.className = "title";
	link.textContent = plugin.title;

	const blurb = document.createElement("p");
	blurb.textContent = plugin.blurb;

	const foot = document.createElement("p");
	foot.className = "foot";
	const wanted = SCOPES[plugin.name];
	if (session && wanted && !(session.scopes & wanted)) {
		foot.textContent = "not on your link";
	}

	item.append(link, blurb, (await extra(plugin.name, world)) ?? foot);
	return item;
}

async function extra(name, world) {
	if (name === "ohayou") return acres(world);
	if (name === "deerkins") return newest();
	return null;
}

function acres(world) {
	if (!world?.totals) return null;

	const row = document.createElement("p");
	row.className = "foot";

	const img = document.createElement("img");
	img.src = spriteURL("acre");
	img.alt = "";
	img.width = 20;
	img.height = 20;

	row.append(
		img,
		`${world.totals.acres.toLocaleString()} acres over ${world.totals.players.toLocaleString()} parcels`,
	);
	return row;
}

/** The three most recent drawings, which is a better advertisement for the
    gallery than a count of it. */
async function newest() {
	const body = await load("/deerkins/api/art");
	const deer = body?.deer?.slice(0, 3) ?? [];
	if (deer.length === 0) return null;

	const strip = document.createElement("p");
	strip.className = "foot art";
	strip.replaceChildren(
		...deer.map((one) => {
			const img = document.createElement("img");
			img.src = toDataURL(normalise(one.kinskode), `art:${one.deer}`);
			img.alt = one.deer;
			img.title = one.creator ? `${one.deer}, by ${one.creator}` : one.deer;
			img.loading = "lazy";
			return img;
		}),
	);
	return strip;
}

/* ---- the webchat, behind a click ---- */

/** Loading it on arrival would put a third-party frame on every visit and is
    the heaviest thing on the page. */
async function chat() {
	const place = await site();
	if (place?.network) $("#network").textContent = place.network;
	if (place?.channel) $("#channel").textContent = place.channel;
	if (!place?.webchat) return;

	const panel = $("#chatpanel");
	const offer = $("#chatoffer");
	const where = [place.channel, place.network].filter(Boolean).join(" on ");

	$("#webchat").href = place.webchat;
	panel.hidden = false;

	$("#openchat").addEventListener(
		"click",
		() => {
			const frame = document.createElement("iframe");
			frame.src = place.webchat;
			frame.title = where ? `IRC webchat, ${where}` : "IRC webchat";
			frame.referrerPolicy = "no-referrer";
			// No allow-top-navigation: a framed page cannot steer this tab. The
			// rest is what a chat client needs, and grants nothing here since the
			// frame is another origin.
			frame.setAttribute(
				"sandbox",
				"allow-scripts allow-forms allow-same-origin allow-popups",
			);
			offer.replaceWith(frame);
		},
		{ once: true },
	);
}

/* ---- your drops ---- */

async function drops() {
	if (!(session.scopes & SCOPES.drop)) return;

	const body = await load("/drop/api/uploads");
	const uploads = body?.uploads ?? [];
	if (uploads.length === 0) return;

	$("#droplist").replaceChildren(
		...uploads.map((drop) => {
			const item = document.createElement("li");
			const link = document.createElement("a");
			link.href = drop.url;
			link.rel = "noreferrer";

			const thumb = document.createElement("img");
			thumb.src = drop.url;
			thumb.alt = "";
			thumb.loading = "lazy";
			link.append(thumb);

			const where = document.createElement("span");
			where.textContent = drop.channel;

			item.append(link, where);
			return item;
		}),
	);
	$("#drops").hidden = false;
}

function anchor(text, href, className) {
	const a = document.createElement("a");
	a.href = href;
	a.className = className;
	a.textContent = text;
	return a;
}

start();
