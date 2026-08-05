/*
 * GET /api/site   where the bot actually lives
 *
 * The pages are static files, so they cannot read wrangler's vars. This hands
 * them the few facts that differ between one deployment and the next: which
 * network and channel the game is played in, and somewhere to join from
 * without installing anything.
 *
 * Nothing here is private. It is the answer to "where do I say !ohayou".
 */

import { guard, json } from "../http.js";

export const onRequestGet = guard(async ({ env }) =>
	json(
		{
			status: "site",
			channel: env.IRC_CHANNEL ?? "",
			network: env.IRC_NETWORK ?? "",
			// A webchat is the difference between "install a client" and
			// "click this", for somebody who has never used irc.
			webchat: env.IRC_WEBCHAT ?? "",
		},
		// It changes when the deployment does, which is to say rarely.
		{ headers: { "cache-control": "public, max-age=600" } },
	),
);
