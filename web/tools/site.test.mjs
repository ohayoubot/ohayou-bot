import assert from "node:assert/strict";
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
