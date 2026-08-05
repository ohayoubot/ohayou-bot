/*
 * Colours the map and the card have to agree on, since one is drawn by css and
 * the other by a worker. public/ohayou/style.css repeats TERRAIN as custom
 * properties; tools/world.map.test.mjs fails if the two drift.
 */

export const TERRAIN = {
	sea: "#050a1f",
	"sea-lit": "#0a1740",
	turf: "#14290f",
	"turf-lit": "#1c3a14",
	soil: "#3d2f18",
	pasture: "#2b5417",
	fallow: "#46402c",
	hedge: "#16300d",
};

/** Wealth bands, poorest first, matching internal/plugins/ohayou/web.go. */
export const BANDS = [
	"newcomer",
	"settler",
	"landowner",
	"industrialist",
	"magnate",
	"tycoon",
];

/** All from the sixteen bar the first, which is a green no palette entry has. */
export const BAND_COLOUR = {
	newcomer: "#2f5c22",
	settler: "#009393",
	landowner: "#00ffff",
	industrialist: "#ffff00",
	magnate: "#fc7e00",
	tycoon: "#ff00ff",
};

export function bandColour(band) {
	return BAND_COLOUR[band] ?? TERRAIN.hedge;
}
