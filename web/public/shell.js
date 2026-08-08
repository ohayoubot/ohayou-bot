/*
 * The header every page wears: the tabs, and who you are.
 *
 * The link the bot sends carries its grant in the url fragment. Redeeming it
 * here rather than in one page's script means the link signs you in wherever
 * you open it, and every page reads the session from the same fetch.
 */

/** Must match SCOPES in lib/hmac.js; a page cannot import from lib. */
export const SCOPES = { drop: 1 << 0, ohayou: 1 << 1 };

/** A grant is <payload>.<tag>, both base64url. */
const GRANT = /^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/;

/** Whether a fragment is a sign-in link rather than the page's own. The
    gallery puts an art name in the fragment and has to skip the same ones. */
export function isGrant(fragment) {
	return GRANT.test(fragment);
}

let signingIn = null;
let plugins = null;
let where = null;

/**
 * Resolves to {session, fromLink}. fromLink says a grant was in the url, so a
 * null session alongside it means the link was refused rather than absent.
 */
export function signin() {
	signingIn ??= redeemOrRead();
	return signingIn;
}

/** Which network and channel the game is played in. */
export function site() {
	where ??= load("/api/site");
	return where;
}

/** The places the site knows about, in nav order. */
export function places() {
	plugins ??= load("/plugins.json").then((body) => body?.plugins ?? []);
	return plugins;
}

async function redeemOrRead() {
	const fragment = location.hash.slice(1);
	if (!isGrant(fragment))
		return { session: await load("/api/session"), fromLink: false };

	const token = fragment;
	// Cleared before anything else runs, so a back button or a shared screen
	// does not hand the grant on. The fragment never reached the server's logs.
	history.replaceState(null, "", location.pathname + location.search);

	const res = await fetch("/api/session", {
		method: "POST",
		headers: { "content-type": "application/json" },
		body: JSON.stringify({ token }),
	}).catch(() => null);

	return { session: res?.ok ? await res.json() : null, fromLink: true };
}

/**
 * Fills in the header and returns the session, or null. `area` colours the
 * page's chrome and `current` is the tab to mark.
 */
export async function shell({ area, current } = {}) {
	if (area) document.body.dataset.area = area;

	const { session } = await signin();
	await Promise.all([tabs(current), badge(session)]);
	return session;
}

async function tabs(current) {
	const into = document.querySelector("#sitenav");
	if (!into) return;

	const links = [
		{ title: "home", path: "/" },
		...(await places()).map((p) => ({ title: p.tab ?? p.title, path: p.path })),
	];

	into.replaceChildren(
		...links.map(({ title, path }) => {
			const a = document.createElement("a");
			a.href = path;
			a.textContent = title;
			if (path === current) a.setAttribute("aria-current", "page");
			return a;
		}),
	);
}

/* ---- who you are ---- */

async function badge(session) {
	const into = document.querySelector("#account");
	if (!into) return;

	into.classList.toggle("on", Boolean(session));

	const drawer = document.createElement("div");
	drawer.className = "drawer";
	drawer.hidden = true;
	drawer.replaceChildren(
		...(session ? signedIn(session) : signedOut(await site())),
	);

	const button = document.createElement("button");
	button.type = "button";
	button.className = "quiet";
	button.setAttribute("aria-expanded", "false");
	button.append(pip(), label(session ? session.nick : "sign in"), caret());

	const toggle = (open) => {
		drawer.hidden = !open;
		button.setAttribute("aria-expanded", String(open));
	};
	button.addEventListener("click", () => toggle(drawer.hidden));
	document.addEventListener("click", (e) => {
		if (!into.contains(e.target)) toggle(false);
	});
	document.addEventListener("keydown", (e) => {
		if (e.key === "Escape" && !drawer.hidden) {
			toggle(false);
			button.focus();
		}
	});

	into.replaceChildren(button, drawer);
}

function pip() {
	const el = document.createElement("i");
	el.className = "pip";
	return el;
}

function caret() {
	const el = document.createElement("i");
	el.className = "caret";
	el.textContent = "▾";
	return el;
}

function label(text) {
	const el = document.createElement("span");
	el.className = "who";
	el.textContent = text;
	return el;
}

function signedIn(session) {
	const name = document.createElement("h3");
	name.textContent = session.nick;

	const account = document.createElement("p");
	account.className = "hint";
	account.textContent = `account ${session.account}`;

	const channels = document.createElement("p");
	channels.className = "hint";
	channels.textContent = session.channels.length
		? `posts to ${session.channels.join(", ")}`
		: "no channels on this link";

	const row = document.createElement("div");
	row.className = "row";
	if (session.scopes & SCOPES.ohayou) {
		row.append(
			link("my land", `/ohayou/p/${encodeURIComponent(session.nick)}`),
		);
	}
	if (session.scopes & SCOPES.drop) row.append(link("drop", "/drop/"));

	const out = document.createElement("button");
	out.type = "button";
	out.className = "quiet";
	out.textContent = "sign out";
	out.addEventListener("click", async () => {
		await fetch("/api/session", { method: "DELETE" }).catch(() => null);
		location.reload();
	});

	const foot = document.createElement("div");
	foot.className = "row";
	foot.append(out);

	return [name, account, channels, row, rule(), foot];
}

function signedOut(place) {
	const name = document.createElement("h3");
	name.textContent = "Not signed in";

	const how = document.createElement("p");
	how.className = "hint";
	how.append("Say ");
	how.append(tag("!web"));
	how.append(
		place?.channel
			? ` to the bot in ${place.channel} and it sends you a link, good once.`
			: " to the bot and it sends you a link, good once.",
	);

	const need = document.createElement("p");
	need.className = "hint";
	need.append("You have to be registered with it first: ");
	need.append(tag("!register"));
	need.append(", then ");
	need.append(tag("!identify"));
	need.append(".");

	const row = document.createElement("div");
	row.className = "row";
	row.append(link("how this works", "/"));

	return [name, how, need, row];
}

function rule() {
	return document.createElement("hr");
}

function tag(text) {
	const el = document.createElement("code");
	el.textContent = text;
	return el;
}

function link(text, href) {
	const a = document.createElement("a");
	a.className = "btn ghost";
	a.href = href;
	a.textContent = text;
	return a;
}

export async function load(url) {
	try {
		const res = await fetch(url);
		return res.ok ? await res.json() : null;
	} catch {
		return null;
	}
}
