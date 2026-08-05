/*
 * Plot geometry and colour, shared by the map, the dashboard and the svg card
 * so a holding looks the same in all three.
 *
 * Lives under public/ because the browser loads it and the worker imports it,
 * the way the gallery's kins.js is shared.
 */

/**
 * Where a plot's acres go. tiles is one entry per acre, holding an item name or
 * null; wide by tall is the block it fills, which is square bar the remainder.
 *
 * Buildings are dealt out in name order, so a holding always draws the same
 * way. Which acre one sits on is not a game rule: the counts are what the bot
 * publishes, the arrangement is decoration.
 */
export function layout(plot) {
	const acres = Math.max(1, plot.acres);
	const wide = Math.ceil(Math.sqrt(acres));

	const tiles = [];
	for (const [item, count] of Object.entries(plot.land ?? {}).sort()) {
		for (let i = 0; i < count && tiles.length < acres; i++) tiles.push(item);
	}
	while (tiles.length < acres) tiles.push(null);

	return { acres, wide, tall: Math.ceil(acres / wide), tiles };
}

/**
 * A hue per item name, so an item added to the game gets a colour without
 * anything being taught its name. Spread by the golden angle so names that sort
 * together do not colour together.
 *
 * Lightness and chroma are the caller's: the stylesheet's for a page, fixed for
 * the card.
 */
export function hueOf(item) {
	let hash = 0;
	for (let i = 0; i < item.length; i++) {
		hash = (hash * 31 + item.charCodeAt(i)) >>> 0;
	}
	return (hash * 137.508) % 360;
}
