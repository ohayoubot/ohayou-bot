/*
 * The kinskode format: one character per cell, lines separated by \n.
 *   ' '      black (IRC colour 01)
 *   'A'-'O'  the other 15 mIRC colours
 *   '_'      transparent
 *
 */

export const MAX_ROWS = 30;
export const MAX_COLS = 40;

export const TRANSPARENT = "_";

export const PALETTE = [
	{ kins: " ", irc: "01", hex: "#000000", name: "black" },
	{ kins: "A", irc: "00", hex: "#FFFFFF", name: "white" },
	{ kins: "B", irc: "02", hex: "#00007F", name: "navy" },
	{ kins: "C", irc: "03", hex: "#009300", name: "green" },
	{ kins: "D", irc: "04", hex: "#FF0000", name: "red" },
	{ kins: "E", irc: "05", hex: "#7F0000", name: "maroon" },
	{ kins: "F", irc: "06", hex: "#9C009C", name: "purple" },
	{ kins: "G", irc: "07", hex: "#FC7E00", name: "orange" },
	{ kins: "H", irc: "08", hex: "#FFFF00", name: "yellow" },
	{ kins: "I", irc: "09", hex: "#00FC00", name: "lime" },
	{ kins: "J", irc: "10", hex: "#009393", name: "teal" },
	{ kins: "K", irc: "11", hex: "#00FFFF", name: "cyan" },
	{ kins: "L", irc: "12", hex: "#0000FC", name: "blue" },
	{ kins: "M", irc: "13", hex: "#FF00FF", name: "magenta" },
	{ kins: "N", irc: "14", hex: "#7F7F7F", name: "grey" },
	{ kins: "O", irc: "15", hex: "#D2D2D2", name: "silver" },
];

export const KINS_CHARS = PALETTE.map((c) => c.kins).concat(TRANSPARENT);

const HEX_BY_KINS = new Map(PALETTE.map((c) => [c.kins, c.hex]));
const IRC_BY_KINS = new Map(PALETTE.map((c) => [c.kins, c.irc]));
const KINS_BY_IRC = new Map(PALETTE.map((c) => [c.irc, c.kins]));

export function hexOf(char) {
	return HEX_BY_KINS.get(char) ?? null;
}

const COLOR = "\x03"; // mIRC colour prefix (^C)
const RESET = "\x0f"; // mIRC reset (^O)
const FILL = "@";

export function normalise(kinskode) {
	const lines = String(kinskode)
		.replace(/\r\n?/g, "\n")
		.toUpperCase()
		.split("\n")
		.map((line) =>
			Array.from(line, (ch) =>
				KINS_CHARS.includes(ch) ? ch : TRANSPARENT,
			).join(""),
		);

	while (lines.length && /^_*$/.test(lines[lines.length - 1])) lines.pop();
	return lines.join("\n");
}

/** Returns null when safe to store, otherwise the reason. Run on the server. */
export function validate(kinskode) {
	if (typeof kinskode !== "string") return "kinskode must be a string";
	if (kinskode.length === 0) return "kinskode is empty";
	if (kinskode.length > (MAX_COLS + 1) * MAX_ROWS)
		return "kinskode is too large";

	const lines = kinskode.split("\n");
	if (lines.length > MAX_ROWS) return `too many rows (max ${MAX_ROWS})`;

	for (const line of lines) {
		if (line.length > MAX_COLS) return `line too long (max ${MAX_COLS})`;
		for (const ch of line) {
			if (!KINS_CHARS.includes(ch))
				return "kinskode contains an illegal character";
		}
	}

	if (!/[^_\n]/.test(kinskode)) return "the canvas is blank";
	return null;
}

/** Repeated cells collapse to a bare '@'. Colour state resets per line. */
export function toIrc(kinskode) {
	return kinskode
		.split("\n")
		.map((line) => {
			let out = "";
			let last = null;
			for (const ch of line) {
				const irc = IRC_BY_KINS.get(ch) ?? "00"; // transparent renders as white
				if (irc === last) {
					out += FILL;
				} else {
					out += `${COLOR}${irc},${irc}${FILL}`;
					last = irc;
				}
			}
			return out + RESET;
		})
		.join("\n");
}

export function fromIrc(irccode) {
	return String(irccode)
		.replace(/\r\n?/g, "\n")
		.split("\n")
		.map((line) => {
			let out = "";
			let current = " ";
			// ^Cnn[,nn] sets the colour; nn,nn without ^C does too; @ paints a cell.
			const token = /\x03(\d{1,2})(?:,(\d{1,2}))?|(\d{1,2}),(\d{1,2})|@|\x0f/g;
			let m;
			// biome-ignore lint/suspicious/noAssignInExpressions: the standard exec loop
			while ((m = token.exec(line)) !== null) {
				if (m[0] === FILL) {
					out += out.length < MAX_COLS ? current : "";
				} else if (m[0] === RESET) {
					current = " ";
				} else {
					const bg = m[2] ?? m[4] ?? m[1] ?? m[3];
					current = KINS_BY_IRC.get(String(bg).padStart(2, "0")) ?? " ";
				}
			}
			return out;
		})
		.join("\n");
}

export function autoCrop(kinskode) {
	const lines = kinskode.split("\n");
	let top = 0;
	let bottom = lines.length - 1;
	while (top <= bottom && /^_*$/.test(lines[top])) top++;
	while (bottom >= top && /^_*$/.test(lines[bottom])) bottom--;
	if (top > bottom) return "";

	const rows = lines.slice(top, bottom + 1);
	const width = Math.max(...rows.map((r) => r.length));
	let left = 0;
	let right = width - 1;
	const columnBlank = (x) =>
		rows.every((r) => (r[x] ?? TRANSPARENT) === TRANSPARENT);
	while (left <= right && columnBlank(left)) left++;
	while (right >= left && columnBlank(right)) right--;

	return rows
		.map((r) => r.padEnd(width, TRANSPARENT).slice(left, right + 1))
		.join("\n");
}

export function measure(kinskode) {
	const lines = kinskode.split("\n");
	return {
		rows: lines.length,
		cols: Math.max(0, ...lines.map((l) => l.length)),
	};
}

const INVERSE = {
	" ": "A",
	A: " ",
	B: "H",
	C: "M",
	D: "J",
	E: "K",
	F: "I",
	G: "L",
	H: "B",
	I: "M",
	J: "D",
	K: "E",
	L: "H",
	M: "I",
	N: "O",
	O: "N",
	_: " ",
};

const split = (k) => k.split("\n");
const join = (l) => l.join("\n");
const widest = (lines) => Math.max(0, ...lines.map((l) => l.length));

export const transforms = {
	/** i: swap every colour for its opposite. */
	invert: (k) =>
		Array.from(k, (ch) => (ch === "\n" ? ch : (INVERSE[ch] ?? ch))).join(""),

	/** r: flip horizontally. */
	reverse: (k) => join(split(k).map((l) => Array.from(l).reverse().join(""))),

	/** u: flip vertically. */
	upsidedown: (k) => join(split(k).reverse()),

	/** f: transpose (rows become columns). */
	flip: (k) => {
		const lines = split(k);
		const width = widest(lines);
		const out = [];
		for (let x = 0; x < width; x++) {
			out.push(lines.map((l) => l[x] ?? TRANSPARENT).join(""));
		}
		return out.length ? join(out) : k;
	},

	/** m: mirror the left half onto the right. */
	mirror: (k) => halves(k, 0),

	/** n: mirror the right half onto the left. */
	unitinu: (k) => halves(k, 1),

	/** d: swap the halves, reversing the left one. */
	divide: (k) => halves(k, 2),

	/** s: rotate each row about its centre, and the rows about theirs. */
	square: (k) => {
		const lines = split(k);
		const half = Math.floor(widest(lines) / 2);
		if (half < 1) return k;
		const out = [];
		lines.forEach((line, i) => {
			const shifted = line.slice(half) + line.slice(0, half);
			if (i < Math.floor(lines.length / 2)) out.push(shifted);
			else out.unshift(shifted);
		});
		return join(out);
	},
};

function halves(kinskode, direction) {
	const lines = split(kinskode);
	const half = Math.floor(widest(lines) / 2);
	if (half < 1) return kinskode;

	const rev = (s) => Array.from(s).reverse().join("");
	return join(
		lines.map((line) => {
			if (direction === 0) return rev(line).slice(0, half) + line.slice(half);
			if (direction === 1) return line.slice(0, half) + rev(line).slice(half);
			return rev(line).slice(half) + line.slice(0, half);
		}),
	);
}

export const MAX_NAME = 48;
export const MAX_CREATOR = 32;

export function sanitiseName(name) {
	return String(name ?? "")
		.toLowerCase()
		.replace(/[^a-z0-9 \-_]/g, "")
		.replace(/\s+/g, " ")
		.trim()
		.slice(0, MAX_NAME);
}

export function sanitiseCreator(creator) {
	return String(creator ?? "")
		.replace(/[\p{C}<>]/gu, "")
		.replace(/\s+/g, " ")
		.trim()
		.slice(0, MAX_CREATOR);
}

export function nameSuffix() {
	const bytes = crypto.getRandomValues(new Uint8Array(5));
	return Array.from(bytes, (b) => String.fromCharCode(97 + (b % 26))).join("");
}
