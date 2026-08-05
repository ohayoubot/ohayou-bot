import { imageToKinskode } from "./image.js";
import {
	autoCrop,
	fromIrc,
	hexOf,
	MAX_COLS,
	MAX_ROWS,
	measure,
	normalise,
	PALETTE,
	sanitiseName,
	TRANSPARENT,
	toIrc,
	transforms,
} from "./kins.js";

const API = "api/art";
const DEFAULT_ROWS = 20;
const DEFAULT_COLS = 30;
const CELL_W = 25;
const CELL_H = 40;

const MAX_IMPORT_BYTES = 10 * 1024 * 1024;
/** Downscale to this before sampling. A 40x30 grid needs nothing bigger, and
    it keeps a 50 megapixel phone photo from becoming 200 MB of ImageData. */
const SAMPLE_MAX = 1024;

const $ = (sel) => document.querySelector(sel);

const gridEl = $("#grid");
const gridWrapEl = $(".grid-wrap");
const ditherEl = $("#dither");
const importFileEl = $("#importfile");
const statusEl = $("#status");
const kinsEl = $("#kinskode");
const ircEl = $("#irccode");
const galleryEl = $("#gallery");
const finderEl = $("#finder");
const finderOptions = $("#finder-options");
const savebox = $("#savebox");

let rows = DEFAULT_ROWS;
let cols = DEFAULT_COLS;
let cells = blank(rows, cols);
let cellEls = [];
let brush = 0; // index into PALETTE, or -1 for the eraser
let painting = false;
let listStart = 0;
let currentName = null;

function blank(r, c) {
	return Array.from({ length: r }, () => Array(c).fill(TRANSPARENT));
}

function brushChar() {
	return brush < 0 ? TRANSPARENT : PALETTE[brush].kins;
}

function classFor(char) {
	const i = PALETTE.findIndex((c) => c.kins === char);
	return i < 0 ? "cell k-t" : `cell k-${i}`;
}

function toKinskode() {
	return cells.map((row) => row.join("")).join("\n");
}

/** Replace the canvas, growing the grid if the art needs it. */
function setKinskode(kinskode, { announce = true } = {}) {
	const clean = normalise(kinskode);
	const size = measure(clean);
	const needRows = Math.min(MAX_ROWS, Math.max(DEFAULT_ROWS, size.rows));
	const needCols = Math.min(MAX_COLS, Math.max(DEFAULT_COLS, size.cols));

	if (needRows !== rows || needCols !== cols) {
		rows = needRows;
		cols = needCols;
		buildGrid();
	}

	cells = blank(rows, cols);
	clean.split("\n").forEach((line, y) => {
		if (y >= rows) return;
		Array.from(line).forEach((ch, x) => {
			if (x < cols) cells[y][x] = ch;
		});
	});

	paintAll();
	syncCodes();
	if (announce && (size.rows > MAX_ROWS || size.cols > MAX_COLS)) {
		say(
			`That art is bigger than ${MAX_COLS}×${MAX_ROWS} and was cropped.`,
			true,
		);
	}
}

function syncCodes() {
	const kins = toKinskode();
	kinsEl.value = kins;
	ircEl.value = toIrc(autoCrop(kins) || kins);
}

function buildGrid() {
	gridEl.style.setProperty("--cols", String(cols));
	const frag = document.createDocumentFragment();
	cellEls = [];

	for (let y = 0; y < rows; y++) {
		const rowEls = [];
		for (let x = 0; x < cols; x++) {
			const el = document.createElement("div");
			el.className = "cell k-t";
			el.dataset.x = String(x);
			el.dataset.y = String(y);
			frag.append(el);
			rowEls.push(el);
		}
		cellEls.push(rowEls);
	}

	gridEl.replaceChildren(frag);
}

function paintAll() {
	for (let y = 0; y < rows; y++) {
		for (let x = 0; x < cols; x++) {
			cellEls[y][x].className = classFor(cells[y][x]);
		}
	}
}

function paintCell(x, y) {
	if (y < 0 || y >= rows || x < 0 || x >= cols) return;
	const char = brushChar();
	if (cells[y][x] === char) return;
	cells[y][x] = char;
	cellEls[y][x].className = classFor(char);
}

function cellAt(event) {
	const el = document.elementFromPoint(event.clientX, event.clientY);
	if (!el?.classList.contains("cell")) return null;
	return { x: Number(el.dataset.x), y: Number(el.dataset.y) };
}

gridEl.addEventListener("pointerdown", (e) => {
	if (e.button !== 0 && e.pointerType === "mouse") return;
	const at = cellAt(e);
	if (!at) return;
	e.preventDefault();
	painting = true;
	gridEl.setPointerCapture(e.pointerId);
	paintCell(at.x, at.y);
});

gridEl.addEventListener("pointermove", (e) => {
	if (!painting) return;
	const at = cellAt(e);
	if (at) paintCell(at.x, at.y);
});

for (const type of ["pointerup", "pointercancel"]) {
	gridEl.addEventListener(type, () => {
		if (!painting) return;
		painting = false;
		syncCodes();
	});
}

// A drag that ends outside the grid still has to commit.
window.addEventListener("pointerup", () => {
	if (!painting) return;
	painting = false;
	syncCodes();
});

function buildPalette() {
	const frag = document.createDocumentFragment();

	PALETTE.forEach((colour, i) => {
		const b = document.createElement("button");
		b.type = "button";
		b.className = `swatch k-${i}`;
		b.setAttribute("role", "radio");
		b.title = `${colour.name} (${colour.kins === " " ? "space" : colour.kins})`;
		b.setAttribute("aria-label", colour.name);
		b.addEventListener("click", () => pick(i));
		frag.append(b);
	});

	const eraser = document.createElement("button");
	eraser.type = "button";
	eraser.className = "swatch k-t";
	eraser.setAttribute("role", "radio");
	eraser.title = "Erase (transparent)";
	eraser.setAttribute("aria-label", "erase");
	eraser.addEventListener("click", () => pick(-1));
	frag.append(eraser);

	$("#palette").replaceChildren(frag);
	pick(0);
}

function pick(i) {
	brush = i;
	const swatches = document.querySelectorAll(".swatch");
	swatches.forEach((el, n) => {
		const isEraser = n === swatches.length - 1;
		const selected = isEraser ? i === -1 : n === i;
		el.setAttribute("aria-checked", String(selected));
	});
}

document.addEventListener("click", (e) => {
	const el = e.target.closest(
		"[data-tool], [data-transform], [data-load], [data-export]",
	);
	if (!el) return;

	if (el.dataset.tool === "fill") {
		cells = cells.map((row) => row.fill(brushChar()));
		paintAll();
		syncCodes();
	} else if (el.dataset.tool === "clear") {
		if (!confirm("Clear the canvas? This cannot be undone.")) return;
		cells = blank(rows, cols);
		paintAll();
		syncCodes();
	} else if (el.dataset.transform) {
		const fn = transforms[el.dataset.transform];
		if (fn) setKinskode(fn(toKinskode()), { announce: false });
	} else if (el.dataset.load === "kinskode") {
		setKinskode(kinsEl.value);
	} else if (el.dataset.load === "irccode") {
		setKinskode(fromIrc(ircEl.value));
	} else if (el.dataset.export) {
		exportArt(el.dataset.export);
	}
});

$("#copy-irc").addEventListener("click", async () => {
	try {
		await navigator.clipboard.writeText(ircEl.value);
		say("IRC codes copied.");
	} catch {
		ircEl.select();
		say("Press ⌘/Ctrl+C to copy.");
	}
});

function exportArt(format) {
	const kins = autoCrop(toKinskode());
	if (!kins) return say("The canvas is empty.", true);

	const name = currentName || "deerkins";
	if (format === "svg")
		return download(
			new Blob([svgOf(kins)], { type: "image/svg+xml" }),
			`${name}.svg`,
		);

	const lines = kins.split("\n");
	const { cols: w, rows: h } = measure(kins);
	const canvas = document.createElement("canvas");
	canvas.width = w * CELL_W;
	canvas.height = h * CELL_H;
	const ctx = canvas.getContext("2d");

	if (format === "jpg") {
		ctx.fillStyle = "#ffffff"; // JPEG has no alpha
		ctx.fillRect(0, 0, canvas.width, canvas.height);
	}

	lines.forEach((line, y) => {
		Array.from(line).forEach((ch, x) => {
			const hex = hexOf(ch);
			if (!hex) return;
			ctx.fillStyle = hex;
			ctx.fillRect(x * CELL_W, y * CELL_H, CELL_W, CELL_H);
		});
	});

	const type = format === "jpg" ? "image/jpeg" : "image/png";
	canvas.toBlob(
		(blob) => blob && download(blob, `${name}.${format}`),
		type,
		0.92,
	);
}

function svgOf(kins) {
	const { cols: w, rows: h } = measure(kins);
	const rects = kins
		.split("\n")
		.flatMap((line, y) =>
			Array.from(line).flatMap((ch, x) => {
				const hex = hexOf(ch);
				return hex
					? `<rect x="${x * CELL_W}" y="${y * CELL_H}" width="${CELL_W}" height="${CELL_H}" fill="${hex}"/>`
					: [];
			}),
		)
		.join("\n");

	return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="${w * CELL_W}" height="${h * CELL_H}" shape-rendering="crispEdges">
${rects}
</svg>`;
}

function download(blob, filename) {
	const url = URL.createObjectURL(blob);
	const a = document.createElement("a");
	a.href = url;
	a.download = filename;
	a.click();
	setTimeout(() => URL.revokeObjectURL(url), 10_000);
}

/* ---- image import ---- */

/**
 * Draw the bitmap into a canvas and read it back. createImageBitmap takes the
 * File directly, which matters: the CSP in public/_headers allows img-src
 * 'self' data: only, so an <img> pointed at a blob: URL would be refused.
 */
function readPixels(bitmap) {
	const scale = Math.min(1, SAMPLE_MAX / Math.max(bitmap.width, bitmap.height));
	const w = Math.max(1, Math.round(bitmap.width * scale));
	const h = Math.max(1, Math.round(bitmap.height * scale));

	const canvas = document.createElement("canvas");
	canvas.width = w;
	canvas.height = h;

	const ctx = canvas.getContext("2d", { willReadFrequently: true });
	ctx.imageSmoothingQuality = "high";
	ctx.drawImage(bitmap, 0, 0, w, h);
	return ctx.getImageData(0, 0, w, h);
}

async function importImage(file) {
	if (!file) return;
	if (!file.type.startsWith("image/"))
		return say(`${file.name} is not an image.`, true);
	// Rasterising an untrusted SVG is a bigger surface than this needs, and one
	// without an intrinsic size decodes differently in every browser.
	if (file.type === "image/svg+xml") return say("SVG is not supported.", true);
	if (file.size > MAX_IMPORT_BYTES)
		return say("That image is over 10 MB.", true);

	say(`Importing ${file.name}...`);

	let bitmap;
	try {
		bitmap = await createImageBitmap(file);
	} catch {
		return say("That image could not be decoded.", true);
	}

	let kins;
	try {
		kins = imageToKinskode(readPixels(bitmap), { dither: ditherEl.checked });
	} finally {
		bitmap.close();
	}

	if (!autoCrop(kins)) return say("That image is entirely transparent.", true);

	setKinskode(kins, { announce: false });
	currentName = sanitiseName(file.name.replace(/\.[^.]+$/, "")) || null;
	history.replaceState(null, "", location.pathname + location.search);

	const { rows: h, cols: w } = measure(kins);
	say(`Imported ${file.name} at ${w}×${h}.`);
}

const hasFiles = (e) =>
	Array.from(e.dataTransfer?.types ?? []).includes("Files");

// dragenter/dragleave fire for every element crossed, so count depth rather
// than toggling on each one.
let dragDepth = 0;

function setDragging(on) {
	gridWrapEl.classList.toggle("dragover", on);
}

window.addEventListener("dragenter", (e) => {
	if (!hasFiles(e)) return;
	e.preventDefault();
	dragDepth++;
	setDragging(true);
});

window.addEventListener("dragover", (e) => {
	// Without this the browser navigates away to the dropped file.
	if (!hasFiles(e)) return;
	e.preventDefault();
	e.dataTransfer.dropEffect = "copy";
});

window.addEventListener("dragleave", (e) => {
	if (!hasFiles(e)) return;
	dragDepth = Math.max(0, dragDepth - 1);
	if (!dragDepth) setDragging(false);
});

window.addEventListener("drop", (e) => {
	if (!hasFiles(e)) return;
	e.preventDefault();
	dragDepth = 0;
	setDragging(false);
	importImage(e.dataTransfer.files[0]);
});

window.addEventListener("paste", (e) => {
	if (e.target.closest("input, textarea")) return;
	const file = e.clipboardData?.files?.[0];
	if (!file) return;
	e.preventDefault();
	importImage(file);
});

// Dropping is not reachable from a keyboard, so keep a real file picker too.
$("#import").addEventListener("click", () => importFileEl.click());

importFileEl.addEventListener("change", () => {
	importImage(importFileEl.files[0]);
	importFileEl.value = ""; // so the same file can be picked twice
});

async function api(path, options) {
	const res = await fetch(path, options);
	const body = await res
		.json()
		.catch(() => ({ status: "error", error: `HTTP ${res.status}` }));
	if (!res.ok) throw new Error(body.error || `HTTP ${res.status}`);
	return body;
}

function say(message, isError = false) {
	statusEl.textContent = message;
	statusEl.classList.toggle("error", isError);
}

$("#save").addEventListener("click", () => {
	if (!autoCrop(toKinskode())) return say("Draw something first.", true);
	$("#artname").value = currentName ?? "";
	savebox.showModal();
});

$("#cancel-save").addEventListener("click", () => savebox.close());

$("#saveform").addEventListener("submit", async (e) => {
	e.preventDefault();
	savebox.close();
	say("Saving...");

	try {
		const saved = await api(API, {
			method: "POST",
			headers: { "content-type": "application/json" },
			body: JSON.stringify({
				name: $("#artname").value,
				creator: $("#artcreator").value,
				kinskode: autoCrop(toKinskode()),
			}),
		});

		currentName = saved.name;
		location.hash = encodeURIComponent(saved.name);
		kinsEl.value = saved.kinskode;
		ircEl.value = saved.irccode;
		say(`Saved as ${saved.name}.`);
		listStart = 0;
		loadGallery();
	} catch (err) {
		say(err.message, true);
	}
});

async function loadGallery() {
	galleryEl.replaceChildren(item("Loading..."));

	try {
		const data = await api(`${API}?start=${listStart}`);
		if (!data.deer.length) {
			galleryEl.replaceChildren(
				item(listStart ? "No more art." : "No art yet."),
			);
		} else {
			galleryEl.replaceChildren(...data.deer.map(entry));
		}
		$("#prev").disabled = listStart === 0;
		$("#next").disabled = !data.more;
	} catch (err) {
		galleryEl.replaceChildren(item(err.message));
	}
}

/** Status line */
function item(text) {
	const li = document.createElement("li");
	li.textContent = text;
	return li;
}

function entry(art) {
	const li = document.createElement("li");
	li.dataset.name = art.deer;
	if (art.deer === currentName) li.className = "current";

	const button = document.createElement("button");
	button.type = "button";

	const name = document.createElement("span");
	name.className = "name";
	name.textContent = art.deer;

	const by = document.createElement("span");
	by.className = "by";
	by.textContent = ` by ${art.creator}`;

	button.append(name, by);
	button.addEventListener("click", () => {
		location.hash = encodeURIComponent(art.deer);
	});

	li.append(button);
	return li;
}

async function loadArt(name) {
	say(`Loading ${name}...`);
	try {
		const art = await api(`${API}/${encodeURIComponent(name)}`);
		currentName = art.deer;
		setKinskode(art.kinskode);
		ircEl.value = art.irccode;
		say(`${art.deer}, by ${art.creator}.`);
		document.querySelectorAll(".gallery li").forEach((li) => {
			li.classList.toggle("current", li.dataset.name === art.deer);
		});
	} catch (err) {
		say(`Could not load ${name}: ${err.message}`, true);
	}
}

let searchTimer;
finderEl.addEventListener("input", () => {
	clearTimeout(searchTimer);
	const term = finderEl.value.trim();
	if (!term) return finderOptions.replaceChildren();

	searchTimer = setTimeout(async () => {
		try {
			const data = await api(`${API}?q=${encodeURIComponent(term)}`);
			finderOptions.replaceChildren(
				...data.deer.map((name) => {
					const option = document.createElement("option");
					option.value = name;
					return option;
				}),
			);
		} catch {
			finderOptions.replaceChildren();
		}
	}, 200);
});

// fires when a datalist suggestion is clicked or 'enter' hit on textfield.
finderEl.addEventListener("change", () => {
	const term = finderEl.value.trim();
	if (term) location.hash = encodeURIComponent(term);
});

$("#prev").addEventListener("click", () => {
	listStart = Math.max(0, listStart - 20);
	loadGallery();
});

$("#next").addEventListener("click", () => {
	listStart += 20;
	loadGallery();
});

$("#refresh").addEventListener("click", () => {
	listStart = 0;
	loadGallery();
});

function fromHash() {
	const raw = location.hash.slice(1);
	try {
		return decodeURIComponent(raw).trim();
	} catch {
		return raw.trim();
	}
}

window.addEventListener("hashchange", () => {
	const name = fromHash();
	if (name && name !== currentName) loadArt(name);
});

buildPalette();
buildGrid();
syncCodes();
loadGallery();

const initial = fromHash();
if (initial) loadArt(initial);
else say("Pick a colour and drag on the grid.");
