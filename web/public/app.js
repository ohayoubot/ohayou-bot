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
	places();
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
