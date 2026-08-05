/*
 * The dashboard. Redeems a link from irc, then says who you are and what is
 * open to you.
 *
 * Which places appear comes from plugins.json and the session's scopes, so
 * adding a plugin adds a card here without this file knowing its name.
 */

const $ = (sel) => document.querySelector(sel);

const waitingEl = $("#waiting");
const youEl = $("#you");
const dropsEl = $("#drops");
const signinEl = $("#signin");

/** The sections that exist to explain the place to somebody who has not been
    here. They come down once we know who is looking. */
const PITCH = ["#pitch", "#join", "#signin"];

/** Must match lib/hmac.js. A page cannot import from lib, so this is the one
    place the browser repeats them. */
const SCOPES = { drop: 1 << 0, ohayou: 1 << 1 };

let session = null;

/**
 * The fragment never reaches the server's logs, and is cleared before anything
 * else runs so a shared screen or a back button does not hand the link on.
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
	// Somebody signed in does not need to be told what irc is.
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

/**
 * The part for somebody who followed a friend's plot here and has never used
 * irc. It says what the place is, shows that people are actually playing, and
 * gives them a way in that is not "install a client".
 */
async function welcome() {
	const [site, world] = await Promise.all([
		load("/api/site"),
		load("/ohayou/api/world"),
	]);

	if (site?.network) $("#network").textContent = site.network;
	if (site?.channel) $("#channel").textContent = site.channel;
	if (site?.webchat) {
		$("#webchat").href = site.webchat;
		$("#webchatline").hidden = false;
	}

	if (world?.plots?.length) glimpse(world);
}

/**
 * A strip of the biggest holdings: enough to show that the map is somebody's
 * work rather than a mock-up. Drawn from the same numbers the map itself uses,
 * so it cannot show a world that is not there.
 */
function glimpse(world) {
	const biggest = [...world.plots]
		.sort((a, b) => b.acres - a.acres)
		.slice(0, 12);

	$("#glimpse").replaceChildren(
		...biggest.map((plot) => {
			const acres = Math.max(1, plot.acres);
			const wide = Math.ceil(Math.sqrt(acres));

			const el = document.createElement("div");
			el.className = plot.named ? "patch" : "patch unnamed";
			el.style.setProperty("--wide", wide);

			const on = [];
			for (const [item, count] of Object.entries(plot.land).sort()) {
				for (let i = 0; i < count && on.length < acres; i++) on.push(item);
			}
			for (let i = 0; i < acres; i++) {
				const tile = document.createElement("span");
				if (on[i]) {
					tile.className = "built";
					tile.style.setProperty("--hue", hueOf(on[i]));
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

/** The same arithmetic the map uses, so a plot is the same colour here. */
function hueOf(item) {
	let hash = 0;
	for (let i = 0; i < item.length; i++) {
		hash = (hash * 31 + item.charCodeAt(i)) >>> 0;
	}
	return (hash * 137.508) % 360;
}

/** Shows the sections that belong to a signed-in visitor, and hides the rest.
    The "where to go" list is always up: it is what the site is. */
function show(section) {
	for (const el of [waitingEl, youEl, signinEl]) el.hidden = el !== section;
	if (section !== youEl) dropsEl.hidden = true;
}

/** Every plugin gets a card. One the session cannot reach still appears, saying
    so, rather than vanishing: a visitor should be able to see what is here. */
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
