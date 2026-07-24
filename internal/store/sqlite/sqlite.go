// Package sqlite provides a store.Store implementation
package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

//go:embed schema.sql
var schema string

// DB is the SQLite-backed store.
type DB struct {
	db *sql.DB
}

// compile-time check that DB satisfies the Store interface.
var _ store.Store = (*DB)(nil)

// Open opens (+ creates) the SQLite database at dsn. ":memory:" is also valid
func Open(dsn string) (*DB, error) {
	sdb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	sdb.SetMaxOpenConns(1)
	return &DB{db: sdb}, nil
}

func (d *DB) Init(ctx context.Context) error {
	for _, pragma := range []string{
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := d.db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("sqlite pragma %q: %w", pragma, err)
		}
	}
	if _, err := d.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("sqlite schema: %w", err)
	}
	return nil
}

func (d *DB) Close() error { return d.db.Close() }

func unix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

const itemCols = `name,description,price,add_amt,multiply,multiplies,defense,` +
	`item_limit,acre_limit,useable,consume,effect,has_function,purchase,` +
	`category,equip_category,needs_acre`

// itemUpsertSet is the SET clause for an ON CONFLICT(name) upsert: every
// catalog column except the name key, assigned from the rejected row.
const itemUpsertSet = `description=excluded.description,price=excluded.price,` +
	`add_amt=excluded.add_amt,multiply=excluded.multiply,` +
	`multiplies=excluded.multiplies,defense=excluded.defense,` +
	`item_limit=excluded.item_limit,acre_limit=excluded.acre_limit,` +
	`useable=excluded.useable,consume=excluded.consume,effect=excluded.effect,` +
	`has_function=excluded.has_function,purchase=excluded.purchase,` +
	`category=excluded.category,equip_category=excluded.equip_category,` +
	`needs_acre=excluded.needs_acre`

// itemColsI is itemCols qualified with the "i" table alias, for joins where a
// bare column (e.g. equip_category) would be ambiguous.
const itemColsI = `i.name,i.description,i.price,i.add_amt,i.multiply,i.multiplies,` +
	`i.defense,i.item_limit,i.acre_limit,i.useable,i.consume,i.effect,` +
	`i.has_function,i.purchase,i.category,i.equip_category,i.needs_acre`

type scanner interface{ Scan(...any) error }

func scanItem(s scanner) (store.Item, error) {
	var it store.Item
	var useable, consume, purchase, needsAcre int
	err := s.Scan(&it.Name, &it.Desc, &it.Price, &it.Add, &it.Multiply,
		&it.Multiplies, &it.Defense, &it.Limit, &it.Acrelimit, &useable,
		&consume, &it.Effect, &it.HasFunction, &purchase, &it.Category,
		&it.EquipCategory, &needsAcre)
	if err != nil {
		return it, err
	}
	it.Useable = useable != 0
	it.Consume = consume != 0
	it.Purchase = purchase != 0
	it.NeedsAcre = needsAcre != 0
	return it, nil
}

func (d *DB) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// guard turns the result of a conditional UPDATE into an error. it will return
// store.ErrInsufficient when the statement matched no row
func guard(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return store.ErrInsufficient
	}
	return nil
}

// incItem adds delta, which may be negative, to a user's item count.
func incItem(ctx context.Context, tx *sql.Tx, user, item string, delta int) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO user_items(username,item,count) VALUES(?,?,?)
		 ON CONFLICT(username,item) DO UPDATE SET count=count+excluded.count`,
		user, item, delta)
	return err
}

func incMetal(ctx context.Context, tx *sql.Tx, user, metal string, delta int) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO user_metals(username,metal,amount) VALUES(?,?,?)
		 ON CONFLICT(username,metal) DO UPDATE SET amount=amount+excluded.amount`,
		user, metal, delta)
	return err
}

// SeedItems syncs the catalog in data/items.json into the items table. New
// items are inserted; existing ones (keyed by name) have their catalog fields
// (price, description, effects, etc.) updated to match the file, so editing
// items.json and restarting is enough to change prices. Per-user state lives in
// other tables and is untouched. It returns the number of items written.
func (d *DB) SeedItems(ctx context.Context, items []store.Item) (int, error) {
	err := d.tx(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx,
			`INSERT INTO items(`+itemCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`+
				` ON CONFLICT(name) DO UPDATE SET `+itemUpsertSet)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, it := range items {
			if _, err := stmt.ExecContext(ctx, it.Name, it.Desc, it.Price,
				it.Add, it.Multiply, it.Multiplies, it.Defense, it.Limit,
				it.Acrelimit, b2i(it.Useable), b2i(it.Consume), it.Effect,
				it.HasFunction, b2i(it.Purchase), it.Category, it.EquipCategory,
				b2i(it.NeedsAcre)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(items), nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (d *DB) GetUser(ctx context.Context, nick string) (*store.User, error) {
	u := &store.User{
		Items:        map[string]int{},
		ItemMultiply: map[string]int{},
		Equipped:     map[string]store.Item{},
		LastUsed:     map[string]time.Time{},
		Status:       map[string]bool{},
		Quarry:       store.Quarry{Metals: map[string]int{}},
	}

	var last, probation, vaultLast int64
	var registered, vaultInstalled int
	err := d.db.QueryRowContext(ctx, `
		SELECT username,last,ohayous,cum_ohayous,steal_success,steal_fail,
		       stolen_from,stolen_ohayous,ohayous_stolen,probation,
		       probation_count,times_ohayoued,registered,fortune,
		       vault_installed,vault_level,vault_ohayous,vault_last
		FROM users WHERE username=?`, nick).Scan(
		&u.Username, &last, &u.Ohayous, &u.CumOhayous, &u.StealSuccess,
		&u.StealFail, &u.StolenFrom, &u.StolenOhayous, &u.OhayousStolen,
		&probation, &u.ProbationCount, &u.TimesOhayoued, &registered, &u.Fortune,
		&vaultInstalled, &u.Vault.Level, &u.Vault.Ohayous, &vaultLast)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.Last = time.Unix(last, 0)
	u.Probation = time.Unix(probation, 0)
	u.Registered = registered != 0
	u.Vault.Installed = vaultInstalled != 0
	u.Vault.Last = time.Unix(vaultLast, 0)

	if err := loadIntMap(ctx, d.db, `SELECT item,count FROM user_items WHERE username=?`, nick, u.Items); err != nil {
		return nil, err
	}
	if err := loadIntMap(ctx, d.db, `SELECT item,multiply FROM user_item_multiply WHERE username=?`, nick, u.ItemMultiply); err != nil {
		return nil, err
	}
	if err := loadIntMap(ctx, d.db, `SELECT metal,amount FROM user_metals WHERE username=?`, nick, u.Quarry.Metals); err != nil {
		return nil, err
	}
	if err := loadStatus(ctx, d.db, nick, u.Status); err != nil {
		return nil, err
	}
	if err := loadLastUsed(ctx, d.db, nick, u.LastUsed); err != nil {
		return nil, err
	}
	if err := loadEquipped(ctx, d.db, nick, u.Equipped); err != nil {
		return nil, err
	}
	return u, nil
}

func loadIntMap(ctx context.Context, db *sql.DB, query, nick string, dst map[string]int) error {
	rows, err := db.QueryContext(ctx, query, nick)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var v int
		if err := rows.Scan(&k, &v); err != nil {
			return err
		}
		dst[k] = v
	}
	return rows.Err()
}

func loadStatus(ctx context.Context, db *sql.DB, nick string, dst map[string]bool) error {
	rows, err := db.QueryContext(ctx, `SELECT action,active FROM user_status WHERE username=?`, nick)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var v int
		if err := rows.Scan(&k, &v); err != nil {
			return err
		}
		dst[k] = v != 0
	}
	return rows.Err()
}

func loadLastUsed(ctx context.Context, db *sql.DB, nick string, dst map[string]time.Time) error {
	rows, err := db.QueryContext(ctx, `SELECT item,ts FROM user_last_used WHERE username=?`, nick)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var ts int64
		if err := rows.Scan(&k, &ts); err != nil {
			return err
		}
		dst[k] = time.Unix(ts, 0)
	}
	return rows.Err()
}

func loadEquipped(ctx context.Context, db *sql.DB, nick string, dst map[string]store.Item) error {
	rows, err := db.QueryContext(ctx,
		`SELECT `+itemColsI+` FROM user_equipped e
		 JOIN items i ON i.name = e.item_name WHERE e.username=?`, nick)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return err
		}
		dst[it.EquipCategory] = it
	}
	return rows.Err()
}

// CreateUser inserts a brand-new user with a single starting acre.
func (d *DB) CreateUser(ctx context.Context, nick string, ohayous int) error {
	now := time.Now().Unix()
	return d.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO users(username,last,ohayous,cum_ohayous,times_ohayoued)
			 VALUES(?,?,?,?,1)`, nick, now, ohayous, ohayous); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO user_items(username,item,count) VALUES(?,'acre',1)`, nick)
		return err
	})
}

// SaveOhayou stores the new ohayou total after a daily ration. addedCum is
// added to the cumulative total and times_ohayoued is incremented by one.
func (d *DB) SaveOhayou(ctx context.Context, nick string, newTotal, addedCum int, last, dayStart time.Time) error {
	// the guard makes this atomic. if a second concurrent greeting slips
	// past the snapshot check, its update matches no row (last is already
	// today) and returns ErrInsufficient.
	return guard(d.db.ExecContext(ctx,
		`UPDATE users SET ohayous=?, last=?, cum_ohayous=cum_ohayous+?,
		 times_ohayoued=times_ohayoued+1 WHERE username=? AND last<?`,
		newTotal, unix(last), addedCum, nick, unix(dayStart)))
}

func (d *DB) SetRegister(ctx context.Context, nick string, registered bool) error {
	_, err := d.db.ExecContext(ctx, `UPDATE users SET registered=? WHERE username=?`, b2i(registered), nick)
	return err
}

func (d *DB) ResetLast(ctx context.Context, nick string) error {
	_, err := d.db.ExecContext(ctx, `UPDATE users SET last=0 WHERE username=?`, nick)
	return err
}

func (d *DB) SetStatus(ctx context.Context, nick, action string, active bool) error {
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO user_status(username,action,active) VALUES(?,?,?)
		 ON CONFLICT(username,action) DO UPDATE SET active=excluded.active`,
		nick, action, b2i(active))
	return err
}

func (d *DB) ResetAllStatus(ctx context.Context) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM user_status`)
	return err
}

func (d *DB) SetLastUsed(ctx context.Context, nick, item string, ts time.Time) error {
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO user_last_used(username,item,ts) VALUES(?,?,?)
		 ON CONFLICT(username,item) DO UPDATE SET ts=excluded.ts`,
		nick, item, unix(ts))
	return err
}

func (d *DB) SetFortune(ctx context.Context, nick, fortune string) error {
	_, err := d.db.ExecContext(ctx, `UPDATE users SET fortune=? WHERE username=?`, fortune, nick)
	return err
}

func (d *DB) GetItem(ctx context.Context, name string) (*store.Item, error) {
	it, err := scanItem(d.db.QueryRowContext(ctx, `SELECT `+itemCols+` FROM items WHERE name=?`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &it, nil
}

func (d *DB) ItemsByCategory(ctx context.Context, category string) ([]store.Item, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT `+itemCols+` FROM items WHERE category=? ORDER BY price`, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []store.Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (d *DB) Categories(ctx context.Context) ([]string, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT DISTINCT category FROM items WHERE category != '' ORDER BY category`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cats []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

// AddItem deducts price*amt ohayous and grants amt of the item, applying the
// item's special side effects (multiplier items and the quarry).
func (d *DB) AddItem(ctx context.Context, nick string, item store.Item, amt int) error {
	return d.tx(ctx, func(tx *sql.Tx) error {
		// prevent stale snapshots from leading a balance negative
		if err := guard(tx.ExecContext(ctx,
			`UPDATE users SET ohayous=ohayous-? WHERE username=? AND ohayous>=?`,
			item.Price*amt, nick, item.Price*amt)); err != nil {
			return err
		}
		if err := incItem(ctx, tx, nick, item.Name, amt); err != nil {
			return err
		}
		if item.Multiplies != "" {
			_, err := tx.ExecContext(ctx,
				`INSERT INTO user_item_multiply(username,item,multiply) VALUES(?,?,?)
				 ON CONFLICT(username,item) DO UPDATE SET multiply=multiply+excluded.multiply`,
				nick, item.Multiplies, item.Multiply)
			return err
		}
		return nil
	})
}

func (d *DB) ConsumeItem(ctx context.Context, nick, item string) error {
	// count>0 keeps a double-use race (two handlers, one item) from going
	// negative. matching no row is a harmless here
	_, err := d.db.ExecContext(ctx,
		`UPDATE user_items SET count=count-1 WHERE username=? AND item=? AND count>0`, nick, item)
	return err
}

func (d *DB) AddCat(ctx context.Context, nick string, amt int) error {
	return d.tx(ctx, func(tx *sql.Tx) error {
		return incItem(ctx, tx, nick, "cat", amt)
	})
}

func (d *DB) AddOil(ctx context.Context, nick string, amt int) error {
	return d.tx(ctx, func(tx *sql.Tx) error {
		return incItem(ctx, tx, nick, "oilbarrel", amt)
	})
}

func (d *DB) AddMetals(ctx context.Context, nick string, yield map[string]int) error {
	return d.tx(ctx, func(tx *sql.Tx) error {
		for metal, amt := range yield {
			if err := incMetal(ctx, tx, nick, metal, amt); err != nil {
				return err
			}
		}
		return nil
	})
}

func (d *DB) Equip(ctx context.Context, nick string, item store.Item) error {
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO user_equipped(username,equip_category,item_name) VALUES(?,?,?)
		 ON CONFLICT(username,equip_category) DO UPDATE SET item_name=excluded.item_name`,
		nick, item.EquipCategory, item.Name)
	return err
}

func (d *DB) Unequip(ctx context.Context, nick, equipCategory string) error {
	_, err := d.db.ExecContext(ctx,
		`DELETE FROM user_equipped WHERE username=? AND equip_category=?`, nick, equipCategory)
	return err
}

func (d *DB) InstallVault(ctx context.Context, nick string) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE users SET vault_installed=1, vault_level=0, vault_ohayous=0 WHERE username=?`, nick)
	return err
}

func (d *DB) IncVaultLevel(ctx context.Context, nick string) error {
	_, err := d.db.ExecContext(ctx,
		`UPDATE users SET vault_level=vault_level+1 WHERE username=?`, nick)
	return err
}

// Build consumes the given metals, items, ohayous, and then grants outAmt of the output item
func (d *DB) Build(ctx context.Context, nick string, metalCost, itemCost map[string]int, ohayouCost int, output string, outAmt int) error {
	return d.tx(ctx, func(tx *sql.Tx) error {
		// Every input is debited under a guard (amount/count/ohayous >= cost) so
		// two concurrent builds off the same snapshot can't consume the same
		// resources twice. the second build's first shortfall aborts the tx.
		for metal, amt := range metalCost {
			if err := guard(tx.ExecContext(ctx,
				`UPDATE user_metals SET amount=amount-? WHERE username=? AND metal=? AND amount>=?`,
				amt, nick, metal, amt)); err != nil {
				return err
			}
		}
		for item, amt := range itemCost {
			if err := guard(tx.ExecContext(ctx,
				`UPDATE user_items SET count=count-? WHERE username=? AND item=? AND count>=?`,
				amt, nick, item, amt)); err != nil {
				return err
			}
		}
		if ohayouCost != 0 {
			if err := guard(tx.ExecContext(ctx,
				`UPDATE users SET ohayous=ohayous-? WHERE username=? AND ohayous>=?`,
				ohayouCost, nick, ohayouCost)); err != nil {
				return err
			}
		}
		return incItem(ctx, tx, nick, output, outAmt)
	})
}

// VaultTransfer moves ohayous between a user's balance and their vault.
func (d *DB) VaultTransfer(ctx context.Context, nick string, ohayousDelta, vaultDelta int, last, dayStart time.Time) error {
	return guard(d.db.ExecContext(ctx,
		`UPDATE users SET ohayous=ohayous+?, vault_ohayous=vault_ohayous+?, vault_last=?
		 WHERE username=? AND ohayous+?>=0 AND vault_ohayous+?>=0 AND vault_last<?`,
		ohayousDelta, vaultDelta, unix(last), nick, ohayousDelta, vaultDelta, unix(dayStart)))
}

func (d *DB) Top(ctx context.Context, n int) ([]store.UserOhayous, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT username,ohayous FROM users ORDER BY ohayous DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var top []store.UserOhayous
	for rows.Next() {
		var u store.UserOhayous
		if err := rows.Scan(&u.Username, &u.Ohayous); err != nil {
			return nil, err
		}
		top = append(top, u)
	}
	return top, rows.Err()
}

func (d *DB) SaveSuccessSteal(ctx context.Context, thief, victim string, cat, ohy int) error {
	return d.tx(ctx, func(tx *sql.Tx) error {
		// The victim's debits are guarded so the theft conserves value: a second
		// thief racing the same snapshot can't take ohayous or a cat the victim
		// no longer has. A guard failure rolls back the whole tx, including the
		// thief's credit, so nothing is created from nothing.
		if err := guard(tx.ExecContext(ctx,
			`UPDATE users SET ohayous=ohayous-?, stolen_from=stolen_from+1,
			 ohayous_stolen=ohayous_stolen+? WHERE username=? AND ohayous>=?`,
			ohy, ohy, victim, ohy)); err != nil {
			return err
		}
		if cat > 0 {
			if err := guard(tx.ExecContext(ctx,
				`UPDATE user_items SET count=count-? WHERE username=? AND item='cat' AND count>=?`,
				cat, victim, cat)); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET ohayous=ohayous+?, steal_success=steal_success+1,
			 stolen_ohayous=stolen_ohayous+? WHERE username=?`, ohy, ohy, thief); err != nil {
			return err
		}
		return incItem(ctx, tx, thief, "cat", cat)
	})
}

func (d *DB) SaveFailSteal(ctx context.Context, nick string, fine int, probation time.Time) error {
	// ohayous>=fine keeps a raced pair of failed steals from overdrawing the
	// fine. the losing call returns ErrInsufficient rather than going negative
	return guard(d.db.ExecContext(ctx,
		`UPDATE users SET ohayous=ohayous-?, probation=?, probation_count=probation_count+1,
		 steal_fail=steal_fail+1 WHERE username=? AND ohayous>=?`, fine, unix(probation), nick, fine))
}
