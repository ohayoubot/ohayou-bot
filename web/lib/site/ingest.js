/*
 * POST /api/ingest   the bot publishes a projection
 *
 * This is the only way game data reaches the site, and the only thing that
 * decides what is allowed to. The bot holds no D1 credential: every write is
 * still made here, where the rules are.
 *
 * A request carries a json body signed with the same secret as the grants:
 *
 *   x-ingest-signature: <first 16 bytes of HMAC-SHA256, base64url>
 *
 * over "ingest.v1\n" followed by the body's bytes. The prefix is domain
 * separation: a grant's payload is raw bytes beginning with a version byte, so
 * one can never be replayed as the other even though the key is shared.
 *
 * Body:
 *   plugin      who is publishing
 *   table       which of its tables, checked against TABLES below
 *   generation  rises with every publish; one already seen is refused
 *   ts          unix seconds, bounding how long a captured request is worth
 *   rows        the whole table, which replaces what is there
 *
 * Nothing here is merged. A publish is the complete contents of that table for
 * that plugin, so a player who stopped consenting simply stops being in it.
 */

import { b64urlDecode } from "../hmac.js";
import { fail, guard, json } from "../http.js";

/** Signed bodies still get a ceiling: this is read before anything is checked. */
const MAX_BODY = 4 * 1024 * 1024;

/** How stale a signed request may be. Long enough for a slow publish, short
    enough that a captured one is not worth keeping. */
const MAX_AGE = 300;

const TAG_BYTES = 16;
const PREFIX = "ingest.v1\n";

/**
 * What each plugin may write, and the exact shape of a row. A column not named
 * here cannot be published, so a field added to the bot's projection has to be
 * added here too, on purpose, before it can reach the internet.
 *
 * "json" means the value is stored as a json string; the bot sends the value
 * itself and this stringifies it, so the two sides cannot disagree about
 * encoding. "boolean" is stored as 0 or 1, because sqlite has no other kind.
 */
const TABLES = {
	ohayou: {
		plot: {
			id: "string",
			nick: "string",
			named: "boolean",
			acres: "integer",
			land: "json",
			wealth: "string",
			rations: "integer",
		},
		plot_private: {
			account: "string",
			nick: "string",
			ohayous: "integer",
			cumulative: "integer",
			items: "json",
			metals: "json",
			equipped: "json",
			defense: "integer",
			vault: "json?",
			probation: "integer",
			fortune: "string",
			running: "json",
		},
	},
};

/** Which binding a plugin's tables live in. A plugin with no entry cannot
    publish at all, whatever it signs. */
const BINDINGS = { ohayou: "GAME" };

export const onRequestPost = guard(async ({ request, env }) => {
	if (!env.UPLOAD_HMAC_SECRET) return fail(503, "ingest is not configured");

	const declared = Number.parseInt(
		request.headers.get("content-length") ?? "",
		10,
	);
	if (Number.isFinite(declared) && declared > MAX_BODY) {
		return refuse("body too large");
	}

	const raw = new Uint8Array(await request.arrayBuffer());
	if (raw.length > MAX_BODY) return refuse("body too large");

	const given = b64urlDecode(request.headers.get("x-ingest-signature") ?? "");
	if (given === null || given.length !== TAG_BYTES)
		return refuse("no signature");
	if (!sameBytes(given, await tag(raw, env.UPLOAD_HMAC_SECRET))) {
		return refuse("bad signature");
	}

	// Everything past here ran on bytes the bot signed, so a failure is the bot
	// disagreeing with us rather than anything hostile. It is still checked.
	let body;
	try {
		body = JSON.parse(new TextDecoder().decode(raw));
	} catch {
		return refuse("body is not json");
	}

	const invalid = validate(body);
	if (invalid) return refuse(invalid);

	const binding = BINDINGS[body.plugin];
	const db = env[binding];
	if (!db) return refuse(`no ${binding} binding for ${body.plugin}`);

	const columns = TABLES[body.plugin][body.table];
	let rows;
	try {
		rows = body.rows.map((row) => flatten(row, columns));
	} catch (err) {
		return refuse(`row: ${err.message}`);
	}

	const seen = await db
		.prepare(
			"SELECT generation FROM publish WHERE plugin = ?1 AND table_name = ?2",
		)
		.bind(body.plugin, body.table)
		.first();

	// Not an error: a retried publish is a publish that already landed.
	if (seen && seen.generation >= body.generation) {
		return json({ status: "stale", generation: seen.generation });
	}

	await replace(db, body, columns, rows);

	console.log(
		`ingest: ${body.plugin}.${body.table} generation ${body.generation}, ${rows.length} rows`,
	);
	return json({ status: "published", rows: rows.length });
});

/**
 * Swaps the table's contents for the new rows in one batch, which D1 runs as a
 * transaction: a page never sees half of one publish beside half of the last.
 */
async function replace(db, body, columns, rows) {
	const names = Object.keys(columns);
	const placeholders = names.map((_, i) => `?${i + 1}`).join(", ");
	const insert = db.prepare(
		`INSERT INTO ${body.table} (${names.join(", ")}) VALUES (${placeholders})`,
	);

	await db.batch([
		db.prepare(`DELETE FROM ${body.table}`),
		...rows.map((row) => insert.bind(...names.map((name) => row[name]))),
		db
			.prepare(
				`INSERT INTO publish (plugin, table_name, generation, rows, updated)
         VALUES (?1, ?2, ?3, ?4, ?5)
         ON CONFLICT (plugin, table_name) DO UPDATE SET
           generation = excluded.generation,
           rows = excluded.rows,
           updated = excluded.updated`,
			)
			.bind(body.plugin, body.table, body.generation, rows.length, Date.now()),
	]);
}

function validate(body) {
	if (body === null || typeof body !== "object" || Array.isArray(body)) {
		return "body is not an object";
	}
	if (!TABLES[body.plugin]) return `unknown plugin ${body.plugin}`;
	if (!TABLES[body.plugin][body.table]) {
		return `${body.plugin} may not write ${body.table}`;
	}
	if (!Number.isSafeInteger(body.generation) || body.generation < 1) {
		return "bad generation";
	}
	if (!Number.isSafeInteger(body.ts)) return "bad timestamp";

	const age = Math.abs(Math.floor(Date.now() / 1000) - body.ts);
	if (age > MAX_AGE) return `timestamp is ${age}s off`;

	if (!Array.isArray(body.rows)) return "rows is not an array";
	return null;
}

/**
 * Checks one row against the column list and returns it ready to bind. Throws
 * on anything unexpected, including a key the table does not have: a field the
 * bot started sending must be added here before it can be stored.
 */
function flatten(row, columns) {
	if (row === null || typeof row !== "object" || Array.isArray(row)) {
		throw new Error("not an object");
	}
	for (const key of Object.keys(row)) {
		if (!(key in columns)) throw new Error(`unexpected column ${key}`);
	}

	const out = {};
	for (const [name, kind] of Object.entries(columns)) {
		const value = row[name];
		switch (kind) {
			case "string":
				if (typeof value !== "string")
					throw new Error(`${name} is not a string`);
				out[name] = value;
				break;
			case "integer":
				if (!Number.isSafeInteger(value)) {
					throw new Error(`${name} is not an integer`);
				}
				out[name] = value;
				break;
			case "boolean":
				if (typeof value !== "boolean") {
					throw new Error(`${name} is not a boolean`);
				}
				out[name] = value ? 1 : 0;
				break;
			case "json":
				if (value === undefined || value === null) {
					throw new Error(`${name} is missing`);
				}
				out[name] = JSON.stringify(value);
				break;
			case "json?":
				out[name] =
					value === undefined || value === null ? null : JSON.stringify(value);
				break;
			default:
				throw new Error(`${name} has an unknown kind`);
		}
	}
	return out;
}

async function tag(body, secret) {
	const key = await crypto.subtle.importKey(
		"raw",
		new TextEncoder().encode(secret),
		{ name: "HMAC", hash: "SHA-256" },
		false,
		["sign"],
	);
	const message = new Uint8Array(PREFIX.length + body.length);
	message.set(new TextEncoder().encode(PREFIX), 0);
	message.set(body, PREFIX.length);

	const full = await crypto.subtle.sign("HMAC", key, message);
	return new Uint8Array(full).subarray(0, TAG_BYTES);
}

/** Constant time for equal lengths, which is the only case that reaches it. */
function sameBytes(a, b) {
	if (a.length !== b.length) return false;
	let diff = 0;
	for (let i = 0; i < a.length; i++) diff |= a[i] ^ b[i];
	return diff === 0;
}

/** One answer to the caller, the reason to the log. The bot is the only
    legitimate caller and it reads its own logs, not this response. */
function refuse(reason) {
	console.log(`ingest refused: ${reason}`);
	return fail(400, "that publish was not accepted");
}

/** Exported for the tests, which sign their own bodies. */
export { PREFIX, tag };
