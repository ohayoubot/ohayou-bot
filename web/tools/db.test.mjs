/*
 * Ties db.mjs to wrangler.toml. A database declared in one and not the other is
 * a deploy that half works: the schema applies to a name nothing is bound to,
 * or a binding resolves to a database with no tables.
 */

import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import test from "node:test";
import {
	ACTIONS,
	DATABASES,
	ENVIRONMENTS,
	target,
	wranglerArgs,
} from "./db.mjs";

const wrangler = readFileSync(
	new URL("../wrangler.toml", import.meta.url),
	"utf8",
);

/** Every database_name in wrangler.toml, production and preview alike. */
const bound = new Set(
	[...wrangler.matchAll(/^\s*database_name\s*=\s*"([^"]+)"/gm)].map(
		(m) => m[1],
	),
);

test("wrangler.toml declares at least one database", () => {
	assert.ok(bound.size > 0, "the binding regex matched nothing");
});

for (const [name, files] of Object.entries(DATABASES)) {
	test(`${name} has the sql files it claims`, () => {
		for (const [action, key] of Object.entries(ACTIONS)) {
			const file = files[key];
			if (!file) continue;
			assert.ok(
				existsSync(new URL(`../${file}`, import.meta.url)),
				`${name} ${action} points at ${file}, which is not there`,
			);
		}
	});

	test(`${name} has a schema`, () => {
		assert.ok(files.schema, "a database with no schema cannot be initialised");
	});

	test(`${name} is bound in wrangler.toml for every environment`, () => {
		for (const environment of ENVIRONMENTS) {
			// local runs against the production name with --local, so there are
			// only two distinct databases to bind.
			const wanted = target(name, environment);
			assert.ok(
				bound.has(wanted),
				`no database_name "${wanted}" in wrangler.toml`,
			);
		}
	});
}

test("preview never resolves to the production database", () => {
	for (const name of Object.keys(DATABASES)) {
		assert.notEqual(target(name, "preview"), target(name, "remote"));
	}
});

test("only local passes --local", () => {
	for (const environment of ENVIRONMENTS) {
		const args = wranglerArgs("deerkins", environment, "schema/deerkins.sql");
		assert.equal(
			args.includes("--local"),
			environment === "local",
			`${environment} got the wrong locality flag`,
		);
	}
});
