package game

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

// vaultCap returns the maximum a vault of the given level can hold.
func vaultCap(level int) int { return int(math.Pow(10, 3+float64(level))) }

// upgradeVault is the item function for the vaultupgrade item. It raises the
// user's vault level (multiplying its capacity by ten) and consumes the upgrade
// item only on success.
func (g *Game) upgradeVault(u *store.User) string {
	if !u.Vault.Installed {
		return " but doesn't have a vault to upgrade! Buy and use a vault first."
	}
	if err := g.store.IncVaultLevel(g.ctx(), u.Username); err != nil {
		g.log.Error("upgrade vault", "nick", u.Username, "err", err)
		return ""
	}
	if err := g.store.ConsumeItem(g.ctx(), u.Username, "vaultupgrade"); err != nil {
		g.log.Error("consume vaultupgrade", "nick", u.Username, "err", err)
	}
	newLevel := u.Vault.Level + 1
	return fmt.Sprintf(" and upgrades their vault to Level %d (capacity %d ohayous).",
		newLevel+1, vaultCap(newLevel))
}

func (g *Game) deposit(u *store.User, amt int) string {
	if msg, blocked := g.mustIdentify(u); blocked {
		return msg
	}
	if amt <= 0 {
		return u.Username + ": Enter a positive number of ohayous to deposit."
	}

	cap := vaultCap(u.Vault.Level)
	if !u.Vault.Installed {
		return u.Username + ": You don't have a vault yet."
	}
	if u.Vault.Ohayous >= cap {
		return u.Username + ": Your vault is full. Consider upgrading it."
	}
	if usedToday(u.Vault.Last, g.est) {
		return u.Username + ": You've already opened your vault once today. Due to " +
			"security concerns you cannot open it again."
	}
	if u.Ohayous < amt {
		return u.Username + ": You don't have that many ohayous."
	}
	if (u.Vault.Ohayous + amt) > cap {
		return u.Username + ": That's more than your vault can hold. Double-check " +
			"your numbers or purchase an upgrade."
	}

	now := time.Now().In(g.est)
	err := g.store.VaultTransfer(g.ctx(), u.Username, -amt, amt, now, startOfDay(now, g.est))
	if errors.Is(err, store.ErrInsufficient) {
		// lost a race against another vault op. this should mean that the  balance moved
		// or the vault was already opened today. bounce it rather than force the transfer.
		return u.Username + ": That didn't go through. Check your balance and note the vault opens once a day."
	}
	if err != nil {
		g.log.Error("deposit", "nick", u.Username, "err", err)
		return "Something went wrong. Try again."
	}
	return fmt.Sprintf("%s deposited %d ohayous to their vault.", u.Username, amt)
}

func (g *Game) withdraw(u *store.User, amt int) string {
	if msg, blocked := g.mustIdentify(u); blocked {
		return msg
	}
	if amt <= 0 {
		return u.Username + ": Enter a positive number of ohayous to withdraw."
	}

	if !u.Vault.Installed {
		return u.Username + ": You don't have a vault yet."
	}
	if u.Vault.Ohayous == 0 {
		return u.Username + ": You don't have any ohayous in your vault."
	}
	if usedToday(u.Vault.Last, g.est) {
		return u.Username + ": You've already opened your vault once today. " +
			"According to vault security protocol you cannot open it again until" +
			" tomorrow."
	}
	if (u.Vault.Ohayous - amt) < 0 {
		return u.Username + ": You don't have that many ohayous in your vault."
	}

	now := time.Now().In(g.est)
	err := g.store.VaultTransfer(g.ctx(), u.Username, amt, -amt, now, startOfDay(now, g.est))
	if errors.Is(err, store.ErrInsufficient) {
		// the vault emptied or was already opened today
		return u.Username + ": That didn't go through. Check your vault and note it opens once a day."
	}
	if err != nil {
		g.log.Error("withdraw", "nick", u.Username, "err", err)
		return "Something went wrong. Try again."
	}
	return fmt.Sprintf("%s withdrew %d ohayous from their vault.", u.Username, amt)
}
