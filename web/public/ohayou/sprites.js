/*
 * Item art, in kinskode: one character per cell, the sixteen IRC colours.
 * Same format the gallery stores deer in, so kins.js renders both.
 * All sprites are 16x16.
 */

import { toDataURL } from "../deerkins/kins.js";

export const SPRITE_SIZE = 16;

/* biome-ignore-start format: the rows are the drawing. */
export const SPRITES = {
	acre: `
________________
__O__O__O__O__O_
_OOOOOOOOOOOOOO_
_ CCCCCCCCCCCC _
_ CIICCIICCIIC _
_ CCCCCCCCCCCC _
_ IICCIICCIICC _
_ CCCCCCCCCCCC _
_ CCIICCIICCII _
_ CCCCCCCCCCCC _
_ IICCIICCIICC _
_ CCCCCCCCCCCC _
_ CIICCIICCIIC _
_OOOOOOOOOOOOOO_
__O__O__O__O__O_
________________`,

	cat: `
________________
__ ________ ____
__ G ____ G ____
_ GGG    GGG ___
_ GGGGGGGGGG ___
_ GGHHGGHHGG ___
_ GG HGGH GG ___
_ GGAAMMAAGG ___
_ GGA AA AGG ___
_ GGGGGGGGGG ___
__ GGGGGGGG ____
__ GEAAAEGG ____
__ GGAAAGGG  GG_
__ GGGAAGGG  GG_
__ AA GG AA  GG_
________________`,

	dog: `
________________
____ OOOOOO ____
___ OOOOOOOO ___
_ NNOOOOOOOONN _
_ NNOO OO OONN _
_ NNOOOOOOOONN _
_ NNOAAAAAAONN _
_ NNOAA  AAONN _
__ OAADDAAO ____
__ OOOAAOOO ____
_ OONOAAOOO  O _
_ ONNOAAONO OO _
_ OOOOAAOOO O __
_ OOOOOOOOO ____
_ AA OO AA _____
________________`,

	cattery: `
________________
_______  _______
______ DD ______
_____ DDDD _____
____ DDDDDD ____
___ DDDDDDDD ___
__ EEEEEEEEEE __
__ OHOOOOOOHO __
__ OO      OO __
__ OO G  G OO __
__ OO GGGG OO __
__ OO GHHG OO __
__ OO GGGG OO __
__ OO GGGG OO __
__ NNNNNNNNNN __
________________`,

	catnip: `
________________
_______FF_______
______FIIF______
______ICCI______
___IIIICCIIII___
__IICIICCIICII__
___IIICCCCIII___
____IICCCCII____
__IIICICCICIII__
___IICCCCCCII___
_____ICCCCI_____
_______CC_______
_______CC_______
______ECCE______
_______CC_______
________________`,

	quarry: `
________________
_____NNNNNN_____
___NNNNNNNNNN___
__NNKNNNNNNHNN__
_NNNNNNNNNNNNNN_
_NNNNNEEEENNNNN_
_NNNNE    ENNNN_
_NNNOE H  ENNNN_
_NNNNE  K ENNNN_
NNNNNE    ENNNNN
NNKNNE    ENNHNN
NNNNNE    ENNNNN
NNNNNE    ENNNNN
NNONNE    ENNONN
NNNNNN    NNNNNN
________________`,

	oilwell: `
________________
___NNNNNNNNNN___
___O________O___
___ON______NO___
___O_N____N_O___
___O__N__N__O___
___O___NN___O___
___O__N__N__O___
___O_N____N_O___
___ONNNNNNNNO___
___O_N____N_O___
___O__N__N__O___
___O___NN___O___
__NNNNNNNNNNNN__
__N  FL  LF  N__
__NNNNNNNNNNNN__`,

	oilbarrel: `
________________
___ NNNNNNNN ___
___ AOOOOOON ___
___ OOOOOOON ___
___ DDDDDDDE ___
___ OOOOOOON ___
___ OOOOOOON ___
___ OOOOOOON ___
___ OOOOOOON ___
___ OOOOOOON ___
___ DDDDDDDE ___
___ OOOOOOON ___
___ OOOOOOON ___
___ OOOOOOON ___
___ NNNNNNNN ___
________________`,

	gear: `
____NN____NN____
____NN____NN____
___NOOOOONNNN___
__NNOOOONNNNNN__
NNNNOOONNNNNNNNN
NNNOON    NNNNNN
__NOON    NNNN__
__NON      NNN__
__NNN      NNN__
__NNNN    NNNN__
NNNNNN    NNNNNN
NNNNNNNNNNNNNNNN
__NNNNNNNNNNNN__
___NNNNNNNNNN___
____NN____NN____
____NN____NN____`,

	circuit: `
________________
__CCCCCCCCCCCC__
__CIICCCCCCIIC__
__CICCHHHHCCIC__
__CICH    HCIC__
__CCCH    HCCC__
__CICH  A HCIC__
__CCCH    HCCC__
__CICH    HCIC__
__CCCCHHHHCCCC__
__CIICCCCCCIIC__
__CCICCCCCCICC__
__CCCIIIIIICCC__
__CCCCCCCCCCCC__
__CHCHCHCHCHCC__
________________`,

	plastic: `
________________
________________
____        ____
___ KAAKKKKK ___
__ KAKKKKKKKJ __
_ KKKKKKKKKKKJ _
_ KKKKKKKKKKJJ _
_ KKKKKKKKKJJJ _
_ JKKKKKKKJJJJ _
_ JJKKKKKJJJJJ _
_ JJJJJJJJJJJJ _
__ JJJJJJJJJJ __
___ JJJJJJJJ ___
____        ____
________________
________________`,

	steelplate: `
________________
________________
_              _
_ OOOOOOOOOOOO _
_ O OOOOOOOO O _
_ OOOOOOOOOOOO _
_ OOOAAOOOOOOO _
_ OOAAOOOOOOOO _
_ OOOOOOOOOONN _
_ OOOOOOOOONNN _
_ NOOOOOOONNNN _
_ O OOOOOOOO O _
_ NNNNNNNNNNNN _
_              _
________________
________________`,

	workshop: `
________________
________________
______JJJJ______
_____JJJJJJ_____
____JJJJJJJJ____
___JJJJJJJJJJ___
__JJJJJJJJJJJJ__
_JJJJJJJJJJJJJJ_
__ OOOOOOOOOO __
__ OHHOOOOHHO __
__ OHHOOOOHHO __
__ OOOOOOOOOO __
__ OOO    OOO __
__ OOO    OOO __
__ NNNNNNNNNN __
________________`,

	factory: `
__O_____________
_OOO____________
__O_____________
_NN_____________
_DD_____________
_NN_____________
_NN_____________
_NN_FFFFFFFF____
_NNFFFFFFFFFF___
 FFFFFFFFFFFFF _
 FHHFHHFHHFHHF _
 FHHFHHFHHFHHF _
 FFFFFFFFFFFFF _
 FF      FFFFF _
 NNNNNNNNNNNNN _
________________`,

	refinery: `
__D_____________
_DGD____________
__GH____________
__ON____________
__ON__OOOOO_____
__ON__OONN _____
__ON__OONN _____
__ON__OONN _____
__ONNNOONN _____
__ON__OONN OOOO_
__ON__OONN OONN_
__ON__OONN OONN_
__ON__OONN OONN_
__ON__OONN OONN_
_NNNNNNNNNNNNNN_
________________`,

	vault: `
________________
               _
 NNNNNNNNNNNNN _
 NNNNNNNNNNNNN _
 NN OOOOOOO NN _
 NN O     O NN _
 ON O HHH O NN _
 NN O H H O NN _
 NN O HHH O NN _
 ON O     O NN _
 NN O  AA O NN _
 NN O     O NN _
 NN OOOOOOO NN _
 NNNNNNNNNNNNN _
               _
________________`,

	vaultupgrade: `
________________
               _
 NNNNNNNNNNNNN _
 NNNNNNNNNNNNN _
 NN OOOOOOO NN _
 NN O  H  O NN _
 NN O HHH O NN _
 NN OHHHHHO NN _
 NN O HHH O NN _
 NN O HHH O NN _
 NN O HHH O NN _
 NN O     O NN _
 NN OOOOOOO NN _
 NNNNNNNNNNNNN _
               _
________________`,

	burger: `
____        ____
__  GGAGGAGG  __
_ GGGAGGGGAGGG _
_ GGGGGGGGGGGE _
_ EEEEEEEEEEEE _
_ IIIIIIIIIIII _
_ IICIIICIIICI _
_ HHHHHHHHHHHH _
_ EDDDDDDDDDDE _
_ EDDDDDDDDDDE _
_ EEEEEEEEEEEE _
_ GGGGGGGGGGGG _
_ GGGGGGGGGGGE _
__  EEEEEEEE  __
____        ____
________________`,

	pancake: `
________________
______OOO_______
______OOO_______
__  HHHHHHHH  __
_ HHEHHHHHHEHH _
_ GGGGGGGGGGGG _
_ HHHHHHHHHHHH _
_ GGGGGGGGGGGG _
_ HHHHHHHHHHHH _
_ GGGGGGGGGGGG _
__ HHHHHHHHHH __
__ GGGGGGGGGG __
_ AAAAAAAAAAAA _
_ OOOOOOOOOOOO _
__  OOOOOOOO  __
________________`,

	fortunecookie: `
________________
______ AAA _____
______ AAA _____
______ AAA _____
___    AAA    __
__ GGGGAAAGGG __
_ GGHHGGGGGGGG _
_ GGGGGGGGGGGG _
_ GGGGGGGGGGGG _
_ GGGGGGGGGGGG _
_ GGGGGGGGGGGG _
__ GGGGGGGGGG __
__ EEEEEEEEEE __
___ EEEEEEEE ___
_____      _____
________________`,

	helmet: `
________________
_______DD_______
_______DD_______
____   DD   ____
___ OOODDOOO ___
__ OOOODDOOOO __
_ OOOOODDOOOOO _
_ OOOOOOOOOOOO _
_ OO        OO _
_ OO        OO _
_ OOOOOOOOOOOO _
_ OO  OOOO  OO _
_ NNNNNNNNNNNN _
__ NNN    NNN __
___ NN    NN ___
________________`,

	gloves: `
________________
________________
_DD DD___DD DD__
_DDDDD___DDDDD__
_DDDDD___DDDDD__
 DDDDD _ DDDDD _
 DOOOD _ DOOOD _
 DOOOD _ DOOOD _
 DDDDD _ DDDDD _
 EDDDE _ EDDDE _
 EEEEE _ EEEEE _
 EOOOE _ EOOOE _
 EEEEE _ EEEEE _
  EEE  _  EEE  _
________________
________________`,

	vest: `
________________
__ LLL    LLL __
_ LLLL    LLLL _
_ LLLLL  LLLLL _
_ LLLLLLLLLLLL _
_ LBBLLLLLLBBL _
_ LOOOOOOOOOOL _
_ LONNNNNNNNOL _
_ LOOOOOOOOOOL _
_ LONNNNNNNNOL _
_ LOOOOOOOOOOL _
_ LLLLLLLLLLLL _
_ LBLLLLLLLLBL _
_ LLLL    LLLL _
__ LLL    LLL __
________________`,

	goldenrooster: `
________________
_________DDD____
________DDDDD___
________HHHHHH__
_______HH HHHHG_
_______HHHHHHHGG
___GG__HHHHHHHG_
__GGG__HHHHDD___
_GGGG_HHHHHHH___
_DGGGHHHHHHHHH__
_DGGGHHHHHHHHH__
__GGGHHHHHHHHH__
___GHHHHHHHHH___
____HHHHHHHHH___
_____GG__GG_____
____GGG__GGG____`,
};
/* biome-ignore-end format: the rows are the drawing. */

/** Drawn for an item with no sprite. */
export const UNKNOWN = `
________________
__            __
_ GGGGGGGGGGGG _
_ GGG      GGG _
_ GGG GGGG  GG _
_ GGGGGGGG  GG _
_ GGGGGGG  GGG _
_ GGGGGG  GGGG _
_ GGGGG  GGGGG _
_ GGGGG  GGGGG _
_ GGGGGGGGGGGG _
_ GGGGG  GGGGG _
_ GGGGG  GGGGG _
_ GGGGGGGGGGGG _
__            __
________________`;

/** Never null: an unknown item gets UNKNOWN. */
export function spriteFor(item) {
	return SPRITES[item] ?? UNKNOWN;
}

/** Never null: an item with no sprite gets UNKNOWN. */
export function spriteURL(item) {
	return toDataURL(spriteFor(item), `item:${item}`);
}
