/*
 * Applies a database's sql with wrangler.
 *
 *   node tools/db.mjs <init|seed|purge> <database> [local|remote|preview] [--yes]
 *
 * One entry per database here rather than a script per file per environment,
 * which is nine scripts for one database and eighteen for two. db.test.mjs
 * checks each entry against wrangler.toml, so a database added here without a
 * binding fails the tests instead of at deploy time.
 */

import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { argv, exit } from "node:process";
import { fileURLToPath } from "node:url";

export const DATABASES = {
	deerkins: {
		schema: "schema/deerkins.sql",
		seed: "seed.sql",
		purge: "tools/purge.sql",
	},
};

export const ACTIONS = { init: "schema", seed: "seed", purge: "purge" };

export const ENVIRONMENTS = ["local", "remote", "preview"];

/** Preview is its own database rather than a flag, so a branch build cannot
    reach production data. */
export function target(database, environment) {
	return environment === "preview" ? `${database}-preview` : database;
}

export function wranglerArgs(database, environment, file) {
	return [
		"wrangler",
		"d1",
		"execute",
		target(database, environment),
		environment === "local" ? "--local" : "--remote",
		`--file=${file}`,
	];
}

function usage(message) {
	console.error(`${message}

usage: node tools/db.mjs <${Object.keys(ACTIONS).join("|")}> <${Object.keys(
		DATABASES,
	).join("|")}> [${ENVIRONMENTS.join("|")}] [--yes]

Anything but local needs --yes: remote is production.`);
	exit(1);
}

function main(args) {
	const yes = args.includes("--yes");
	const [action, database, environment = "local"] = args.filter(
		(a) => a !== "--yes",
	);

	if (!ACTIONS[action]) usage(`unknown action ${action ?? "(none)"}`);
	if (!DATABASES[database]) usage(`unknown database ${database ?? "(none)"}`);
	if (!ENVIRONMENTS.includes(environment)) {
		usage(`unknown environment ${environment}`);
	}

	const file = DATABASES[database][ACTIONS[action]];
	if (!file) usage(`${database} has no ${action}`);
	if (!existsSync(new URL(`../${file}`, import.meta.url))) {
		usage(`${file} does not exist`);
	}

	// The README's deploy steps run these against production. Making that an
	// explicit flag is the difference between a typo and a restore.
	if (environment !== "local" && !yes) {
		usage(`${action} on ${target(database, environment)} needs --yes`);
	}

	const [command, ...rest] = wranglerArgs(database, environment, file);
	console.log(`> pnpm exec ${command} ${rest.join(" ")}`);

	const run = spawnSync("pnpm", ["exec", command, ...rest], {
		stdio: "inherit",
		cwd: fileURLToPath(new URL("..", import.meta.url)),
	});
	exit(run.status ?? 1);
}

if (argv[1] === fileURLToPath(import.meta.url)) main(argv.slice(2));
