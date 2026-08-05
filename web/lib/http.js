const BASE_HEADERS = {
	"content-type": "application/json; charset=utf-8",
	"x-content-type-options": "nosniff",
	"content-security-policy": "default-src 'none'",
	"referrer-policy": "no-referrer",
};

export function json(body, { status = 200, headers = {} } = {}) {
	return new Response(JSON.stringify(body), {
		status,
		headers: { ...BASE_HEADERS, ...headers },
	});
}

export function fail(status, error) {
	return json(
		{ status: "error", error },
		{ status, headers: { "cache-control": "no-store" } },
	);
}

/** Salted SHA-256 of the client IP so no address is stored. */
export async function clientKey(request, env) {
	const ip = request.headers.get("CF-Connecting-IP") ?? "unknown";
	const salt = env.IP_SALT ?? "deerkins-unsalted";
	const digest = await crypto.subtle.digest(
		"SHA-256",
		new TextEncoder().encode(`${salt}:${ip}`),
	);
	return [...new Uint8Array(digest)]
		.map((b) => b.toString(16).padStart(2, "0"))
		.join("");
}

/** A typo in wrangler.toml must not silently disable a limit. */
export function intVar(value, fallback) {
	const n = Number.parseInt(value, 10);
	return Number.isFinite(n) && n > 0 ? n : fallback;
}

/**
 * Origin is compared against the host the request actually arrived on, not a
 * list of known domains, so this holds for every hostname the project is served
 * from: hemera.day, the pages.dev domain, preview deployments, and anything
 * added later. Do not replace it with a hardcoded allowlist. Both apps post to
 * relative urls, so a legitimate request never crosses between them.
 *
 * This covers clients that send an Origin without preflighting. Making the
 * browser preflight in the first place is the caller's job: see
 * rejectCrossOrigin, and the header the upload endpoint requires.
 */
export function rejectForeignOrigin(request) {
	const origin = request.headers.get("origin");
	if (!origin) return null;

	let host;
	try {
		host = new URL(origin).host;
	} catch {
		return fail(403, "bad origin");
	}
	if (host !== new URL(request.url).host)
		return fail(403, "cross-origin requests are not allowed");

	return null;
}

/**
 * Requiring application/json forces browsers to preflight a cross-origin POST.
 * No OPTIONS handler is exported, so the preflight fails and a third-party page
 * cannot spend a visitor's rate limit.
 */
export function rejectCrossOrigin(request) {
	const type = (request.headers.get("content-type") ?? "").toLowerCase();
	if (!type.startsWith("application/json"))
		return fail(415, "send application/json");

	return rejectForeignOrigin(request);
}

export async function readJson(request, maxBytes = 8 * 1024) {
	const declared = Number.parseInt(
		request.headers.get("content-length") ?? "",
		10,
	);
	if (Number.isFinite(declared) && declared > maxBytes)
		return { error: "body too large" };

	const text = await request.text();
	if (new TextEncoder().encode(text).length > maxBytes)
		return { error: "body too large" };

	try {
		const body = JSON.parse(text);
		if (body === null || typeof body !== "object" || Array.isArray(body)) {
			return { error: "body must be a JSON object" };
		}
		return { body };
	} catch {
		return { error: "body is not valid JSON" };
	}
}

export function escapeLike(value) {
	return value.replace(/[\\%_]/g, "\\$&");
}

/**
 * Pages hands route params over still percent-encoded, so "a%20cake" arrives
 * verbatim rather than as "a cake". Decoding is the caller's job.
 *
 * Returns null for a malformed escape ("%zz", a lone "%"), which is a bad
 * request rather than something to sanitise into a different name.
 */
export function decodeParam(value) {
	try {
		return decodeURIComponent(String(value ?? ""));
	} catch {
		return null;
	}
}

export function guard(handler) {
	return async (context) => {
		try {
			return await handler(context);
		} catch (err) {
			console.error(err);
			return fail(500, "something went wrong");
		}
	};
}
