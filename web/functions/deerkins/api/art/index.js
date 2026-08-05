/*
 * GET  /deerkins/api/art?start=0   list art, newest first
 * GET  /deerkins/api/art?q=foo     prefix search on name
 * POST /deerkins/api/art           save a drawing
 */

import {
	clientKey,
	escapeLike,
	fail,
	guard,
	intVar,
	json,
	readJson,
	rejectCrossOrigin,
} from "../../../../lib/http.js";
import {
	MAX_NAME,
	nameSuffix,
	normalise,
	sanitiseCreator,
	sanitiseName,
	toIrc,
	validate,
} from "../../../../public/deerkins/kins.js";

const PAGE_SIZE = 20;
const SEARCH_LIMIT = 10;
const MAX_START = 5000;
const NAME_ATTEMPTS = 6;
const HOUR = 3600_000;
const DAY = 24 * HOUR;

export const onRequestGet = guard(async ({ request, env }) => {
	const url = new URL(request.url);
	const query = url.searchParams.get("q");

	if (query !== null) {
		const term = sanitiseName(query);
		if (!term) return json({ status: "search", deer: [] });

		const { results } = await env.DB.prepare(
			"SELECT deer FROM deer WHERE deer LIKE ?1 ESCAPE '\\' ORDER BY deer LIMIT ?2",
		)
			.bind(`${escapeLike(term)}%`, SEARCH_LIMIT)
			.all();

		return json(
			{ status: "search", deer: results.map((r) => r.deer) },
			{ headers: { "cache-control": "public, max-age=60" } },
		);
	}

	// Capped because OFFSET scans everything it skips.
	const start = Math.min(
		MAX_START,
		Math.max(0, Number.parseInt(url.searchParams.get("start") ?? "0", 10) || 0),
	);

	const { results } = await env.DB.prepare(
		"SELECT deer, creator, date FROM deer ORDER BY date DESC, id DESC LIMIT ?1 OFFSET ?2",
	)
		.bind(PAGE_SIZE + 1, start)
		.all();

	return json(
		{
			status: "list",
			start,
			pageSize: PAGE_SIZE,
			more: results.length > PAGE_SIZE,
			deer: results.slice(0, PAGE_SIZE),
		},
		{ headers: { "cache-control": "public, max-age=30" } },
	);
});

export const onRequestPost = guard(async ({ request, env }) => {
	const blocked = rejectCrossOrigin(request);
	if (blocked) return blocked;

	const { body, error } = await readJson(request);
	if (error) return fail(400, error);

	const kinskode = normalise(body.kinskode ?? "");
	const invalid = validate(kinskode);
	if (invalid) return fail(400, invalid);

	const requested = sanitiseName(body.name) || "deer";
	const creator = sanitiseCreator(body.creator) || "Anonydeer";

	const rejection = await reserveSlot(request, env);
	if (rejection) return rejection;

	const irccode = toIrc(kinskode);
	const date = new Date().toISOString().slice(0, 19).replace("T", " ");

	// deer.deer is UNIQUE. On collision, retry with a random suffix, leaving room
	// for it so a maximum-length name cannot collide with itself forever.
	const base = requested.slice(0, MAX_NAME - 6);
	let name = requested;

	for (let attempt = 0; attempt < NAME_ATTEMPTS; attempt++) {
		try {
			await env.DB.prepare(
				"INSERT INTO deer (date, creator, deer, kinskode, irccode) VALUES (?1, ?2, ?3, ?4, ?5)",
			)
				.bind(date, creator, name, kinskode, irccode)
				.run();

			return json(
				{ status: "saved", name, creator, kinskode, irccode },
				{ status: 201, headers: { "cache-control": "no-store" } },
			);
		} catch (err) {
			if (!isUniqueViolation(err)) throw err;
			name = `${base} ${nameSuffix()}`.trim();
		}
	}

	return fail(409, "could not find a free name, try a different one");
});

function isUniqueViolation(err) {
	return /UNIQUE constraint failed/i.test(String(err?.message ?? err));
}

/**
 * Claims one save against the per-IP and site-wide hourly limits, returning a
 * 429 Response if there is nothing left to claim.
 *
 * The claim is a single INSERT whose WHERE does the counting, so it is decided
 * inside one statement. Counting in a separate query first would let a burst of
 * concurrent requests all read the same count and all pass.
 */
async function reserveSlot(request, env) {
	const perIp = intVar(env.SAVES_PER_HOUR, 10);
	const siteWide = intVar(env.GLOBAL_SAVES_PER_HOUR, 300);
	const now = Date.now();
	const since = now - HOUR;
	const key = await clientKey(request, env);

	const [, claim] = await env.DB.batch([
		env.DB.prepare("DELETE FROM save_log WHERE ts < ?1").bind(now - DAY),
		env.DB.prepare(
			`INSERT INTO save_log (ip_hash, ts)
       SELECT ?1, ?2
       WHERE (SELECT COUNT(*) FROM save_log WHERE ip_hash = ?1 AND ts > ?3) < ?4
         AND (SELECT COUNT(*) FROM save_log WHERE ts > ?3) < ?5`,
		).bind(key, now, since, perIp, siteWide),
	]);

	if (claim.meta.changes === 1) return null;

	const mine = await env.DB.prepare(
		"SELECT COUNT(*) AS n FROM save_log WHERE ip_hash = ?1 AND ts > ?2",
	)
		.bind(key, since)
		.first();

	if ((mine?.n ?? 0) >= perIp) {
		return json(
			{
				status: "error",
				error: `you have saved ${perIp} pieces in the last hour, try again later`,
			},
			{
				status: 429,
				headers: { "retry-after": "900", "cache-control": "no-store" },
			},
		);
	}

	return json(
		{
			status: "error",
			error: "the gallery is busy right now, try again in a bit",
		},
		{
			status: 429,
			headers: { "retry-after": "600", "cache-control": "no-store" },
		},
	);
}
