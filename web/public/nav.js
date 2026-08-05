/*
 * The shell's nav, built from plugins.json so a new plugin adds a link by
 * shipping an entry rather than by editing every page.
 */

const HOME = { title: "registry", path: "/ohayou/" };

export async function nav(into, current) {
	const target = typeof into === "string" ? document.querySelector(into) : into;
	if (!target) return;

	let places = [];
	try {
		const res = await fetch("/plugins.json");
		if (res.ok) places = (await res.json()).plugins ?? [];
	} catch {
		places = [];
	}

	const links = [
		{ title: "home", path: "/" },
		...places.map((p) => ({
			title: p.name === "ohayou" ? HOME.title : p.title,
			path: p.path,
		})),
	];

	target.replaceChildren(
		...links.map(({ title, path }) => {
			const a = document.createElement("a");
			a.href = path;
			a.textContent = title;
			if (path === current) a.setAttribute("aria-current", "page");
			return a;
		}),
	);
}
