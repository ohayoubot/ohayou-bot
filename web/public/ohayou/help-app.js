/*
 * The handbook: the shop and the crafting tree, drawn as cards.
 *
 * The catalogue comes from items.js rather than from the api, because none of
 * it changes between publishes and the page has to read the same whether the
 * game is up or not. Signing in only fills in the two things that are yours:
 * what you hold, and what you can afford.
 */

import { load, shell } from "../shell.js";
import { nameOf } from "./catalog.js";
import { factsOf, forSale, madeNotBought, recipeFor } from "./items.js";
import { spriteURL } from "./sprites.js";

const $ = (sel) => document.querySelector(sel);

/** Ohayous on hand, or null for "do not judge anything". */
let purse = null;
let held = {};

const shop = forSale();

async function start() {
	$("#made").replaceChildren(...madeNotBought().map(card));
	draw();

	$("#purse").addEventListener("input", (e) => {
		const n = Number.parseInt(e.target.value, 10);
		purse = Number.isFinite(n) && n >= 0 ? n : null;
		afford();
	});
	$("#clearpurse").addEventListener("click", () => {
		$("#purse").value = "";
		purse = null;
		afford();
	});
	$("#order").addEventListener("change", draw);

	const session = await shell({ area: "ohayou", current: "/ohayou/" });
	if (session) await standing();
}

/** Fills in the purse and what you hold, once, from your own row. */
async function standing() {
	const yours = await load("/ohayou/api/me");
	if (!yours || yours.status === "unclaimed") return;

	held = yours.items ?? {};
	purse = yours.ohayous;
	$("#purse").value = String(yours.ohayous);
	$("#signedout").textContent =
		`Filled in from your file: ${yours.ohayous.toLocaleString()} ohayous on hand.`;
	draw();
}

function draw() {
	const order = $("#order").value;
	const items =
		order === "kind"
			? [...shop].sort(
					(a, b) =>
						a.category.localeCompare(b.category) ||
						a.price - b.price ||
						a.name.localeCompare(b.name),
				)
			: shop;

	$("#shop").replaceChildren(...items.map(card));
	afford();
}

/* ---- one card ---- */

function card(item) {
	const el = document.createElement("article");
	el.className = "ticket";
	el.dataset.item = item.name;

	const art = document.createElement("img");
	art.className = "art";
	art.src = spriteURL(item.name);
	art.alt = "";
	art.width = 48;
	art.height = 48;

	const head = document.createElement("div");
	head.className = "top";
	const title = document.createElement("h3");
	title.textContent = nameOf(item.name);
	head.append(title, item.purchase ? price(item) : made(item));

	const body = document.createElement("div");
	body.className = "body";
	body.append(head, kind(item.category), blurb(item.desc));

	const facts = factsOf(item);
	if (facts.length) body.append(tags(facts));

	const recipe = recipeFor(item.name);
	if (recipe) body.append(needs(recipe));

	const yours = held[item.name];
	if (yours) body.append(owned(yours, item));

	el.append(art, body);
	return el;
}

function price(item) {
	const el = document.createElement("p");
	el.className = "price";
	const n = document.createElement("b");
	n.textContent = item.price.toLocaleString();
	el.append(n, unit());

	// How far the purse has got towards this, filled in by afford(). Only drawn
	// for something out of reach: on anything else it is a full bar saying what
	// the lit border already says.
	const bar = document.createElement("span");
	bar.className = "worth";
	bar.hidden = true;
	bar.setAttribute("aria-hidden", "true");
	bar.append(document.createElement("i"));
	el.append(bar);
	return el;
}

function made(item) {
	const el = document.createElement("p");
	el.className = "price made";
	el.textContent = item.name === "oilbarrel" ? "pumped" : "built";
	return el;
}

function unit() {
	const el = document.createElement("span");
	el.className = "unit";
	el.textContent = "ohayous";
	return el;
}

function kind(category) {
	const el = document.createElement("p");
	el.className = "kind";
	el.textContent = category;
	return el;
}

function blurb(text) {
	const el = document.createElement("p");
	el.className = "blurb";
	el.textContent = text;
	return el;
}

function tags(facts) {
	const list = document.createElement("ul");
	list.className = "facts";
	list.replaceChildren(
		...facts.map((fact) => {
			const li = document.createElement("li");
			li.textContent = fact;
			return li;
		}),
	);
	return list;
}

/** What a build costs: the ohayous, the metals, and the parts, drawn. */
function needs(recipe) {
	const list = document.createElement("ul");
	list.className = "needs";

	if (recipe.ohayous) {
		list.append(chip(null, recipe.ohayous.toLocaleString(), "ohayous"));
	}
	for (const [metal, n] of Object.entries(recipe.metals)) {
		list.append(chip(null, `${n}×`, metal, metal));
	}
	for (const [part, n] of Object.entries(recipe.items)) {
		list.append(chip(part, `${n}×`, nameOf(part, n)));
	}

	const wrap = document.createElement("div");
	wrap.className = "recipe";
	const label = document.createElement("p");
	label.className = "label";
	label.textContent = "takes";
	wrap.append(label, list);
	return wrap;
}

/** One input: its sprite when it has one, an ore swatch when it is a metal. */
function chip(item, count, label, ore = null) {
	const li = document.createElement("li");

	if (item) {
		const img = document.createElement("img");
		img.src = spriteURL(item);
		img.alt = "";
		img.width = 20;
		img.height = 20;
		li.append(img);
	} else if (ore) {
		const swatch = document.createElement("i");
		swatch.className = "ore";
		swatch.dataset.ore = ore;
		li.append(swatch);
	}

	const n = document.createElement("b");
	n.textContent = count;
	const name = document.createElement("span");
	name.textContent = label;
	li.append(n, name);
	return li;
}

function owned(n, item) {
	const el = document.createElement("p");
	el.className = "owned";
	el.textContent = `You have ${n.toLocaleString()} ${nameOf(item.name, n)}.`;
	return el;
}

/* ---- what your purse reaches ---- */

function afford() {
	const cards = $("#shop").querySelectorAll(".ticket");
	let within = 0;

	for (const el of cards) {
		const item = shop.find((i) => i.name === el.dataset.item);
		const reach = purse === null || purse >= item.price;
		if (reach) within++;
		el.classList.toggle("beyond", purse !== null && !reach);
		el.classList.toggle("within", purse !== null && reach);

		const short = el.querySelector(".short");
		if (short) short.remove();

		const bar = el.querySelector(".worth");
		bar.hidden = reach;
		if (!reach) {
			bar.querySelector("i").style.width = `${(purse / item.price) * 100}%`;

			const gap = document.createElement("p");
			gap.className = "short";
			gap.textContent = `${(item.price - purse).toLocaleString()} short`;
			el.querySelector(".price").append(gap);
		}
	}

	$("#reach").textContent =
		purse === null
			? `${cards.length} things for sale.`
			: `${within} of ${cards.length} within reach.`;
}

start();
