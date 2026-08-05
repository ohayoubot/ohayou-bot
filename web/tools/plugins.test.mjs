/*
 * plugins.json is what this site serves. Adding a plugin means touching places
 * that do not fail loudly when one is missed: the pages, the routes, and the
 * _routes.json include list, without which the function never runs and the
 * request falls through to a 404 asset.
 */

import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import test from "node:test";
import * as hmac from "../lib/hmac.js";

/** Copied out of the namespace so the scope names can be looked up by plugin. */
const WIRE_SCOPES = { ...hmac };

const read = (path) =>
	readFileSync(new URL(`../${path}`, import.meta.url), "utf8");

const { plugins } = JSON.parse(read("public/plugins.json"));
const routes = JSON.parse(read("public/_routes.json"));
const dashboard = read("public/app.js");

/** The site's own routes, which belong to no plugin. */
const SITE_ROUTES = ["/api/*"];

const exists = (path) => existsSync(new URL(`../${path}`, import.meta.url));

test("the manifest is not empty", () => {
	assert.ok(plugins.length > 0);
});

for (const plugin of plugins) {
	test(`${plugin.name} is described in full`, () => {
		assert.equal(typeof plugin.login, "boolean", "login is missing");
		for (const field of ["name", "title", "blurb", "path", "api"]) {
			assert.equal(typeof plugin[field], "string", `${field} is missing`);
			assert.notEqual(plugin[field], "", `${field} is empty`);
		}
		assert.equal(plugin.path, `/${plugin.name}/`);
		assert.equal(plugin.api, `/${plugin.name}/api/*`);
	});

	test(`${plugin.name} has its pages and its handlers`, () => {
		assert.ok(exists(`public/${plugin.name}`), "no public directory");
		assert.ok(exists(`functions/${plugin.name}/api`), "no api routes");
		assert.ok(exists(`lib/${plugin.name}`), "no lib directory");
	});

	test(`${plugin.name} is routed to its functions`, () => {
		assert.ok(
			routes.include.includes(plugin.api),
			`_routes.json does not include ${plugin.api}, so its api is dead`,
		);
	});

	// The dashboard builds its nav from this manifest. What it repeats is the
	// scope bitmask, because a page cannot import from lib.
	test(`${plugin.name}'s scope means the same in the browser as on the wire`, () => {
		const named = `SCOPE_${plugin.name.toUpperCase()}`;
		const onTheWire = WIRE_SCOPES[named];

		if (!plugin.login) {
			// Nothing here is behind a session, so a scope would be one nobody
			// checks and a link would carry a permission that means nothing.
			assert.equal(onTheWire, undefined, `${named} exists but nothing uses it`);
			return;
		}
		assert.equal(typeof onTheWire, "number", `lib/hmac.js has no ${named}`);

		const inTheBrowser = dashboard.match(
			new RegExp(`\\b${plugin.name}:\\s*1\\s*<<\\s*(\\d+)`),
		);
		assert.ok(inTheBrowser, `public/app.js has no scope for ${plugin.name}`);
		assert.equal(
			1 << Number(inTheBrowser[1]),
			onTheWire,
			`public/app.js disagrees with lib/hmac.js about ${plugin.name}`,
		);
	});
}

/*
 * A plugin may route anything under its own path: the api it must have, and
 * whatever else it needs rendered rather than served as a file. What it may not
 * do is claim a route belonging to somebody else, or to the site.
 */
test("_routes.json includes nothing that is not a plugin's or the site's", () => {
	const owned = plugins.map((p) => p.path);
	for (const include of routes.include) {
		if (SITE_ROUTES.includes(include)) continue;
		assert.ok(
			owned.some((path) => include.startsWith(path)),
			`${include} is under no plugin's path`,
		);
	}
});

test("the site's own routes are routed", () => {
	for (const route of SITE_ROUTES) {
		assert.ok(
			routes.include.includes(route),
			`${route} is not in _routes.json`,
		);
	}
});
