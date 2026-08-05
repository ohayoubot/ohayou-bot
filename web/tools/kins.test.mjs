import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import {
	decodeParam,
	escapeLike,
	intVar,
	readJson,
	rejectCrossOrigin,
} from "../lib/http.js";
import {
	autoCrop,
	fromIrc,
	MAX_COLS,
	MAX_ROWS,
	measure,
	normalise,
	sanitiseCreator,
	sanitiseName,
	toIrc,
	transforms,
	validate,
} from "../public/deerkins/kins.js";

const C = "\x03";
const O = "\x0f";

test("normalise uppercases and replaces unknown characters", () => {
	assert.equal(normalise("ab\nZ!c"), "AB\n__C");
	assert.equal(normalise("A\r\nB"), "A\nB");
});

test("normalise keeps spaces, which are black, but drops trailing blank rows", () => {
	assert.equal(normalise("  A  \n___\n__"), "  A  ");
	assert.equal(normalise("A\n_B_"), "A\n_B_");
});

test("validate rejects oversized, illegal and blank art", () => {
	assert.equal(validate("A"), null);
	assert.equal(validate(""), "kinskode is empty");
	assert.equal(validate("___\n___"), "the canvas is blank");
	assert.equal(
		validate("A".repeat(MAX_COLS + 1)),
		`line too long (max ${MAX_COLS})`,
	);
	assert.equal(
		validate(
			Array(MAX_ROWS + 1)
				.fill("A")
				.join("\n"),
		),
		`too many rows (max ${MAX_ROWS})`,
	);
	assert.equal(validate("AZA"), "kinskode contains an illegal character");
	assert.equal(validate(42), "kinskode must be a string");
});

test("toIrc collapses runs and resets state per line", () => {
	assert.equal(toIrc("AAA"), `${C}00,00@@@${O}`);
	assert.equal(toIrc("AB"), `${C}00,00@${C}02,02@${O}`);
	// the second line repeats the color code because state resets
	assert.equal(toIrc("A\nA"), `${C}00,00@${O}\n${C}00,00@${O}`);
});

test("toIrc renders transparent as white", () => {
	assert.equal(toIrc("_"), toIrc("A"));
});

test("fromIrc reverses toIrc for every palette colour", () => {
	const all = " ABCDEFGHIJKLMNO";
	assert.equal(fromIrc(toIrc(all)), all);
});

test("fromIrc reads the uncompressed form older versions wrote", () => {
	assert.equal(fromIrc(`${C}01,01@${C}07,07@${C}07,07@`), " GG");
	assert.equal(fromIrc(`${C}01,01@${C}01,01@${C}00,00@`), "  A");
});

test("fromIrc tolerates a missing colour prefix", () => {
	assert.equal(fromIrc("01,01@00,00@01,01@"), " A ");
});

test("autoCrop trims transparent margins on all four sides", () => {
	assert.equal(autoCrop("___\n_A_\n___"), "A");
	assert.equal(autoCrop("__\n__"), "");
	assert.equal(autoCrop("_AB\n_C_"), "AB\nC_");
});

test("measure reports the widest line", () => {
	assert.deepEqual(measure("AB\nABC"), { rows: 2, cols: 3 });
});

test("transforms are shape-preserving where they should be", () => {
	assert.equal(transforms.reverse("AB\nCD"), "BA\nDC");
	assert.equal(transforms.upsidedown("AB\nCD"), "CD\nAB");
	assert.equal(transforms.flip("AB\nCD"), "AC\nBD");
	assert.equal(transforms.invert("A "), " A");
	assert.equal(transforms.invert("AB\nCD"), " H\nMJ");
});

test("invert is an involution on the colours that pair up", () => {
	assert.equal(transforms.invert(transforms.invert("BHNO")), "BHNO");
});

test("transpose of a transpose is the original", () => {
	const art = "ABC\nDEF";
	assert.equal(transforms.flip(transforms.flip(art)), art);
});

test("mirror reflects the row about its centre", () => {
	assert.equal(transforms.mirror("ABCD"), "DCCD");
	assert.equal(transforms.unitinu("ABCD"), "ABBA");
});

test("sanitiseName keeps only URL-safe characters and bounds length", () => {
	assert.equal(sanitiseName("  My Art!! <script> "), "my art script");
	assert.equal(sanitiseName("../../etc/passwd"), "etcpasswd");
	assert.equal(sanitiseName("a".repeat(100)).length, 48);
	assert.equal(sanitiseName(null), "");
});

test("sanitiseCreator strips control characters but keeps unicode", () => {
	assert.equal(sanitiseCreator("ni\u0000ck"), "nick");
	assert.equal(sanitiseCreator("a\u202Eb"), "ab"); // no right-to-left override in display names
	assert.equal(
		sanitiseCreator("<img src=x onerror=alert(1)>"),
		"img src=x onerror=alert(1)",
	);
	assert.equal(sanitiseCreator("[deer]^{|}`_"), "[deer]^{|}`_"); // legal IRC nick characters survive
	assert.equal(sanitiseCreator("nick\u0007\u001B"), "nick");
	assert.equal(sanitiseCreator("  Jonas  Skovmand "), "Jonas Skovmand");
	assert.equal(sanitiseCreator("日本"), "日本");
});

/* ---- seed integrity ---- */

test("seed.sql keeps the trailing spaces that pad kinskode lines", () => {
	const seed = readFileSync(new URL("../seed.sql", import.meta.url), "utf8");
	const padded = seed.split("\n").filter((l) => / $/.test(l)).length;
	assert.ok(
		padded > 5000,
		`only ${padded} padded lines in seed.sql; trailing whitespace looks stripped`,
	);
});

/* ---- request handling ---- */

const req = (headers, body = "{}") =>
	new Request("https://hemera.day/deerkins/api/art", {
		method: "POST",
		headers,
		body,
	});

test("rejectCrossOrigin requires a JSON content type", async () => {
	assert.equal(
		rejectCrossOrigin(req({ "content-type": "application/json" })),
		null,
	);
	assert.equal(
		rejectCrossOrigin(
			req({ "content-type": "application/json; charset=utf-8" }),
		),
		null,
	);
	assert.equal(
		rejectCrossOrigin(req({ "content-type": "text/plain" })).status,
		415,
	);
	assert.equal(rejectCrossOrigin(req({})).status, 415);
});

test("rejectCrossOrigin refuses a foreign Origin", async () => {
	const same = {
		"content-type": "application/json",
		origin: "https://hemera.day",
	};
	const other = {
		"content-type": "application/json",
		origin: "https://evil.example",
	};
	assert.equal(rejectCrossOrigin(req(same)), null);
	assert.equal(rejectCrossOrigin(req(other)).status, 403);
	assert.equal(
		rejectCrossOrigin(
			req({ "content-type": "application/json", origin: "garbage" }),
		).status,
		403,
	);
});

test("readJson rejects oversized bodies by byte length, not character count", async () => {
	const multibyte = `"${"é".repeat(20)}"`;
	const { error } = await readJson(
		req({ "content-type": "application/json" }, `{"k":${multibyte}}`),
		30,
	);
	assert.equal(error, "body too large");
});

test("readJson rejects arrays and scalars", async () => {
	assert.equal(
		(await readJson(req({}, "[1,2]"))).error,
		"body must be a JSON object",
	);
	assert.equal(
		(await readJson(req({}, '"hi"'))).error,
		"body must be a JSON object",
	);
	assert.equal(
		(await readJson(req({}, "nope"))).error,
		"body is not valid JSON",
	);
	assert.deepEqual((await readJson(req({}, '{"a":1}'))).body, { a: 1 });
});

test("decodeParam turns an encoded path segment back into a name", () => {
	assert.equal(decodeParam("a%20cake"), "a cake");
	assert.equal(decodeParam("3%20deers"), "3 deers");
	assert.equal(decodeParam("deer"), "deer");
	// already-decoded input survives, so an extra pass cannot mangle a name
	assert.equal(decodeParam("a cake"), "a cake");
});

test("decodeParam reports a malformed escape rather than throwing", () => {
	assert.equal(decodeParam("a%zz"), null);
	assert.equal(decodeParam("%"), null);
	assert.equal(decodeParam(undefined), "");
});

test("a name survives the round trip the editor puts it through", () => {
	const name = "5 deers looking for someone to murder";
	assert.equal(sanitiseName(decodeParam(encodeURIComponent(name))), name);
});

test("escapeLike neutralises wildcards", () => {
	assert.equal(escapeLike("100%_x"), "100\\%\\_x");
});

test("intVar falls back on anything that is not a positive integer", () => {
	assert.equal(intVar("25", 10), 25);
	assert.equal(intVar("nonsense", 10), 10);
	assert.equal(intVar("0", 10), 10);
	assert.equal(intVar("-5", 10), 10);
	assert.equal(intVar(undefined, 10), 10);
});
