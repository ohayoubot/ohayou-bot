/*
 * POST /ohayou/api/command   ask the bot to change something
 *
 * The site decides nothing. It records what somebody asked for and the bot
 * applies it against its own rules, which is where every guard lives: the
 * browser cannot move a single ohayou, because nothing here writes game state.
 *
 * Two checks, in two places. The shape is checked here, so nonsense never
 * reaches the queue. The authority is checked at the bot, which is the only
 * side that knows whether the account owns anything.
 *
 * Only cosmetic changes are on this list, and that is deliberate. The game is
 * played in irc; the site is for looking at it.
 */

import { SCOPE_OHAYOU } from "../hmac.js";
import {
	fail,
	guard,
	intVar,
	json,
	readJson,
	rejectCrossOrigin,
} from "../http.js";
import { requireScope } from "../session.js";

const HOUR = 3600_000;
const DAY = 24 * HOUR;

/** What a value may be, per kind. A kind not named here cannot be queued. */
const KINDS = {
	/** A deer's name from the gallery, or empty to take the flag down. */
	flag: (value) => typeof value === "string" && value.length <= 48,
	/** Whether the plot carries its owner's name. */
	territory: (value) => value === "on" || value === "off",
};

export const onRequestPost = guard(async ({ request, env }) => {
	const blocked = rejectCrossOrigin(request);
	if (blocked) return blocked;

	const session = await requireScope(request, env, SCOPE_OHAYOU);
	if (!session) return fail(401, "no session");

	if (!env.GAME) return fail(503, "the game is not published here");

	const { body, error } = await readJson(request);
	if (error) return fail(400, error);

	const check = KINDS[body.kind];
	if (!check) return fail(400, "there is nothing like that to change");
	if (!check(body.value)) return fail(400, "that is not a value for it");

	if (!(await reserveSlot(env, session.account))) {
		return json(
			{ status: "error", error: "you have asked for a lot lately, try later" },
			{
				status: 429,
				headers: { "retry-after": "300", "cache-control": "no-store" },
			},
		);
	}

	await env.GAME.prepare(
		"INSERT INTO command (ts, account, kind, value) VALUES (?1, ?2, ?3, ?4)",
	)
		.bind(Date.now(), session.account, body.kind, String(body.value))
		.run();

	// Queued, not done: the bot polls, and the map redraws when it publishes.
	return json(
		{ status: "queued", kind: body.kind },
		{ headers: { "cache-control": "no-store" } },
	);
});

/**
 * Claims one command against the hourly limit, returning whether there was one
 * to claim. The claim is a single INSERT whose WHERE does the counting, so a
 * burst of requests cannot all read the same count and all pass.
 *
 * It counts command_log rather than the queue, because the bot drains the
 * queue: a limit that forgets as soon as the bot catches up is no limit.
 */
async function reserveSlot(env, account) {
	const perHour = intVar(env.COMMANDS_PER_HOUR, 30);
	const now = Date.now();

	const [, claim] = await env.GAME.batch([
		env.GAME.prepare("DELETE FROM command_log WHERE ts < ?1").bind(now - DAY),
		env.GAME.prepare(
			`INSERT INTO command_log (account, ts)
       SELECT ?1, ?2
       WHERE (SELECT COUNT(*) FROM command_log WHERE account = ?1 AND ts > ?3) < ?4`,
		).bind(account, now, now - HOUR, perHour),
	]);

	return claim.meta.changes === 1;
}
