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
	fallow: "#574d33",
	hedge: "#0b1a06",

	// The three crops an unworked acre is under, and the ground the worked ones
	// stand on. See GROUND in catalog.js for which thing takes which.
	"crop-a": "#2f5d18",
	"crop-b": "#38621a",
	"crop-c": "#4a5c1c",
	pen: "#5b4526",
	yard: "#33333d",
	spoil: "#3b3833",
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
