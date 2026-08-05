/*
 * What an uploaded file actually is. The client's Content-Type is never
 * consulted and never stored; this is the only thing that decides.
 *
 * This answers "is this plausibly one of the four raster formats we accept",
 * not "is this a safe file". A gif header followed by markup is still a gif by
 * this test, and browsers have been talked into running such things. What makes
 * that harmless is the rest of the chain: the sniffed type is pinned onto the
 * object, served with nosniff, from a hostname that holds no cookie.
 */

const PNG = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];
const JPEG = [0xff, 0xd8, 0xff];
const GIF87 = [0x47, 0x49, 0x46, 0x38, 0x37, 0x61]; // GIF87a
const GIF89 = [0x47, 0x49, 0x46, 0x38, 0x39, 0x61]; // GIF89a
const RIFF = [0x52, 0x49, 0x46, 0x46]; // RIFF
const WEBP = [0x57, 0x45, 0x42, 0x50]; // WEBP, after a four byte length

/** Longest signature we look at, webp's included. */
export const SNIFF_BYTES = 12;

/**
 * Returns {mime, ext} or null. The extension travels with the type so the
 * object key cannot end up disagreeing with what it is served as.
 */
export function sniffImage(bytes) {
	if (!(bytes instanceof Uint8Array)) return null;

	if (at(bytes, PNG, 0)) return { mime: "image/png", ext: "png" };
	if (at(bytes, JPEG, 0)) return { mime: "image/jpeg", ext: "jpg" };
	if (at(bytes, GIF87, 0) || at(bytes, GIF89, 0))
		return { mime: "image/gif", ext: "gif" };
	if (at(bytes, RIFF, 0) && at(bytes, WEBP, 8))
		return { mime: "image/webp", ext: "webp" };

	return null;
}

function at(bytes, signature, offset) {
	if (bytes.length < offset + signature.length) return false;
	return signature.every((byte, i) => bytes[offset + i] === byte);
}
