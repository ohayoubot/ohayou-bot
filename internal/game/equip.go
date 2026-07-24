package game

import "github.com/ohayoubot/ohayou-bot/internal/store"

func (g *Game) equip(u *store.User, itm string) string {
	if msg, blocked := g.mustIdentify(u); blocked {
		return msg
	}

	item, err := g.store.GetItem(g.ctx(), itm)
	if err != nil {
		return u.Username + ": That isn't an item."
	}
	if u.Items[itm] <= 0 {
		return u.Username + ": You don't have that item."
	}
	if item.EquipCategory == "" {
		return u.Username + ": That item can't be equipped."
	}
	if u.Equipped[item.EquipCategory].Name == item.Name {
		return u.Username + ": You already have that item equipped."
	}

	prev := u.Equipped[item.EquipCategory].Name
	if err := g.store.Equip(g.ctx(), u.Username, *item); err != nil {
		g.log.Error("equip", "nick", u.Username, "item", itm, "err", err)
		return "Something went wrong. Try again."
	}
	if prev == "" {
		return u.Username + " equipped " + itm + "."
	}
	return u.Username + " unequipped " + prev + " from " + item.EquipCategory +
		" and equipped " + item.Name + "."
}

func (g *Game) unequip(u *store.User, itm string) string {
	if msg, blocked := g.mustIdentify(u); blocked {
		return msg
	}

	item, err := g.store.GetItem(g.ctx(), itm)
	if err != nil {
		return u.Username + ": That isn't an item."
	}
	if u.Equipped[item.EquipCategory].Name == item.Name {
		if err := g.store.Unequip(g.ctx(), u.Username, item.EquipCategory); err != nil {
			g.log.Error("unequip", "nick", u.Username, "err", err)
			return "Something went wrong. Try again."
		}
		return u.Username + " unequipped " + item.Name + " from " + item.EquipCategory + "."
	}
	if u.Equipped[item.EquipCategory].Name == "" {
		return u.Username + ": That item isn't equipped."
	}
	return ""
}
