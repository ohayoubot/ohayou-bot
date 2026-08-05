import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { onRequestGet } from "../lib/site/site.js";

async function get(env) {
	const response = await onRequestGet({ env });
	return { response, body: await response.json() };
}

test("the site says where the game is played", async () => {
	const { response, body } = await get({
		IRC_CHANNEL: "#chan",
		IRC_NETWORK: "Rizon",
		IRC_WEBCHAT: "https://webchat.example/",
	});

	assert.equal(response.status, 200);
	assert.deepEqual(body, {
		status: "site",
		channel: "#chan",
		network: "Rizon",
		webchat: "https://webchat.example/",
	});
});

// A deployment that has not been told is not a broken page.
test("an unconfigured site answers empties", async () => {
	const { response, body } = await get({});

	assert.equal(response.status, 200);
	assert.deepEqual(body, {
		status: "site",
		channel: "",
		network: "",
		webchat: "",
	});
});

/*
 * The landing page embeds the webchat, and a content-security-policy is not
 * configurable per deployment: it is a static header file. So the host named in
 * IRC_WEBCHAT has to be the host frame-src allows, or the panel opens onto a
 * frame the browser refuses to load.
 */
test("the webchat's host is one the policy allows to be framed", () => {
	const wrangler = readFileSync(
		new URL("../wrangler.toml", import.meta.url),
		"utf8",
	);
	const headers = readFileSync(
		new URL("../public/_headers", import.meta.url),
		"utf8",
	);

	const configured = [...wrangler.matchAll(/IRC_WEBCHAT\s*=\s*"([^"]*)"/g)].map(
		(m) => m[1],
	);
	assert.ok(configured.length > 0, "no IRC_WEBCHAT in wrangler.toml");

	const policy = headers.match(/frame-src ([^;\n]+)/);
	assert.ok(policy, "no frame-src in _headers");
	const allowed = policy[1].trim().split(/\s+/);

	for (const url of configured) {
		if (!url) continue;
		const { origin, protocol } = new URL(url);
		// An http frame inside an https page is mixed content and is blocked
		// before any policy is consulted.
		assert.equal(protocol, "https:", `${url} is not https`);
		assert.ok(
			allowed.includes(origin),
			`frame-src does not allow ${origin}: ${allowed.join(" ")}`,
		);
	}
});
