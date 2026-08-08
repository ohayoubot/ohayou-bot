/*
 * What the site needs to know about the game's items to draw them.
 *
 * ACRELIMIT mirrors data/items.json, which the bot owns. It decides how many of
 * an item fit on one acre, and so how much of a plot a holding covers.
 * tools/catalog.test.mjs reads items.json and fails if the two disagree.
 */

export const ACRELIMIT = {
	cat: 20,
	dog: 1,
	cattery: 1,
	quarry: 1,
	oilwell: 1,
	workshop: 5,
	factory: 2,
	refinery: 1,
};

/**
 * What the acre under a thing looks like. A quarry stands on its own spoil and
 * a cattery on trodden earth; one brown square under everything is what made
 * the map read as icons on a lawn rather than as land.
 */
export const GROUND = {
	cat: "pen",
	dog: "pen",
	cattery: "pen",
	quarry: "spoil",
	oilwell: "spoil",
	workshop: "yard",
	factory: "yard",
	refinery: "yard",
};

/** Never null: a thing nobody has placed gets a works yard. */
export function groundFor(item) {
	return GROUND[item] ?? "yard";
}

/** Items that take up land. Anything else is carried, not built. */
export function occupiesLand(item) {
	return (ACRELIMIT[item] ?? 0) > 0;
}

/** Acres a holding of n covers, at that item's density. */
export function acresFor(item, n) {
	const limit = ACRELIMIT[item] ?? 0;
	if (limit <= 0 || n <= 0) return 0;
	return Math.ceil(n / limit);
}

/** Plural display names; anything absent takes an "s". */
const PLURALS = {
	catnip: "catnip",
	plastic: "plastic",
	steelplate: "steel plates",
	oilbarrel: "oil barrels",
	oilwell: "oil wells",
	fortunecookie: "fortune cookies",
	goldenrooster: "golden roosters",
	vaultupgrade: "vault upgrades",
	cattery: "catteries",
	factory: "factories",
	refinery: "refineries",
	quarry: "quarries",
	gloves: "gloves",
};

export function nameOf(item, n = 1) {
	if (n === 1)
		return item.replace(/(oil|fortune|golden|steel|vault)(?=\w)/, "$1 ");
	return PLURALS[item] ?? `${item}s`;
}
