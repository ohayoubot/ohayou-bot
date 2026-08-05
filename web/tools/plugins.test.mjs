/*
 * plugins.json is the list of what this site serves, and adding a plugin means
 * touching four places that do not fail loudly when one is missed: the pages,
 * the routes, the _routes.json include list (without which the function never
 * runs and the request falls through to a 404 asset) and the landing page.
 *
 * This is what makes the manifest load-bearing rather than decorative.
 */

import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import test from "node:test";

const read = (path) =>
	readFileSync(new URL(`../${path}`, import.meta.url), "utf8");

const { plugins } = JSON.parse(read("public/plugins.json"));
const routes = JSON.parse(read("public/_routes.json"));
const landing = read("public/index.html");

const exists = (path) => existsSync(new URL(`../${path}`, import.meta.url));

test("the manifest is not empty", () => {
	assert.ok(plugins.length > 0);
});

for (const plugin of plugins) {
	test(`${plugin.name} is described in full`, () => {
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

	test(`${plugin.name} is linked from the landing page`, () => {
		assert.ok(
			landing.includes(`href="${plugin.path}"`),
			`nothing on the landing page links to ${plugin.path}`,
		);
	});
}

test("_routes.json includes nothing that is not a plugin", () => {
	const declared = new Set(plugins.map((p) => p.api));
	for (const include of routes.include) {
		assert.ok(declared.has(include), `${include} belongs to no plugin`);
	}
});
