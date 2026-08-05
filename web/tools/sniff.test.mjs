import assert from "node:assert/strict";
import test from "node:test";
import { SNIFF_BYTES, sniffImage } from "../lib/sniff.js";

/** A header followed by filler, the way a real file arrives. */
function file(...header) {
	return new Uint8Array([...header.flat(), ...new Uint8Array(64)]);
}

const ascii = (text) => [...text].map((c) => c.charCodeAt(0));

const PNG = file(0x89, ascii("PNG"), 0x0d, 0x0a, 0x1a, 0x0a);
const JPEG = file(0xff, 0xd8, 0xff, 0xe0);
const GIF = file(ascii("GIF89a"));
const WEBP = file(ascii("RIFF"), [0x24, 0x00, 0x00, 0x00], ascii("WEBP"));

test("the four accepted formats are recognised", () => {
	assert.deepEqual(sniffImage(PNG), { mime: "image/png", ext: "png" });
	assert.deepEqual(sniffImage(JPEG), { mime: "image/jpeg", ext: "jpg" });
	assert.deepEqual(sniffImage(GIF), { mime: "image/gif", ext: "gif" });
	assert.deepEqual(sniffImage(WEBP), { mime: "image/webp", ext: "webp" });
});

test("both gif versions are recognised", () => {
	assert.equal(sniffImage(file(ascii("GIF87a"))).mime, "image/gif");
	assert.equal(sniffImage(file(ascii("GIF89a"))).mime, "image/gif");
	assert.equal(sniffImage(file(ascii("GIF88a"))), null);
});

test("jpeg is recognised whatever segment follows the marker", () => {
	for (const next of [0xe0, 0xe1, 0xdb, 0xee]) {
		assert.equal(sniffImage(file(0xff, 0xd8, 0xff, next)).mime, "image/jpeg");
	}
});

test("other riff containers are not webp", () => {
	const wave = file(ascii("RIFF"), [0x24, 0, 0, 0], ascii("WAVE"));
	const avi = file(ascii("RIFF"), [0x24, 0, 0, 0], ascii("AVI "));
	assert.equal(sniffImage(wave), null);
	assert.equal(sniffImage(avi), null);
});

test("formats we do not accept are refused", () => {
	const cases = {
		svg: ascii('<svg xmlns="http://www.w3.org/2000/svg">'),
		svgWithBom: [0xef, 0xbb, 0xbf, ...ascii("<svg>")],
		html: ascii("<!doctype html><script>alert(1)</script>"),
		bmp: ascii("BM"),
		tiff: [0x49, 0x49, 0x2a, 0x00],
		ico: [0x00, 0x00, 0x01, 0x00],
		pdf: ascii("%PDF-1.7"),
		zip: ascii("PK\x03\x04"),
		elf: [0x7f, ...ascii("ELF")],
		text: ascii("just some words"),
	};

	for (const [name, header] of Object.entries(cases)) {
		assert.equal(sniffImage(file(header)), null, name);
	}
});

test("a signature must be complete", () => {
	// One byte short of png, and of webp's second half.
	assert.equal(
		sniffImage(new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a])),
		null,
	);
	assert.equal(
		sniffImage(new Uint8Array([...ascii("RIFF"), 0, 0, 0, 0, ...ascii("WEB")])),
		null,
	);
});

test("nothing at all is refused", () => {
	assert.equal(sniffImage(new Uint8Array(0)), null);
	assert.equal(sniffImage(new Uint8Array(SNIFF_BYTES)), null);
	assert.equal(sniffImage(null), null);
	assert.equal(sniffImage(undefined), null);
	assert.equal(sniffImage("GIF89a"), null);
	assert.equal(sniffImage(PNG.buffer), null);
	assert.equal(sniffImage([...PNG]), null);
});

test("a polyglot still sniffs as its header", () => {
	// Documenting the limit rather than pretending otherwise: this is a gif by
	// every sniffing rule there is. It is safe because of where it is served
	// from and what it is served as, not because of this function.
	const polyglot = new Uint8Array([
		...ascii("GIF89a"),
		...ascii("/*<script>alert(1)</script>*/"),
	]);
	assert.equal(sniffImage(polyglot).mime, "image/gif");
});
