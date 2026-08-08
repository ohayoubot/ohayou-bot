/*
 * The chronicle's words, shared by the front page and the deed page.
 *
 * Lives under public/ because the browser loads it and the worker imports it,
 * the way plot.js is shared.
 *
 * The bot has its own copy of these sentences for irc (chronicle.go). Neither
 * is authoritative: an event whose kind is not here is dropped rather than
 * guessed at, so a kind the bot learns before this does costs a line, not a
 * page.
 */

/** Who an event names, or nobody. A hidden player arrives already empty. */
function who(name) {
	return name || "Someone";
}

/**
 * One event as a sentence, or null when there are no words for its kind.
 * The subject of a robbery may be unnamed while its actor is not, and either
 * way the sentence has to read.
 */
export function phrase(event) {
	const actor = who(event.actor);
	const subject = event.subject || "someone";
	const detail = event.detail ?? {};

	switch (event.kind) {
		case "settle":
			return `${actor} settled, taking their first ration.`;
		case "land": {
			const acres = Number.parseInt(detail.acres, 10);
			if (!Number.isFinite(acres) || acres < 1) return null;
			return `${actor} bought ${acres} ${acres === 1 ? "acre" : "acres"}.`;
		}
		case "build":
			return detail.thing ? `${actor} raised a ${detail.thing}.` : null;
		case "strike":
			return detail.metal
				? `${actor} struck ${detail.metal} in the quarry.`
				: null;
		case "steal":
			return detail.took
				? `${actor} robbed ${subject} of ${detail.took}.`
				: null;
		case "caught":
			return `${actor} was caught robbing ${subject}, fined and put on probation.`;
		case "cat":
			return `${actor} took in the stray cat.`;
		case "flag":
			return detail.deer
				? `${actor} ran up the deer named ${detail.deer}.`
				: `${actor} struck their flag.`;
		case "double":
			return "The ohayou distributor malfunctioned. Rations doubled for a while.";
		default:
			return null;
	}
}

/** How long since, in the one unit that reads. */
export function ago(ts, now = Date.now()) {
	const seconds = Math.max(0, Math.floor(now / 1000 - ts));
	if (seconds < 60) return "just now";
	if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
	if (seconds < 86_400) return `${Math.floor(seconds / 3600)}h`;
	if (seconds < 172_800) return "yesterday";
	return `${Math.floor(seconds / 86_400)}d`;
}

/** The events that have words, newest first, each with its sentence. */
export function readable(events, limit = Number.POSITIVE_INFINITY) {
	const out = [];
	for (const event of events ?? []) {
		const said = phrase(event);
		if (!said) continue;
		out.push({ ...event, said });
		if (out.length >= limit) break;
	}
	return out;
}
