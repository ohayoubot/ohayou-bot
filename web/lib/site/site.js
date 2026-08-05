/*
 * GET /api/site   which network and channel the game is played in
 *
 * The pages are static files and cannot read wrangler's vars. Nothing here is
 * private: it is the answer to "where do I say !ohayou".
 */

import { guard, json } from "../http.js";

export const onRequestGet = guard(async ({ env }) =>
	json(
		{
			status: "site",
			channel: env.IRC_CHANNEL ?? "",
			network: env.IRC_NETWORK ?? "",
			webchat: env.IRC_WEBCHAT ?? "",
		},
		{ headers: { "cache-control": "public, max-age=600" } },
	),
);
