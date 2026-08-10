/* Route only. GET /ohayou/api/card/<nick> — see lib/ohayou/player.js. */
export {
	onRequestGetCard as onRequestGet,
	onRequestHeadCard as onRequestHead,
} from "../../../../lib/ohayou/player.js";
