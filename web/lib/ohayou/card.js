/*
 * A plot as an svg card, for the preview a permalink hands to whatever fetched
 * it. Rendered per request rather than stored, because the plot changes.
 *
 * No fonts, images or scripts: one file, which is also all the site's own
 * policy would allow. The parcel is drawn from the same layout the map uses and
 * the deer from the gallery's kinskode, so the card and the site agree.
 */

import {
	hexOf,
	normalise,
	TRANSPARENT,
	toRects,
} from "../../public/deerkins/kins.js";
import { layout, usage } from "../../public/ohayou/plot.js";
import { SPRITE_SIZE, spriteFor } from "../../public/ohayou/sprites.js";
import { bandColour, TERRAIN } from "../../public/ohayou/terrain.js";
import { escapeHTML as esc } from "../http.js";

/** The shape link previews expect. */
export const CARD_WIDTH = 1200;
export const CARD_HEIGHT = 630;

const INK = "#ded9e6";
const DIM = "#8e8a9c";
const WHITE = "#ffffff";
const ACCENT = "#fc7e00";
const ACCENT_INK = "#1a0d00";
const PANEL = "#141419";
const LINE = "#2e2e38";

const MONO = "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace";

const BAR = 54;
const FOOT = 56;
const PAD = 40;
const SURVEY = 690;

/** plot is a row from the projection; flag is its deer's kinskode, or null. */
export function card(plot, flag, { channel, network } = {}) {
	const { acres, built } = usage(plot);
	const where = [channel, network].filter(Boolean).join(" on ");
	const band = bandColour(plot.wealth);

	return `<svg xmlns="http://www.w3.org/2000/svg" width="${CARD_WIDTH}" height="${CARD_HEIGHT}" viewBox="0 0 ${CARD_WIDTH} ${CARD_HEIGHT}" role="img" aria-label="${esc(
		plot.nick,
	)}'s territory">
<rect width="${CARD_WIDTH}" height="${CARD_HEIGHT}" fill="${PANEL}"/>

<rect width="${CARD_WIDTH}" height="${BAR}" fill="${ACCENT}"/>
<text x="${PAD}" y="36" fill="${ACCENT_INK}" font-family="${MONO}" font-size="21" font-weight="700" letter-spacing="3">HEMERA LAND OFFICE${
		where ? ` &#183; ${esc(where.toUpperCase())}` : ""
	}</text>
<text x="${CARD_WIDTH - PAD}" y="36" fill="${ACCENT_INK}" font-family="${MONO}" font-size="21" text-anchor="end" letter-spacing="2">PARCEL No ${fileNo(
		plot.id,
	)}</text>

${survey(plot, PAD, BAR + PAD, SURVEY, CARD_HEIGHT - BAR - FOOT - PAD * 2)}

${details(plot, flag, acres, built, band)}

<rect y="${CARD_HEIGHT - FOOT}" width="${CARD_WIDTH}" height="${FOOT}" fill="${TERRAIN.sea}"/>
<rect y="${CARD_HEIGHT - FOOT}" width="${CARD_WIDTH}" height="2" fill="${LINE}"/>
<text x="${PAD}" y="${
		CARD_HEIGHT - 20
	}" fill="${WHITE}" font-family="${MONO}" font-size="24">say <tspan fill="${ACCENT}">!ohayou</tspan>${
		where ? ` in ${esc(where)}` : ""
	} and the office will find you an acre</text>
<text x="${CARD_WIDTH - PAD}" y="${
		CARD_HEIGHT - 20
	}" fill="${DIM}" font-family="${MONO}" font-size="24" text-anchor="end">hemera.day</text>
</svg>`;
}

/** The parcel itself, on its own patch of ground. */
function survey(plot, x, y, w, h) {
	const { wide, tall, tiles } = layout(plot);
	const tile = Math.max(
		10,
		Math.min(96, Math.floor(Math.min(w / (wide + 2), h / (tall + 2)))),
	);
	const pw = wide * tile;
	const ph = tall * tile;
	const px = Math.round(x + (w - pw) / 2);
	const py = Math.round(y + (h - ph) / 2);

	const out = [
		`<rect x="${x}" y="${y}" width="${w}" height="${h}" fill="${TERRAIN.turf}"/>`,
		tufts(x, y, w, h),
		`<rect x="${px}" y="${py}" width="${pw}" height="${ph}" fill="${TERRAIN.pasture}"/>`,
	];

	tiles.forEach((slot, i) => {
		if (!slot) return;
		const cx = px + (i % wide) * tile;
		const cy = py + Math.floor(i / wide) * tile;
		out.push(
			`<rect x="${cx}" y="${cy}" width="${tile}" height="${tile}" fill="${TERRAIN.soil}"/>`,
		);

		const side = slot.n === 1 ? 1 : 2;
		const box = tile / side;
		for (let k = 0; k < Math.min(slot.n, side * side); k++) {
			out.push(
				toRects(spriteFor(slot.item), {
					x: cx + (k % side) * box,
					y: cy + Math.floor(k / side) * box,
					cell: box / SPRITE_SIZE,
				}),
			);
		}
	});

	out.push(
		`<rect x="${px}" y="${py}" width="${pw}" height="${ph}" fill="none" stroke="${TERRAIN.hedge}" stroke-width="3"/>`,
	);
	return out.join("");
}

/** Grass, so the ground around a parcel is not a flat rectangle. */
function tufts(x, y, w, h) {
	const out = [];
	for (let i = 0; i < 220; i++) {
		const tx = x + Math.floor(noise(i, 1) * w);
		const ty = y + Math.floor(noise(i, 2) * h);
		out.push(
			`<rect x="${tx}" y="${ty}" width="3" height="8" fill="${TERRAIN["turf-lit"]}"/>`,
		);
	}
	return out.join("");
}

function details(plot, flag, acres, built, band) {
	const x = PAD + SURVEY + 40;
	const w = CARD_WIDTH - x - PAD;
	let y = BAR + PAD + 34;

	const out = [
		`<text x="${x}" y="${y}" fill="${WHITE}" font-family="${MONO}" font-size="54" font-weight="700">${esc(
			clip(plot.nick, 12),
		)}</text>`,
	];

	y += 30;
	out.push(`<rect x="${x}" y="${y}" width="${w}" height="8" fill="${band}"/>`);
	y += 34;
	out.push(
		`<text x="${x}" y="${y}" fill="${band}" font-family="${MONO}" font-size="23" letter-spacing="3">${esc(
			plot.wealth.toUpperCase(),
		)}</text>`,
	);

	y += 44;
	for (const [label, value] of [
		["ACRES HELD", acres],
		["ACRES WORKED", built],
		["RATIONS DRAWN", plot.rations],
	]) {
		out.push(
			`<text x="${x}" y="${y}" fill="${DIM}" font-family="${MONO}" font-size="20" letter-spacing="2">${label}</text>`,
			`<text x="${x + w}" y="${y}" fill="${INK}" font-family="${MONO}" font-size="26" font-weight="700" text-anchor="end">${value}</text>`,
			`<rect x="${x}" y="${y + 10}" width="${w}" height="1" fill="${LINE}"/>`,
		);
		y += 44;
	}

	const deer = banner(flag, x, y + 6, w, CARD_HEIGHT - FOOT - PAD - y);
	if (deer) {
		out.push(
			deer,
			`<text x="${x}" y="${
				CARD_HEIGHT - FOOT - 24
			}" fill="${DIM}" font-family="${MONO}" font-size="19">flying ${esc(
				clip(plot.flag, 22),
			)}</text>`,
		);
	}
	return out.join("");
}

/** The plot's deer, when it flies one the gallery still has. */
function banner(kinskode, x, y, w, h) {
	if (!kinskode || h < 40) return null;

	const rows = normalise(kinskode).split("\n");
	const wide = Math.max(...rows.map((r) => r.length), 1);
	const cell = Math.max(1, Math.min(w / wide, (h - 34) / rows.length));

	const cells = [];
	rows.forEach((row, ry) => {
		let start = 0;
		while (start < wide) {
			const char = row[start] ?? TRANSPARENT;
			let end = start;
			while (end + 1 < wide && (row[end + 1] ?? TRANSPARENT) === char) end++;

			const hex = char === TRANSPARENT ? null : hexOf(char);
			if (hex) {
				cells.push(
					`<rect x="${(x + start * cell).toFixed(1)}" y="${(
						y + ry * cell
					).toFixed(1)}" width="${((end - start + 1) * cell).toFixed(
						1,
					)}" height="${cell.toFixed(1)}" fill="${hex}"/>`,
				);
			}
			start = end + 1;
		}
	});
	return cells.join("");
}

/** A file number, so the same parcel is always the same one on the shelf. */
function fileNo(id) {
	let hash = 2166136261;
	for (const ch of String(id)) {
		hash = Math.imul(hash ^ ch.charCodeAt(0), 16777619) >>> 0;
	}
	return String(hash % 10000).padStart(4, "0");
}

function clip(text, max) {
	const s = String(text ?? "");
	return s.length > max ? `${s.slice(0, max - 1)}…` : s;
}

function noise(x, y) {
	let n = (x * 73856093) ^ (y * 19349663);
	n = (n ^ (n >>> 13)) >>> 0;
	return ((n * 1274126177) >>> 0) / 4294967296;
}
