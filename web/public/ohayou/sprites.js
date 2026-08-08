/*
 * Item art, in kinskode: one character per cell, the sixteen IRC colours.
 * Same format the gallery stores deer in, so kins.js renders both.
 * All sprites are 12x12.
 */

import { toDataURL } from "../deerkins/kins.js";

export const SPRITE_SIZE = 12;

/* biome-ignore-start format: the rows are the drawing. */
export const SPRITES = {
	acre: `
____________
_O_O__O__O__
_OOOOOOOOOO_
_CCCCCCCCCC_
_CICCCCICCC_
_CCCCICCCCC_
_CICCCCCCIC_
_CCCICCCICC_
_CCCCCCCCCC_
_OOOOOOOOOO_
_O__O__O__O_
____________`,

	cat: `
____________
_G__G_______
_GGGGG___G__
_G G GG_GG__
_GGGGG__GG__
_GGGGGGGGG__
__GGGGGGGG__
__GGGGGGGG__
__GG_GG_GG__
__G___G__G__
____________
____________`,

	dog: `
____________
__OOO_______
_OOOOO______
_OO OONN____
_OOOOO  N___
_OOOOOOOOO__
_OOOOOOOOOOO
_OOOOOOOOO__
_OOOOOOOO___
__NN_NN_N___
____________
____________`,

	cattery: `
____________
_____GG_____
____GGGG____
___DDDDDD___
__DDDDDDDD__
__OOOOOOOO__
__OO    OO__
__OO_  _OO__
__OOO  OOO__
__OOO  OOO__
__OOOOOOOO__
____________`,

	catnip: `
____________
______I_____
_____III____
____IICII___
___IIICIII__
____IICII___
_____ICI____
______C_____
______C_____
_____CCC____
____________
____________`,

	quarry: `
____________
_______N____
______NNN___
____N_NON___
___NON_NN___
__NNONNNN_N_
_NOONNOONNN_
_NNNNNNNNNNN
_NNNNNNNNNNN
____________
____________
____________`,

	oilwell: `
____________
_____OO_____
____O__O____
____O__O____
___O_OO_O___
___O_OO_O___
__O__OO__O__
__O_OOOO_O__
_O__OOOO__O_
_OOOOOOOOOO_
_NN      NN_
____________`,

	oilbarrel: `
____________
___NNNNNN___
___N    N___
___N    N___
___NDDDDN___
___N    N___
___N    N___
___NDDDDN___
___N    N___
___N    N___
___NNNNNN___
____________`,

	gear: `
____________
___N_NN_N___
___NNNNNN___
_NNNNNNNNNN_
_NN OOOO NN_
_NNOO  OONN_
_NNOO  OONN_
_NN OOOO NN_
_NNNNNNNNNN_
___NNNNNN___
___N_NN_N___
____________`,

	circuit: `
____________
_CCCCCCCCCC_
_CH_HH_HH_C_
_C  OOOO  C_
_CH O  O HC_
_C  O  O  C_
_CH O  O HC_
_C  OOOO  C_
_CH_HH_HH_C_
_CCCCCCCCCC_
____________
____________`,

	plastic: `
____________
____________
___KKKKKK___
__KAAKKKKK__
__KAKKKKKK__
__KKKKKKKK__
__KKKKKKKK__
__KKKKKKKK__
___KKKKKK___
____________
____________
____________`,

	steelplate: `
____________
_OOOOOOOOOO_
_O N    N O_
_OOOOOOOOOO_
_OOOOOOOOOO_
_OOOOOOOOOO_
_OOOOOOOOOO_
_OOOOOOOOOO_
_O N    N O_
_OOOOOOOOOO_
____________
____________`,

	workshop: `
____________
____________
___JJJJJJ___
__JJJJJJJJ__
_JJJJJJJJJJ_
_OOOOOOOOOO_
_OO HHHH OO_
_OO H  H OO_
_OO HHHH OO_
_OOOOOOOOOO_
____________
____________`,

	factory: `
_NN_________
_O__________
_NN_________
_NN__FFFFF__
_NN_FFFFFFF_
_FFFFFFFFFF_
_FF HH HH F_
_FF HH HH F_
_FF      FF_
_FFFFFFFFFF_
____________
____________`,

	refinery: `
____________
__D_________
__G___NNNN__
__N__NOOOON_
__N__NOOOON_
_NNN_NOOOON_
__N__NOOOON_
__N__NOOOON_
_NNN_NNNNNN_
_NNNNNNNNNN_
____________
____________`,

	vault: `
____________
_NNNNNNNNNN_
_NOOOOOOOON_
_NO  N   ON_
_NO HHN  ON_
_NO HHN  ON_
_NO  N   ON_
_NO  NN  ON_
_NOOOOOOOON_
_NNNNNNNNNN_
____________
____________`,

	vaultupgrade: `
____________
_NNNNNNNN_H_
_NOOOOOOOHHH
_NO  N  HHHH
_NO HHN__H__
_NO HHN__H__
_NO  N   ON_
_NO  NN  ON_
_NOOOOOOOON_
_NNNNNNNNNN_
____________
____________`,

	burger: `
____________
___GGGGGG___
__GAGGGGAG__
__GGGGGGGG__
__HHHHHHHH__
__IIIIIIII__
__EEEEEEEE__
__DDDDDDDD__
__GGGGGGGG__
___GGGGGG___
____________
____________`,

	pancake: `
____________
____________
_____HH_____
____HHHH____
__GGGGGGGG__
__EGGGGGGE__
__GGGGGGGG__
__EGGGGGGE__
__GGGGGGGG__
___EEEEEE___
____________
____________`,

	fortunecookie: `
____________
____________
____GGGG____
__GGGGGGGG__
_GGGAAAAGGG_
_GGGAAAAGGG_
_GGEGGGGEGG_
__GGGGGGGG__
___GG__GG___
____________
____________
____________`,

	helmet: `
____________
____NNNN____
___NNNNNN___
__NNNNNNNN__
__NN    NN__
__NNKKKKNN__
__NN    NN__
__NNNNNNNN__
___NNNNNN___
___N____N___
____________
____________`,

	gloves: `
____________
__D_D__D_D__
__DDD__DDD__
_DDDD__DDDD_
_DDDD__DDDD_
_DDDD__DDDD_
_EEEE__EEEE_
_DDDD__DDDD_
_DDDD__DDDD_
__DDD__DDD__
____________
____________`,

	vest: `
____________
__BB____BB__
_BBBB__BBBB_
_BBBBBBBBBB_
_BBOOOOOOBB_
_BBOBBBBOBB_
_BBOBBBBOBB_
_BBOOOOOOBB_
_BBBBBBBBBB_
_BBBB__BBBB_
_BBB____BBB_
____________`,

	goldenrooster: `
____________
_____DD_____
____DDDD____
___HHHHHH___
___HH HHH___
___HHHHHGG__
__HHHHHHH___
__HHHHHHHH__
__HHHHHHHH__
___HHHHHH___
____G__G____
____________`,
};
/* biome-ignore-end format: the rows are the drawing. */

/** Drawn for an item with no sprite. */
export const UNKNOWN = `
____________
____________
__GGGGGGGG__
__G      G__
__G GGGG G__
__G G  G G__
__G G  G G__
__G GGGG G__
__G      G__
__GGGGGGGG__
____________
____________`;

/** Never null: an unknown item gets UNKNOWN. */
export function spriteFor(item) {
	return SPRITES[item] ?? UNKNOWN;
}

/** Never null: an item with no sprite gets UNKNOWN. */
export function spriteURL(item) {
	return toDataURL(spriteFor(item), `item:${item}`);
}
