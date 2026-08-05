/*
 * The dashboard when signed in, the landing page when not.
 *
 * Which places appear comes from plugins.json and the session's scopes, so a
 * plugin added there appears here without this file knowing its name.
 */

import { layout } from "./ohayou/plot.js";
import { spriteURL } from "./ohayou/sprites.js";

const $ = (sel) => document.querySelector(sel);

const waitingEl = $("#waiting");
const youEl = $("#you");
const dropsEl = $("#drops");
const signinEl = $("#signin");

/** Shown only to a visitor who is not signed in. */
const PITCH = ["#pitch", "#join", "#signin"];

/** Must match lib/hmac.js; a page cannot import from lib. */
const SCOPES = { drop: 1 << 0, ohayou: 1 << 1 };

let session = null;

/**
 * The fragment never reaches the server's logs, and is cleared before anything
 * else runs so a back button or a shared screen does not hand the link on.
 */
async function start() {
	show(waitingEl);

	const token = location.hash.slice(1);
	if (token) history.replaceState(null, "", location.pathname);

	try {
		session = token ? await redeem(token) : await current();
	} catch {
		return signedOut("Could not reach the server.");
	}

	if (!session) {
		return signedOut(
			token ? "That link is expired, already used, or not valid." : "",
		);
	}

	$("#nick").textContent = session.nick;
	$("#account").textContent = `(${session.account})`;
	$("#channels").textContent = session.channels.length
		? `Posting to ${session.channels.join(", ")}.`
		: "";

	show(youEl);
	// Signed in, so no need to explain what irc is.
	for (const id of PITCH) $(id).hidden = true;
	await places();
	await drops();
}

async function redeem(token) {
	const res = await fetch("/api/session", {
		method: "POST",
		headers: { "content-type": "application/json" },
		body: JSON.stringify({ token }),
	});
	return res.ok ? await res.json() : null;
}

async function current() {
	const res = await fetch("/api/session");
	return res.ok ? await res.json() : null;
}

function signedOut(why) {
	$("#why").textContent = why;
	show(signinEl);
	for (const id of PITCH) $(id).hidden = false;
	places();
	welcome();
}

/** Fills in the network, channel and webchat, and a glimpse of the map. */
async function welcome() {
	const [site, world] = await Promise.all([
		load("/api/site"),
		load("/ohayou/api/world"),
	]);

	if (site?.network) $("#network").textContent = site.network;
	if (site?.channel) $("#channel").textContent = site.channel;
	if (site?.webchat) chat(site);

	if (world?.plots?.length) glimpse(world);
}

/**
 * The webchat, behind a click: loading it on arrival would put a third-party
 * frame on every visit and is the heaviest thing on the page.
 */
function chat(site) {
	const panel = $("#chatpanel");
	const offer = $("#chatoffer");
	const where = [site.channel, site.network].filter(Boolean).join(" on ");

	$("#webchat").href = site.webchat;
	panel.hidden = false;

	$("#openchat").addEventListener(
		"click",
		() => {
			const frame = document.createElement("iframe");
			frame.src = site.webchat;
			frame.title = where ? `IRC webchat, ${where}` : "IRC webchat";
			frame.referrerPolicy = "no-referrer";
			// No allow-top-navigation: a framed page cannot steer this tab. The
			// rest is what a chat client needs, and grants nothing here since
			// the frame is another origin.
			frame.setAttribute(
				"sandbox",
				"allow-scripts allow-forms allow-same-origin allow-popups",
			);
			offer.replaceWith(frame);
		},
		{ once: true },
	);
}

/** The biggest holdings, drawn from the same numbers the map itself uses. */
function glimpse(world) {
	const biggest = [...world.plots]
		.sort((a, b) => b.acres - a.acres)
		.slice(0, 12);

	$("#glimpse").replaceChildren(
		...biggest.map((plot) => {
			const { wide, tiles } = layout(plot);

			const el = document.createElement("div");
			el.className = plot.named ? "patch" : "patch unnamed";
			el.style.setProperty("--wide", wide);

			for (const built of tiles) {
				const tile = document.createElement("span");
				if (built) {
					tile.className = "built";
					tile.style.backgroundImage = `url("${spriteURL(built.item)}")`;
				}
				el.append(tile);
			}
			return el;
		}),
	);

	$("#glimpsecap").textContent = `${world.totals.players} ${
		world.totals.players === 1 ? "player" : "players"
	} holding ${world.totals.acres} acres. `;
	const link = document.createElement("a");
	link.href = "/ohayou/";
	link.textContent = "See the whole map";
	$("#glimpsecap").append(link);
}

/** The "where to go" list stays up either way. */
function show(section) {
	for (const el of [waitingEl, youEl, signinEl]) el.hidden = el !== section;
	if (section !== youEl) dropsEl.hidden = true;
}

/** A plugin the session cannot reach still appears, marked, rather than
    vanishing: a visitor should see what is here. */
async function places() {
	const list = $("#places");
	let plugins = [];
	try {
		plugins = (await (await fetch("/plugins.json")).json()).plugins;
	} catch {
		return;
	}

	list.replaceChildren(
		...plugins.map((plugin) => {
			const item = document.createElement("li");
			const link = document.createElement("a");
			link.href = plugin.path;
			link.textContent = plugin.title;
			item.append(link, `: ${plugin.blurb}`);

			const wanted = SCOPES[plugin.name];
			if (session && wanted && !(session.scopes & wanted)) {
				const note = document.createElement("span");
				note.className = "dim";
				note.textContent = " (not in your link)";
				item.append(note);
			}
			return item;
		}),
	);
}

async function drops() {
	if (!(session.scopes & SCOPES.drop)) return;

	let uploads = [];
	try {
		const res = await fetch("/drop/api/uploads");
		if (!res.ok) return;
		uploads = (await res.json()).uploads;
	} catch {
		return;
	}
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
			where.className = "dim";
			where.textContent = drop.channel;

			item.append(link, where);
			return item;
		}),
	);
	dropsEl.hidden = false;
}

$("#signout").addEventListener("click", async () => {
	await fetch("/api/session", { method: "DELETE" });
	session = null;
	signedOut("Signed out.");
});

start();
