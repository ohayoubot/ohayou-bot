/*
 * POST /api/ingest   the bot publishes a projection
 *
 * The only way game data reaches the site, and the only thing that decides what
 * may. The bot holds no database credential: every write is made here.
 *
 * The body is json, signed with the grant secret:
 *
 *   x-ingest-signature: first 16 bytes of HMAC-SHA256, base64url
 *
 * over "ingest.v1
" then the body's bytes. The prefix is domain separation: a
 * grant's payload is raw bytes starting with a version byte, so neither can be
 * replayed as the other under the shared key.
 *
 * Body: plugin, table, generation, ts, rows, and optionally mode and keep. A
 * publish replaces the table outright, so a player who withdrew consent is
 * absent rather than stale. mode "append" instead adds the rows and trims to
 * the newest keep of them, which is what the chronicle grows by.
 */

import { b64urlDecode } from "../hmac.js";
import { fail, guard, json } from "../http.js";

const MAX_BODY = 4 * 1024 * 1024;

/** How stale a signed request may be. */
const MAX_AGE = 300;

const TAG_BYTES = 16;
const PREFIX = "ingest.v1\n";

/**
 * What each plugin may write, and the shape of a row. A column not named here
 * cannot be published: a field added to the bot's projection has to be added
 * here too before it can reach the internet.
 *
 * "json" is stored as a json string, stringified here so the two sides cannot
 * disagree about encoding. "boolean" is stored as 0 or 1.
 */
const TABLES = {
	ohayou: {
		plot: {
			id: "string",
			nick: "string",
			named: "boolean",
			flag: "string",
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
		/* The chronicle. actor and subject are empty for anyone whose plot
		   carries no name, and detail holds bands rather than amounts. */
		event: {
			id: "integer",
			ts: "integer",
			kind: "string",
			actor: "string",
			subject: "string",
			detail: "json",
		},
	},
};

/** Which binding a plugin's tables live in. */
const BINDINGS = { ohayou: "GAME" };

/** The tables an append may be aimed at, and the column rows are trimmed by.
    Appending to the world would leave a withdrawn name on the map. */
const APPENDABLE = { ohayou: { event: "id" } };

/** The most rows an append may ask to keep. */
const MAX_KEEP = 5000;

export const onRequestPost = guard(async ({ request, env }) => {
	if (!env.OHAYOU_WEB_SECRET) return fail(503, "ingest is not configured");

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
	if (!sameBytes(given, await tag(raw, env.OHAYOU_WEB_SECRET))) {
		return refuse("bad signature");
	}

	// Past here the bytes are signed, so a failure is the bot disagreeing with
	// us rather than anything hostile. Still checked.
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

	// A retried publish is one that already landed, not an error.
	if (seen && seen.generation >= body.generation) {
		return json({ status: "stale", generation: seen.generation });
	}

	const total =
		body.mode === "append"
			? await append(db, body, columns, rows)
			: await replace(db, body, columns, rows);

	console.log(
		`ingest: ${body.plugin}.${body.table} generation ${body.generation}, ${
			body.mode === "append" ? `${rows.length} appended, ${total}` : rows.length
		} rows`,
	);
	return json({ status: "published", rows: rows.length, total });
});

/** One batch, which D1 runs as a transaction: no half-published table. */
async function replace(db, body, columns, rows) {
	const names = Object.keys(columns);
	const insert = db.prepare(
		`INSERT INTO ${body.table} (${names.join(", ")}) VALUES (${holes(names)})`,
	);

	await db.batch([
		db.prepare(`DELETE FROM ${body.table}`),
		...rows.map((row) => insert.bind(...names.map((name) => row[name]))),
		stamp(db, body),
	]);
	return rows.length;
}

/** Adds rows and trims the tail in one batch, returning what the table holds
    afterwards: how the bot notices the two ends have drifted apart. */
async function append(db, body, columns, rows) {
	const key = APPENDABLE[body.plugin][body.table];
	const names = Object.keys(columns);
	// OR REPLACE, so a row published twice lands rather than failing the batch.
	const insert = db.prepare(
		`INSERT OR REPLACE INTO ${body.table} (${names.join(", ")})
     VALUES (${holes(names)})`,
	);

	const done = await db.batch([
		...rows.map((row) => insert.bind(...names.map((name) => row[name]))),
		db
			.prepare(
				`DELETE FROM ${body.table} WHERE ${key} NOT IN
         (SELECT ${key} FROM ${body.table} ORDER BY ${key} DESC LIMIT ?1)`,
			)
			.bind(body.keep),
		stamp(db, body),
		db.prepare(`SELECT COUNT(*) AS held FROM ${body.table}`),
	]);
	return done.at(-1)?.results?.[0]?.held ?? 0;
}

const holes = (names) => names.map((_, i) => `?${i + 1}`).join(", ");

/** The publish row: what landed, how much is there, and when. */
function stamp(db, body) {
	return db
		.prepare(
			`INSERT INTO publish (plugin, table_name, generation, rows, updated)
       VALUES (?1, ?2, ?3, (SELECT COUNT(*) FROM ${body.table}), ?4)
       ON CONFLICT (plugin, table_name) DO UPDATE SET
         generation = excluded.generation,
         rows = excluded.rows,
         updated = excluded.updated`,
		)
		.bind(body.plugin, body.table, body.generation, Date.now());
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

	if (body.mode !== undefined && !["replace", "append"].includes(body.mode)) {
		return `unknown mode ${body.mode}`;
	}
	if (body.mode === "append") {
		if (!APPENDABLE[body.plugin]?.[body.table]) {
			return `${body.plugin}.${body.table} may not be appended to`;
		}
		if (
			!Number.isSafeInteger(body.keep) ||
			body.keep < 1 ||
			body.keep > MAX_KEEP
		) {
			return "bad keep";
		}
	}

	const age = Math.abs(Math.floor(Date.now() / 1000) - body.ts);
	if (age > MAX_AGE) return `timestamp is ${age}s off`;

	if (!Array.isArray(body.rows)) return "rows is not an array";
	return null;
}

/** Returns the row ready to bind, throwing on a column the table lacks. */
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

/** Constant time for equal lengths, the only case that reaches it. */
function sameBytes(a, b) {
	if (a.length !== b.length) return false;
	let diff = 0;
	for (let i = 0; i < a.length; i++) diff |= a[i] ^ b[i];
	return diff === 0;
}

/** One answer to every caller; the reason goes to the log. */
function refuse(reason) {
	console.log(`ingest refused: ${reason}`);
	return fail(400, "that publish was not accepted");
}

/** Exported for the tests, which sign their own bodies. */
export { PREFIX, tag };
