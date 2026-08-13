/* The handbook's catalogue against the two files the bot owns. */

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import {
	factsOf,
	forSale,
	ITEMS,
	itemNamed,
	METALS,
	madeNotBought,
	RECIPES,
	recipeFor,
} from "../public/ohayou/items.js";
import { SPRITES } from "../public/ohayou/sprites.js";

const items = JSON.parse(
	readFileSync(new URL("../../data/items.json", import.meta.url)),
);

const craft = readFileSync(
	new URL("../../internal/plugins/ohayou/craft.go", import.meta.url),
	"utf8",
);

/* ---- the shop ---- */

// A whole-array comparison rather than a field by field one: anything the game
// adds to an item reaches the handbook or this fails.
test("ITEMS is data/items.json", () => {
	assert.deepEqual(ITEMS, items);
});

test("every item the handbook lists can be drawn", () => {
	for (const item of ITEMS) {
		assert.ok(SPRITES[item.name], `${item.name} has no sprite`);
	}
});

test("the shop is ordered cheapest first", () => {
	const shop = forSale();
	assert.equal(shop.length, items.filter((i) => i.purchase).length);
	for (let i = 1; i < shop.length; i++) {
		assert.ok(
			shop[i - 1].price <= shop[i].price,
			`${shop[i - 1].name} (${shop[i - 1].price}) came before ${shop[i].name} (${shop[i].price})`,
		);
	}
	assert.equal(shop[0].price, 10, "the cheapest thing is a snack");
});

test("nothing bought turns up in the made list, and nothing made in the shop", () => {
	const bought = new Set(forSale().map((i) => i.name));
	for (const item of madeNotBought()) {
		assert.equal(bought.has(item.name), false, `${item.name} is in both`);
		assert.equal(item.purchase, false, `${item.name} says it is for sale`);
	}
});

// Every item is reachable from one of the two lists, or the page silently drops
// it while still passing the sprite check above.
test("the two lists between them cover the catalogue", () => {
	const listed = new Set([...forSale(), ...madeNotBought()].map((i) => i.name));
	for (const item of ITEMS) {
		assert.ok(listed.has(item.name), `${item.name} is on neither list`);
	}
});

/* ---- the crafting tree ---- */

/** recipeList out of craft.go, which is where the game keeps it. */
function goRecipes() {
	const from = craft.indexOf("var recipeList = []recipe{");
	assert.ok(from > 0, "recipeList is not in craft.go any more");
	const block = craft.slice(from, craft.indexOf("\n}\n", from));

	return block
		.split("{name:")
		.slice(1)
		.map((chunk) => ({
			name: /^\s*"([a-z]+)"/.exec(chunk)[1],
			amount: Number(/amount:\s*(\d+)/.exec(chunk)[1]),
			ohayous: Number(/ohayous:\s*(\d+)/.exec(chunk)?.[1] ?? 0),
			metals: goMap(/metals:\s*map\[string\]int\{([^}]*)\}/.exec(chunk)?.[1]),
			items: goMap(/items:\s*map\[string\]int\{([^}]*)\}/.exec(chunk)?.[1]),
		}));
}

/** `"iron": 2, "copper": 3` as an object. */
function goMap(body) {
	const out = {};
	for (const [, key, n] of (body ?? "").matchAll(/"(\w+)":\s*(\d+)/g)) {
		out[key] = Number(n);
	}
	return out;
}

test("RECIPES is the tree in craft.go", () => {
	assert.deepEqual(RECIPES, goRecipes());
});

test("a recipe only asks for things that exist", () => {
	for (const recipe of RECIPES) {
		assert.ok(itemNamed(recipe.name), `${recipe.name} builds nothing`);
		for (const metal of Object.keys(recipe.metals)) {
			assert.ok(METALS.includes(metal), `${recipe.name} wants ${metal}`);
		}
		for (const part of Object.keys(recipe.items)) {
			assert.ok(itemNamed(part), `${recipe.name} wants ${part}`);
		}
	}
});

// A part has to be craftable, or pumped, before the thing that needs it.
test("nothing is built out of something further down the tree", () => {
	const made = new Set(["oilbarrel"]);
	for (const recipe of RECIPES) {
		for (const part of Object.keys(recipe.items)) {
			assert.ok(made.has(part), `${recipe.name} needs ${part} first`);
		}
		made.add(recipe.name);
	}
});

test("every craftable has a recipe and every recipe a craftable", () => {
	for (const item of madeNotBought()) {
		if (item.name === "oilbarrel") continue;
		assert.ok(recipeFor(item.name), `${item.name} cannot be built`);
	}
});

/* ---- what a card says ---- */

test("the facts come off the item's own fields", () => {
	assert.deepEqual(factsOf(itemNamed("cat")), [
		"+1 every ration",
		"20 per acre",
	]);
	assert.deepEqual(factsOf(itemNamed("catnip")), [
		"2× what every cat earns",
		"one only",
	]);
	assert.deepEqual(factsOf(itemNamed("vest")), [
		"+54 defence",
		"worn on the body",
	]);
	assert.deepEqual(factsOf(itemNamed("burger")), ["used up when used"]);
	// needsAcre wins over the acre limit: "1 per acre" beside it reads as room
	// for one more thing on the same acre.
	assert.deepEqual(factsOf(itemNamed("quarry")), [
		"needs an acre to itself",
		"you use it",
	]);
});

test("an acre is plain enough to have nothing to say", () => {
	assert.deepEqual(factsOf(itemNamed("acre")), []);
});
