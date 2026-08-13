/*
 * The shop and the crafting tree, as the handbook reads them.
 *
 * ITEMS restates data/items.json and RECIPES restates recipeList in
 * internal/plugins/ohayou/craft.go, both of which the bot owns.
 * tools/handbook.test.mjs reads those two and fails if this drifts from either.
 */

/* biome-ignore-start format: one item per block, the way the game files read. */
export const ITEMS = [
	{
		name: "acre",
		desc: "A plot of land. It caps how many animals, industries and buildings you can own.",
		price: 250,
		category: "land",
		purchase: true,
	},
	{
		name: "cat",
		desc: "A fuzzy cat. Adds 1 to every ohayou. Breedable and, sadly, stealable. Max 20 per acre.",
		price: 12,
		add: 1,
		acrelimit: 20,
		category: "animals",
		purchase: true,
	},
	{
		name: "dog",
		desc: "A loyal dog. Adds 3 to every ohayou and guards the place from thieves, though only your first three add defense. Use it to take it out digging once a day. Bad news for cats.",
		price: 250,
		add: 3,
		defense: 12,
		acrelimit: 1,
		useable: true,
		effect: "takes the dog out for a walk",
		hasFunction: "walkDog",
		category: "animals",
		purchase: true,
	},
	{
		name: "cattery",
		desc: "Use it to breed your cats for a few hours. More catteries mean bigger litters. Needs its own acre.",
		price: 4000,
		acrelimit: 1,
		needsAcre: true,
		useable: true,
		effect: "opens the cattery and lets the cats mingle",
		hasFunction: "attemptBreedCat",
		category: "animals",
		purchase: true,
	},
	{
		name: "catnip",
		desc: "Doubles the ohayous every one of your cats produces. Only one needed.",
		price: 2500,
		multiplies: "cat",
		multiply: 2,
		limit: 1,
		category: "boosts",
		purchase: true,
	},
	{
		name: "quarry",
		desc: "A mine on its own acre. Use it to spend hours digging up metals for crafting.",
		price: 3000,
		acrelimit: 1,
		needsAcre: true,
		useable: true,
		effect: "fires up the quarry and starts mining",
		hasFunction: "startMining",
		category: "industry",
		purchase: true,
	},
	{
		name: "oilwell",
		desc: "An oil well on its own acre. Use it to pump barrels of crude oil for crafting.",
		price: 1200,
		acrelimit: 1,
		needsAcre: true,
		useable: true,
		effect: "starts up the oil well and begins pumping",
		hasFunction: "startPumping",
		category: "industry",
		purchase: true,
	},
	{
		name: "oilbarrel",
		desc: "A barrel of crude oil. Craft it into plastic.",
		price: 0,
		category: "resources",
		purchase: false,
	},
	{
		name: "gear",
		desc: "A mechanical gear crafted from iron. A building block for factories.",
		price: 0,
		category: "materials",
		purchase: false,
	},
	{
		name: "circuit",
		desc: "An electronic circuit crafted from copper. Needed for advanced buildings.",
		price: 0,
		category: "materials",
		purchase: false,
	},
	{
		name: "plastic",
		desc: "A bar of plastic crafted from oil. Needed for advanced buildings.",
		price: 0,
		category: "materials",
		purchase: false,
	},
	{
		name: "steelplate",
		desc: "A steel plate smelted from iron. The backbone of heavy industry.",
		price: 0,
		category: "materials",
		purchase: false,
	},
	{
		name: "workshop",
		desc: "A crafted building that earns 3 ohayous every ration. Up to 5 per acre.",
		price: 0,
		add: 3,
		acrelimit: 5,
		category: "factories",
		purchase: false,
	},
	{
		name: "factory",
		desc: "A crafted building that earns 10 ohayous every ration. Up to 2 per acre.",
		price: 0,
		add: 10,
		acrelimit: 2,
		category: "factories",
		purchase: false,
	},
	{
		name: "refinery",
		desc: "A crafted building that earns 30 ohayous every ration. One per acre.",
		price: 0,
		add: 30,
		acrelimit: 1,
		category: "factories",
		purchase: false,
	},
	{
		name: "vault",
		desc: "A personal vault for stashing ohayous safely from thieves. Use it to install. Limit one.",
		price: 1500,
		limit: 1,
		useable: true,
		effect: "installs a shiny new vault",
		hasFunction: "makeVault",
		category: "storage",
		purchase: true,
	},
	{
		name: "vaultupgrade",
		desc: "Use it to upgrade your vault by one level, multiplying its capacity by ten.",
		price: 5000,
		useable: true,
		effect: "gets to work upgrading the vault",
		hasFunction: "upgradeVault",
		category: "storage",
		purchase: true,
	},
	{
		name: "burger",
		desc: "A tasty burger. Handy for luring a stray cat during a cat event.",
		price: 10,
		useable: true,
		consume: true,
		effect: "offers a juicy burger to %s",
		hasFunction: "adoptCat",
		category: "food",
		purchase: true,
	},
	{
		name: "pancake",
		desc: "A fluffy pancake. Also good for luring a stray cat during a cat event.",
		price: 10,
		useable: true,
		consume: true,
		effect: "offers a warm pancake to %s",
		hasFunction: "adoptCat",
		category: "food",
		purchase: true,
	},
	{
		name: "fortunecookie",
		desc: "Crack it open for your fortune. Works once a day. Purely for fun.",
		price: 50,
		useable: true,
		effect: "cracks open a fortune cookie",
		hasFunction: "fortune",
		category: "misc",
		purchase: true,
	},
	{
		name: "helmet",
		desc: "Head armor. Lowers your chance of being robbed.",
		price: 400,
		defense: 36,
		equipCategory: "head",
		category: "armor",
		purchase: true,
	},
	{
		name: "gloves",
		desc: "Sturdy gloves. Lower your chance of being robbed.",
		price: 600,
		defense: 30,
		equipCategory: "hands",
		category: "armor",
		purchase: true,
	},
	{
		name: "vest",
		desc: "Body armor. Strongly lowers your chance of being robbed.",
		price: 800,
		defense: 54,
		equipCategory: "body",
		category: "armor",
		purchase: true,
	},
	{
		name: "goldenrooster",
		desc: "A cursed charm that crows up a second dawn, letting you ohayou a second time each day. Limit one.",
		price: 100,
		limit: 1,
		useable: true,
		effect: "winds up the golden rooster at %s",
		hasFunction: "goldenRooster",
		category: "charms",
		purchase: true,
	},
];

/** The crafting tree, in the order the game offers it. */
export const RECIPES = [
	{ name: "gear", amount: 1, ohayous: 0, metals: { iron: 2 }, items: {} },
	{ name: "circuit", amount: 1, ohayous: 0, metals: { copper: 3 }, items: {} },
	{
		name: "plastic",
		amount: 1,
		ohayous: 0,
		metals: {},
		items: { oilbarrel: 2 },
	},
	{ name: "steelplate", amount: 1, ohayous: 0, metals: { iron: 5 }, items: {} },
	{
		name: "workshop",
		amount: 1,
		ohayous: 500,
		metals: {},
		items: { gear: 10 },
	},
	{
		name: "factory",
		amount: 1,
		ohayous: 2000,
		metals: {},
		items: { gear: 20, circuit: 10, steelplate: 5 },
	},
	{
		name: "refinery",
		amount: 1,
		ohayous: 5000,
		metals: { uranium: 2 },
		items: { steelplate: 5, circuit: 10, plastic: 5 },
	},
];
/* biome-ignore-end format: one item per block, the way the game files read. */

const BY_NAME = new Map(ITEMS.map((item) => [item.name, item]));
const RECIPE_BY_NAME = new Map(RECIPES.map((r) => [r.name, r]));

export function itemNamed(name) {
	return BY_NAME.get(name) ?? null;
}

export function recipeFor(name) {
	return RECIPE_BY_NAME.get(name) ?? null;
}

/** What the shop sells, cheapest first. Ties break by name so the order is stable. */
export function forSale() {
	return ITEMS.filter((item) => item.purchase).sort(
		(a, b) => a.price - b.price || a.name.localeCompare(b.name),
	);
}

/** What you never buy: the barrel the well pumps, then the crafting tree in order. */
export function madeNotBought() {
	const crafted = RECIPES.map((r) => itemNamed(r.name)).filter(Boolean);
	return [itemNamed("oilbarrel"), ...crafted].filter(Boolean);
}

/** Metals come out of the quarry, not the shop, so they have no item entry. */
export const METALS = ["iron", "copper", "uranium"];

/**
 * The plain facts about an item, as short phrases. Read off the same fields the
 * game runs on, so a change to items.json changes the handbook.
 */
export function factsOf(item) {
	const facts = [];
	if (item.add) facts.push(`+${item.add} every ration`);
	if (item.multiplies && item.multiply)
		facts.push(`${item.multiply}× what every ${item.multiplies} earns`);
	if (item.defense) facts.push(`+${item.defense} defence`);
	if (item.equipCategory) facts.push(`worn on the ${item.equipCategory}`);
	if (item.needsAcre) facts.push("needs an acre to itself");
	else if (item.acrelimit) facts.push(`${item.acrelimit} per acre`);
	if (item.limit)
		facts.push(item.limit === 1 ? "one only" : `${item.limit} only`);
	if (item.useable)
		facts.push(item.consume ? "used up when used" : "you use it");
	return facts;
}
