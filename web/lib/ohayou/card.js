/*
 * A plot as an svg card, for the preview a permalink hands to whatever fetched
 * it. Rendered per request rather than stored, because the plot changes.
 *
 * No fonts, images or scripts: one file, which is also all the site's own
 * policy would allow. The deer is drawn from the gallery's kinskode.
 */

import { hexOf, normalise, TRANSPARENT } from "../../public/deerkins/kins.js";
import { layout } from "../../public/ohayou/plot.js";
import {
	SPRITE_SIZE,
	spriteFor,
	toRects,
} from "../../public/ohayou/sprites.js";
import { escapeHTML as esc } from "../http.js";

/** The shape link previews expect. */
export const CARD_WIDTH = 1200;
export const CARD_HEIGHT = 630;

const BACKDROP = "#0d0f12";
const INK = "#e8e6e3";
const DIM = "#8b9199";
const MINE = "#4ade80";
const SOIL = "#2a2113";
const EMPTY = "#1b1f24";

/** The acre grid, each worked acre carrying its item's sprite. */
function land(plot, x, y, size, gap) {
	const { wide, tiles } = layout(plot);

	const out = tiles.map((tile, i) => {
		const cx = x + (i % wide) * (size + gap);
		const cy = y + Math.floor(i / wide) * (size + gap);
		const ground = `<rect x="${cx}" y="${cy}" width="${size}" height="${size}" rx="${
			size / 6
		}" fill="${tile ? SOIL : EMPTY}"/>`;
		if (!tile) return ground;

		const side = tile.n === 1 ? 1 : 2;
		const box = size / side;
		const sprites = [];
		for (let k = 0; k < Math.min(tile.n, side * side); k++) {
			sprites.push(
				toRects(spriteFor(tile.item), {
					x: cx + (k % side) * box,
					y: cy + Math.floor(k / side) * box,
					cell: box / SPRITE_SIZE,
				}),
			);
		}
		return ground + sprites.join("");
	});
	return out.join("");
}

/** The plot's deer, when it flies one the gallery still has. */
function banner(kinskode, x, y, box) {
	if (!kinskode) return "";

	const rows = normalise(kinskode).split("\n");
	const wide = Math.max(...rows.map((r) => r.length), 1);
	const cell = Math.max(1, Math.min(box / wide, box / rows.length));

	const cells = [];
	rows.forEach((row, ry) => {
		for (let rx = 0; rx < wide; rx++) {
			const hex = row[rx] === TRANSPARENT ? null : hexOf(row[rx] ?? " ");
			if (!hex) continue;
			cells.push(
				`<rect x="${(x + rx * cell).toFixed(1)}" y="${(y + ry * cell).toFixed(
					1,
				)}" width="${cell.toFixed(1)}" height="${cell.toFixed(1)}" fill="${hex}"/>`,
			);
		}
	});
	return cells.join("");
}

/** plot is a row from the projection; flag is its deer's kinskode, or null. */
export function card(plot, flag, { channel, network } = {}) {
	const built = Object.entries(plot.land)
		.sort()
		.map(([item, n]) => `${item} ${n}`)
		.join(" · ");

	const where = [channel, network].filter(Boolean).join(" on ");

	return `<svg xmlns="http://www.w3.org/2000/svg" width="${CARD_WIDTH}" height="${CARD_HEIGHT}" viewBox="0 0 ${CARD_WIDTH} ${CARD_HEIGHT}" role="img" aria-label="${esc(
		plot.nick,
	)}'s territory">
<defs><linearGradient id="sky" x1="0" y1="0" x2="0" y2="1">
<stop offset="0" stop-color="#12161b"/><stop offset="1" stop-color="${BACKDROP}"/>
</linearGradient></defs>
<rect width="${CARD_WIDTH}" height="${CARD_HEIGHT}" fill="url(#sky)"/>
<rect x="0" y="0" width="${CARD_WIDTH}" height="6" fill="${MINE}"/>

<text x="72" y="132" fill="${INK}" font-family="ui-monospace, monospace" font-size="72" font-weight="700">${esc(
		plot.nick,
	)}</text>
<text x="72" y="184" fill="${DIM}" font-family="ui-monospace, monospace" font-size="30">${esc(
		plot.wealth,
	)} · ${plot.acres} ${plot.acres === 1 ? "acre" : "acres"} · ${
		plot.rations
	} ${plot.rations === 1 ? "ration" : "rations"}</text>

${land(plot, 72, 236, 46, 10)}
${banner(flag, 800, 150, 330)}

<text x="72" y="566" fill="${DIM}" font-family="ui-monospace, monospace" font-size="26">${esc(
		built,
	)}</text>
<text x="72" y="606" fill="${INK}" font-family="ui-monospace, monospace" font-size="26">say !ohayou${
		where ? ` in ${esc(where)}` : ""
	}</text>
</svg>`;
}
