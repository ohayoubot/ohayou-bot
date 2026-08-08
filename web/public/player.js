/* One player's page is rendered by a function; this is the only thing on it
   that needs the browser. See lib/ohayou/player.js. */

import { shell } from "./shell.js";

shell({ area: "ohayou", current: "/ohayou/" });
