/* GET /deerkins/api/art/<name> */

import { decodeParam, fail, guard, json } from "../../../../lib/http.js";
import { sanitiseName, toIrc } from "../../../../public/deerkins/kins.js";

export const onRequestGet = guard(async ({ params, env }) => {
	const decoded = decodeParam(params.name);
	if (decoded === null) return fail(400, "bad name encoding");

	const name = sanitiseName(decoded);
	if (!name) return fail(400, "no name given");

	const row = await env.DB.prepare(
		"SELECT deer, creator, date, kinskode FROM deer WHERE deer = ?1 LIMIT 1",
	)
		.bind(name)
		.first();

	if (!row) return fail(404, "not found");

	return json(
		{
			status: "found",
			deer: row.deer,
			creator: row.creator,
			date: row.date,
			kinskode: row.kinskode,
			// Derived on read so a change to the encoder reaches old art too.
			irccode: toIrc(row.kinskode),
		},
		{ headers: { "cache-control": "public, max-age=60" } },
	);
});
