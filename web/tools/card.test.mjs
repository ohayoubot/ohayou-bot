/* The card and the permalink. A nick reaches both from the game. */

import assert from "node:assert/strict";
import test from "node:test";
import { CARD_HEIGHT, CARD_WIDTH, card } from "../lib/ohayou/card.js";
import { onRequestGetCard, onRequestGetPage } from "../lib/ohayou/player.js";

function plot(overrides = {}) {
	return {
		id: "Mallow",
		nick: "mallow",
		flag: "",
		acres: 6,
		land: { cat: 4, quarry: 1 },
		wealth: "industrialist",
		rations: 120,
		...overrides,
	};
}

/** A GAME binding holding one plot, and a gallery holding one deer. */
function env(row, deer = null, extra = {}) {
	const answering = (first) => ({
		prepare: () => ({ bind: () => ({ first: async () => first }) }),
	});
	const out = {
		GAME: answering(row ? { ...row, land: JSON.stringify(row.land) } : null),
		...extra,
	};
	if (deer !== null) out.DB = answering(deer);
	return out;
}

async function get(handler, name, context) {
	const request = new Request(`https://hemera.day/ohayou/p/${name}`);
	const response = await handler({ request, params: { name }, env: context });
	return { response, text: await response.text() };
}

test("a card is an svg of the right shape", () => {
	const svg = card(plot(), null, {});

	assert.match(svg, /^<svg xmlns="http:\/\/www\.w3\.org\/2000\/svg"/);
	assert.ok(svg.includes(`width="${CARD_WIDTH}"`));
	assert.ok(svg.includes(`height="${CARD_HEIGHT}"`));
	assert.ok(svg.includes("mallow"));
	assert.ok(svg.includes("!ohayou"));
});

// One file: whatever fetches it needs nothing else, and the site's own policy
// would allow nothing else.
test("a card asks for nothing from anywhere else", () => {
	const svg = card(plot({ flag: "d" }), "AB\nCD", { channel: "#chan" });

	for (const forbidden of [
		"<script",
		"<image",
		"<use",
		"xlink:href",
		"@import",
	]) {
		assert.equal(svg.includes(forbidden), false, forbidden);
	}
	// The only url it carries is the xml namespace, which is a name rather than
	// somewhere anything is fetched from.
	assert.deepEqual(svg.match(/https?:\/\/[^"'\s]+/g), [
		"http://www.w3.org/2000/svg",
	]);
});

test("a nick cannot carry markup into the card", () => {
	const svg = card(plot({ nick: '<script>alert("x")</script>' }), null, {});

	assert.equal(svg.includes("<script>"), false);
	assert.ok(svg.includes("&lt;script&gt;"));
});

test("a nick cannot carry markup into the page", async () => {
	const { text } = await get(
		onRequestGetPage,
		"x",
		env(plot({ nick: '"><script>alert(1)</script>' })),
	);

	assert.equal(text.includes("<script>"), false);
	assert.ok(text.includes("&lt;script&gt;"));
});

test("the page carries the tags a preview reads", async () => {
	const { response, text } = await get(onRequestGetPage, "mallow", env(plot()));

	assert.equal(response.status, 200);
	assert.match(response.headers.get("content-type"), /text\/html/);
	assert.ok(text.includes('property="og:title"'));
	assert.ok(text.includes('property="og:image"'));
	assert.ok(text.includes("/ohayou/api/card/mallow"));
	// Rendered here, not by a script: the crawler will not run one.
	assert.equal(text.includes("<script"), false);
});

test("the card is served as an image", async () => {
	const { response, text } = await get(onRequestGetCard, "mallow", env(plot()));

	assert.equal(response.status, 200);
	assert.match(response.headers.get("content-type"), /image\/svg\+xml/);
	assert.equal(response.headers.get("x-content-type-options"), "nosniff");
	assert.ok(text.startsWith("<svg"));
});

// The query matches named = 1, so this is not a filter that can be forgotten.
test("a plot nobody named has no page", async () => {
	const { response } = await get(onRequestGetPage, "quiet", env(null));
	assert.equal(response.status, 404);

	const { response: image } = await get(onRequestGetCard, "quiet", env(null));
	assert.equal(image.status, 404);
});

test("a name that is not one is refused", async () => {
	for (const name of ["", "   ", "x".repeat(49), "%zz"]) {
		const { response } = await get(onRequestGetCard, name, env(plot()));
		assert.equal(response.status, 400, JSON.stringify(name));
	}
});

test("a flag is drawn from the gallery when there is one", () => {
	const svg = card(plot({ flag: "senordeer" }), "DD\nDD", {});
	assert.ok(svg.includes("flying senordeer"), "the deer drew nothing");
});

// The column the deer would fill is most parcels' empty space: nobody has to
// fly one.
test("a parcel with no deer shows what is on the land instead", () => {
	const svg = card(plot(), null, {});

	assert.ok(svg.includes("ON THE LAND"));
	assert.equal(svg.includes("flying"), false);
});

test("bare land shows neither", () => {
	const svg = card(plot({ land: {} }), null, {});

	assert.equal(svg.includes("ON THE LAND"), false);
	assert.equal(svg.includes("flying"), false);
});

test("the card says where the game is played", () => {
	const svg = card(plot(), null, { channel: "#chan", network: "Rizon" });
	assert.ok(svg.includes("#chan on Rizon"));
});
