/*
 * A plot as a picture: the card that shows up when somebody posts their
 * permalink somewhere.
 *
 * Rendered as svg at request time rather than stored, because the plot changes
 * and the card should too. No fonts, no images, no scripts: a crawler fetching
 * this gets one file and needs nothing else, which is also the only thing the
 * site's own content-security-policy would allow.
 *
 * The deer is drawn from the gallery's kinskode, one rect per cell, in the
 * sixteen colours irc has.
 */

import { hexOf, normalise, TRANSPARENT } from "../../public/deerkins/kins.js";

/** The shape link previews expect. */
export const CARD_WIDTH = 1200;
export const CARD_HEIGHT = 630;

const BACKDROP = "#0d0f12";
const INK = "#e8e6e3";
const DIM = "#8b9199";
const MINE = "#4ade80";

/** Escapes the five characters that could end an attribute or a text node. A
    nick reaches this from the game, so it is not to be trusted with markup. */
function esc(text) {
	return String(text ?? "")
		.replace(/&/g, "&amp;")
		.replace(/</g, "&lt;")
		.replace(/>/g, "&gt;")
		.replace(/"/g, "&quot;")
		.replace(/'/g, "&#39;");
}

/**
 * A hue per item name, the same arithmetic the map uses, so a plot looks like
 * itself whether you are on the site or looking at a preview of it.
 */
function hueOf(item) {
	let hash = 0;
	for (let i = 0; i < item.length; i++) {
		hash = (hash * 31 + item.charCodeAt(i)) >>> 0;
	}
	return (hash * 137.508) % 360;
}

/** What sits on each acre, dealt out in name order like the map does. */
function fill(plot) {
	const out = [];
	for (const [item, count] of Object.entries(plot.land).sort()) {
		for (let i = 0; i < count && out.length < plot.acres; i++) out.push(item);
	}
	while (out.length < plot.acres) out.push(null);
	return out;
}

/** The acre grid, laid out as square as the acreage allows. */
function land(plot, x, y, size, gap) {
	const acres = Math.max(1, plot.acres);
	const wide = Math.ceil(Math.sqrt(acres));
	const on = fill(plot);

	const tiles = on.map((item, i) => {
		const cx = x + (i % wide) * (size + gap);
		const cy = y + Math.floor(i / wide) * (size + gap);
		const paint = item
			? `oklch(66% 0.15 ${hueOf(item).toFixed(1)})`
			: "#20242a";
		return `<rect x="${cx}" y="${cy}" width="${size}" height="${size}" rx="${
			size / 5
		}" fill="${paint}"/>`;
	});
	return tiles.join("");
}

/** The plot's deer, if it flies one and the gallery still has it. */
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

/**
 * The whole card. plot is a row from the projection; flag is the kinskode of
 * the deer it flies, or null.
 */
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
