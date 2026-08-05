/*
 * The upload page. A grant arrives in the url fragment, is traded for a cookie,
 * and images are staged for review, then sent one at a time to api/upload once
 * the user confirms.
 *
 * Every image is redrawn through a canvas before it is sent, which is what
 * drops exif: a phone photo carries the coordinates it was taken at. Animated
 * gifs are the exception, since a canvas would keep only the first frame.
 */

const API = "api";

/** Matches the server's cap. Anything larger is refused there anyway. */
const MAX_BYTES = 5 * 1024 * 1024;

const MAX_EDGE = 2048;

const $ = (sel) => document.querySelector(sel);

const waitingEl = $("#waiting");
const lockedEl = $("#locked");
const lockedWhyEl = $("#locked-why");
const readyEl = $("#ready");
const doneEl = $("#done");
const nickEl = $("#nick");
const accountEl = $("#account");
const channelEl = $("#channel");
const zoneEl = $("#zone");
const fileEl = $("#file");
const statusEl = $("#status");
const uploadsEl = $("#uploads");
const pendingEl = $("#pending");
const queueEl = $("#queue");
const uploadEl = $("#upload");
const discardEl = $("#discard");

let session = null;
let busy = false;

/** Files chosen but not sent yet, as { file, card }. Sending empties it. */
let staged = [];

/* ---- session ---- */

/**
 * The fragment never reaches the server's logs, and is cleared before anything
 * else runs so a shared screen or a back button does not hand the grant on.
 */
async function start() {
	show(waitingEl);

	const token = location.hash.slice(1);
	if (token) history.replaceState(null, "", location.pathname);

	try {
		session = token ? await redeem(token) : await current();
	} catch {
		return locked("Could not reach the server.");
	}

	if (!session)
		return locked(
			token ? "That link is expired, already used, or not valid." : "",
		);

	nickEl.textContent = session.nick;
	accountEl.textContent = `(${session.account})`;
	channelEl.replaceChildren(
		...session.channels.map((name) => new Option(name, name)),
	);
	show(readyEl);
}

async function redeem(token) {
	const res = await fetch(`${API}/session`, {
		method: "POST",
		headers: { "content-type": "application/json" },
		body: JSON.stringify({ token }),
	});
	return res.ok ? await res.json() : null;
}

async function current() {
	const res = await fetch(`${API}/session`);
	return res.ok ? await res.json() : null;
}

function locked(why) {
	lockedWhyEl.textContent = why;
	show(lockedEl);
}

function show(section) {
	for (const el of [waitingEl, lockedEl, readyEl]) el.hidden = el !== section;
}

$("#signout").addEventListener("click", async () => {
	await fetch(`${API}/session`, { method: "DELETE" });
	session = null;
	locked("Signed out.");
});

/* ---- preparing an image ---- */

/**
 * Returns a blob ready to send, or throws with a reason for the user.
 *
 * A gif is passed through untouched so it keeps moving; there is no exif in one
 * worth stripping. Everything else is redrawn, which both removes metadata and
 * bounds the dimensions.
 */
async function prepare(file) {
	if (!file.type.startsWith("image/")) throw new Error("not an image");
	if (file.type === "image/svg+xml") throw new Error("SVG is not supported");

	if (file.type === "image/gif") {
		if (file.size > MAX_BYTES) throw new Error("that gif is over 5 MB");
		return file;
	}

	let bitmap;
	try {
		bitmap = await createImageBitmap(file);
	} catch {
		throw new Error("could not decode that image");
	}

	try {
		const canvas = fit(bitmap);
		// Png keeps a screenshot sharp, but a photograph saved as png can come
		// out over the limit, so fall back to jpeg rather than refusing it.
		const png = file.type === "image/png";
		let blob = await encode(canvas, png ? "image/png" : "image/jpeg", 0.9);
		if (blob.size > MAX_BYTES && png)
			blob = await encode(canvas, "image/jpeg", 0.85);
		if (blob.size > MAX_BYTES) throw new Error("that image is too big");
		return blob;
	} finally {
		bitmap.close();
	}
}

function fit(bitmap) {
	const scale = Math.min(1, MAX_EDGE / Math.max(bitmap.width, bitmap.height));
	const canvas = document.createElement("canvas");
	canvas.width = Math.max(1, Math.round(bitmap.width * scale));
	canvas.height = Math.max(1, Math.round(bitmap.height * scale));

	const ctx = canvas.getContext("2d");
	ctx.imageSmoothingQuality = "high";
	ctx.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
	return canvas;
}

function encode(canvas, type, quality) {
	return new Promise((resolve, reject) => {
		canvas.toBlob(
			(blob) => (blob ? resolve(blob) : reject(new Error("could not encode"))),
			type,
			quality,
		);
	});
}

/* ---- sending ---- */

/** XHR rather than fetch, for upload.onprogress. */
function send(blob, channel, onProgress) {
	return new Promise((resolve, reject) => {
		const xhr = new XMLHttpRequest();
		xhr.open("POST", `${API}/upload?channel=${encodeURIComponent(channel)}`);
		xhr.setRequestHeader("x-drop-upload", "1");
		xhr.responseType = "json";

		xhr.upload.addEventListener("progress", (e) => {
			if (e.lengthComputable) onProgress(e.loaded / e.total);
		});
		xhr.addEventListener("load", () => {
			const body = xhr.response;
			if (xhr.status === 201) resolve(body);
			else reject(new Error(body?.error ?? `upload failed (${xhr.status})`));
		});
		xhr.addEventListener("error", () => reject(new Error("connection lost")));

		xhr.send(blob);
	});
}

/* ---- staging ---- */

/**
 * Dropped and pasted images wait here. Sending them is a deliberate second
 * step, so the wrong window or the wrong picture is a click away from being
 * discarded rather than already in the channel.
 */
function stage(files) {
	if (!session) return;
	if (busy) return say("Still sending the last batch.");

	for (const file of [...files]) staged.push({ file, card: queueCard(file) });

	// So choosing the same file again still fires a change event.
	fileEl.value = "";
	refresh();
	say(staged.length ? "Press Upload to send." : "Nothing to upload.");
}

function queueCard(file) {
	const li = document.createElement("li");
	li.className = "card";
	li.innerHTML = `<div class="thumb"></div>
    <div class="card-body">
      <p class="card-name"></p>
      <p class="card-state"></p>
    </div>
    <button type="button" class="drop-one">Remove</button>`;
	li.querySelector(".card-name").textContent = file.name;
	li.querySelector(".card-state").textContent = size(file.size);

	const remove = li.querySelector(".drop-one");
	remove.setAttribute("aria-label", `Remove ${file.name}`);
	remove.addEventListener("click", () => unstage(file));

	thumbnail(li, file);
	queueEl.append(li);
	return li;
}

function unstage(file) {
	if (busy) return;
	staged = staged.filter((item) => {
		if (item.file !== file) return true;
		item.card.remove();
		return false;
	});
	refresh();
	if (!staged.length) say("");
}

function clearStaged() {
	staged = [];
	queueEl.replaceChildren();
	refresh();
}

function refresh() {
	pendingEl.hidden = !staged.length;
	uploadEl.textContent =
		staged.length === 1 ? "Upload 1 image" : `Upload ${staged.length} images`;
	uploadEl.disabled = busy || !staged.length;
	discardEl.disabled = busy;
}

function size(bytes) {
	return bytes < 1024 * 1024
		? `${Math.max(1, Math.round(bytes / 1024))} KB`
		: `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

async function handle(files) {
	if (busy || !session) return;
	busy = true;
	refresh();

	const channel = channelEl.value;
	let queued = 0;

	for (const file of [...files]) {
		const card = addCard(file.name);
		try {
			const blob = await prepare(file);
			thumbnail(card, blob);
			const result = await send(blob, channel, (fraction) =>
				progress(card, fraction),
			);
			succeed(card, result);
			queued++;
		} catch (err) {
			failCard(card, err.message);
		}
	}

	busy = false;
	refresh();
	say(
		queued
			? `Queued for ${channel}. The bot posts it in a few seconds.`
			: "Nothing was uploaded.",
	);
}

/* ---- the list ---- */

function addCard(name) {
	const li = document.createElement("li");
	li.className = "card";
	li.innerHTML = `<div class="thumb"></div>
    <div class="card-body">
      <p class="card-name"></p>
      <p class="card-state">Preparing…</p>
    </div>`;
	li.querySelector(".card-name").textContent = name;
	uploadsEl.prepend(li);
	doneEl.hidden = false;
	return li;
}

/**
 * Draws the blob into a canvas rather than pointing an <img> at a blob: url,
 * which the CSP in public/_headers does not allow.
 */
async function thumbnail(card, blob) {
	let bitmap;
	try {
		bitmap = await createImageBitmap(blob);
	} catch {
		return;
	}
	const canvas = fitThumb(bitmap);
	bitmap.close();
	card.querySelector(".thumb").replaceChildren(canvas);
}

function fitThumb(bitmap) {
	const size = 64;
	const scale = Math.min(size / bitmap.width, size / bitmap.height);
	const canvas = document.createElement("canvas");
	canvas.width = Math.max(1, Math.round(bitmap.width * scale));
	canvas.height = Math.max(1, Math.round(bitmap.height * scale));
	canvas.getContext("2d").drawImage(bitmap, 0, 0, canvas.width, canvas.height);
	return canvas;
}

function progress(card, fraction) {
	card.querySelector(".card-state").textContent =
		`Uploading ${Math.round(fraction * 100)}%`;
}

function succeed(card, result) {
	const state = card.querySelector(".card-state");
	state.replaceChildren();

	const link = document.createElement("a");
	link.href = result.url;
	link.textContent = result.url;
	link.rel = "noreferrer";

	const copy = document.createElement("button");
	copy.type = "button";
	copy.className = "copy";
	copy.textContent = "copy";
	copy.addEventListener("click", async () => {
		await navigator.clipboard.writeText(result.url);
		copy.textContent = "copied";
	});

	state.append(link, " ", copy);
}

function failCard(card, why) {
	const state = card.querySelector(".card-state");
	state.textContent = why;
	state.classList.add("bad");
}

function say(message) {
	statusEl.textContent = message;
}

/* ---- input ---- */

zoneEl.addEventListener("click", () => fileEl.click());
zoneEl.addEventListener("keydown", (e) => {
	if (e.key === "Enter" || e.key === " ") {
		e.preventDefault();
		fileEl.click();
	}
});
fileEl.addEventListener("change", () => stage(fileEl.files));

uploadEl.addEventListener("click", () => {
	if (busy || !staged.length) return;
	const files = staged.map((item) => item.file);
	clearStaged();
	handle(files);
});

discardEl.addEventListener("click", () => {
	if (busy) return;
	clearStaged();
	say("Discarded.");
});

function dragging(on) {
	zoneEl.classList.toggle("dragover", on);
}

window.addEventListener("dragover", (e) => {
	// Without this the browser navigates away to the dropped file.
	e.preventDefault();
	if (!session) return;
	e.dataTransfer.dropEffect = "copy";
	dragging(true);
});
window.addEventListener("dragleave", (e) => {
	if (!e.relatedTarget) dragging(false);
});
window.addEventListener("drop", (e) => {
	e.preventDefault();
	dragging(false);
	if (e.dataTransfer?.files?.length) stage(e.dataTransfer.files);
});
window.addEventListener("paste", (e) => {
	const files = [...(e.clipboardData?.files ?? [])];
	if (files.length) stage(files);
});

start();
