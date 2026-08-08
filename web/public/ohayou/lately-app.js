/*
 * The day book: the whole chronicle, one page of it.
 *
 * ?nick=<name> narrows it to one holder, which is what a deed page links to.
 * The filter is passed to the api rather than applied here, so a name that was
 * withheld cannot be found by asking for it.
 */

import { load, shell } from "../shell.js";
import { ago, readable } from "./chronicle.js";

const $ = (sel) => document.querySelector(sel);

const MAX_NICK = 48;

async function start() {
	await shell({ area: "ohayou", current: "/ohayou/" });

	const asked = new URL(location.href).searchParams.get("nick");
	const nick = asked && asked.length <= MAX_NICK ? asked : null;
	if (nick) narrowed(nick);

	const feed = await load(
		nick
			? `/ohayou/api/lately?nick=${encodeURIComponent(nick)}`
			: "/ohayou/api/lately",
	);

	const entries = readable(feed?.events);
	if (entries.length === 0) {
		empty(nick);
		return;
	}

	const now = Date.now();
	$("#entries").replaceChildren(...entries.map((entry) => line(entry, now)));
	$("#footnote").textContent = feed?.updated
		? `Surveyed ${ago(Math.floor(feed.updated / 1000), now)} ago.`
		: "";
}

function narrowed(nick) {
	$("#heading").textContent = `Lately: ${nick}`;
	$("#lede").textContent =
		"Everything on file for this holder, most recent first. A robbery appears at both ends, so it is here whether they did it or it was done to them.";
	document.title = `${nick} · lately · hemera.day`;
}

function empty(nick) {
	const li = document.createElement("li");
	li.className = "hint";
	li.textContent = nick
		? "Nothing on file for that name. Either nobody by it plays, or they hold land without their name on it."
		: "Nothing has happened yet.";
	$("#entries").replaceChildren(li);
}

function line(entry, now) {
	const li = document.createElement("li");
	li.className = "entry";

	const when = document.createElement("time");
	when.dateTime = new Date(entry.ts * 1000).toISOString();
	when.textContent = ago(entry.ts, now);

	const said = document.createElement("p");
	said.textContent = entry.said;

	li.append(when, said);
	return li;
}

start();
