-- Tables owned by the ohayou game. Applied by the plugin at startup, so a
-- bot running without it never creates them.
CREATE TABLE IF NOT EXISTS users (
    username        TEXT    PRIMARY KEY,
    last            INTEGER NOT NULL DEFAULT 0,
    ohayous         INTEGER NOT NULL DEFAULT 0,
    cum_ohayous     INTEGER NOT NULL DEFAULT 0,
    steal_success   INTEGER NOT NULL DEFAULT 0,
    steal_fail      INTEGER NOT NULL DEFAULT 0,
    stolen_from     INTEGER NOT NULL DEFAULT 0,
    stolen_ohayous  INTEGER NOT NULL DEFAULT 0,
    ohayous_stolen  INTEGER NOT NULL DEFAULT 0,
    probation       INTEGER NOT NULL DEFAULT 0,
    probation_count INTEGER NOT NULL DEFAULT 0,
    times_ohayoued  INTEGER NOT NULL DEFAULT 0,
    registered      INTEGER NOT NULL DEFAULT 0,
    fortune         TEXT    NOT NULL DEFAULT '',
    vault_installed INTEGER NOT NULL DEFAULT 0,
    vault_level     INTEGER NOT NULL DEFAULT 0,
    vault_ohayous   INTEGER NOT NULL DEFAULT 0,
    vault_last      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS user_items (
    username TEXT    NOT NULL REFERENCES users(username) ON DELETE CASCADE,
    item     TEXT    NOT NULL,
    count    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (username, item)
);

CREATE TABLE IF NOT EXISTS user_item_multiply (
    username TEXT    NOT NULL REFERENCES users(username) ON DELETE CASCADE,
    item     TEXT    NOT NULL,
    multiply INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (username, item)
);

CREATE TABLE IF NOT EXISTS user_equipped (
    username       TEXT NOT NULL REFERENCES users(username) ON DELETE CASCADE,
    equip_category TEXT NOT NULL,
    item_name      TEXT NOT NULL,
    PRIMARY KEY (username, equip_category)
);

CREATE TABLE IF NOT EXISTS user_last_used (
    username TEXT    NOT NULL REFERENCES users(username) ON DELETE CASCADE,
    item     TEXT    NOT NULL,
    ts       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (username, item)
);

CREATE TABLE IF NOT EXISTS user_status (
    username TEXT    NOT NULL REFERENCES users(username) ON DELETE CASCADE,
    action   TEXT    NOT NULL,
    active   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (username, action)
);

CREATE TABLE IF NOT EXISTS user_metals (
    username TEXT    NOT NULL REFERENCES users(username) ON DELETE CASCADE,
    metal    TEXT    NOT NULL,
    amount   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (username, metal)
);

CREATE TABLE IF NOT EXISTS items (
    name           TEXT    PRIMARY KEY,
    description    TEXT    NOT NULL DEFAULT '',
    price          INTEGER NOT NULL DEFAULT 0,
    add_amt        INTEGER NOT NULL DEFAULT 0,
    multiply       INTEGER NOT NULL DEFAULT 0,
    multiplies     TEXT    NOT NULL DEFAULT '',
    defense        INTEGER NOT NULL DEFAULT 0,
    item_limit     INTEGER NOT NULL DEFAULT 0,
    acre_limit     INTEGER NOT NULL DEFAULT 0,
    useable        INTEGER NOT NULL DEFAULT 0,
    consume        INTEGER NOT NULL DEFAULT 0,
    effect         TEXT    NOT NULL DEFAULT '',
    has_function   TEXT    NOT NULL DEFAULT '',
    purchase       INTEGER NOT NULL DEFAULT 0,
    category       TEXT    NOT NULL DEFAULT '',
    equip_category TEXT    NOT NULL DEFAULT '',
    needs_acre     INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_items_category ON items(category);
CREATE INDEX IF NOT EXISTS idx_users_ohayous ON users(ohayous DESC);
