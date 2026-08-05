package store

import "time"

// Visibility is whether a user's holdings may be shown outside irc. Unset is
// its own state, not a default: somebody who has never been asked is not on the
// website, and is not treated as having agreed to be.
type Visibility string

const (
	VisibilityUnset  Visibility = ""
	VisibilityPublic Visibility = "public"
	VisibilityHidden Visibility = "hidden"
)

// User is the full state for a single player. Its fields are both the user table and associated
// tables. Should _always_ be non-nil after a load.
type User struct {
	Username string
	// Account is the services account last proved, empty for a user who has
	// never identified. A nick can be taken by somebody else; an account cannot.
	Account string
	// Web is whether this user agreed to appear on the website.
	Web Visibility
	// Flag is the name of a deer from the gallery, flown over this user's plot.
	// Empty for no flag, and only ever shown on a plot that carries a name.
	Flag           string
	Last           time.Time
	Ohayous        int
	CumOhayous     int
	StealSuccess   int
	StealFail      int
	StolenFrom     int
	StolenOhayous  int
	OhayousStolen  int
	Probation      time.Time
	ProbationCount int
	TimesOhayoued  int
	Items          map[string]int
	ItemMultiply   map[string]int
	Equipped       map[string]Item
	LastUsed       map[string]time.Time
	Status         map[string]bool
	Registered     bool
	Fortune        string
	Vault          Vault
	Quarry         Quarry
}

type Item struct {
	Name          string `json:"name"`
	Desc          string `json:"desc"`
	Price         int    `json:"price"`
	Add           int    `json:"add"`
	Multiply      int    `json:"multiply"`
	Multiplies    string `json:"multiplies"`
	Defense       int    `json:"defense"`
	Limit         int    `json:"limit"`
	Acrelimit     int    `json:"acrelimit"`
	Useable       bool   `json:"useable"`
	Consume       bool   `json:"consume"`
	Effect        string `json:"effect"`
	HasFunction   string `json:"hasFunction"`
	Purchase      bool   `json:"purchase"`
	Category      string `json:"category"`
	EquipCategory string `json:"equipCategory"`
	NeedsAcre     bool   `json:"needsAcre"`
}

// Vault holds a user's savings vault state
type Vault struct {
	Installed bool
	Level     int
	Ohayous   int
	Last      time.Time
}

// Quarry holds a user's accumulated mined metals. The number of quarries a user
// owns is tracked as an ordinary item ("quarry") in Items.
type Quarry struct {
	Metals map[string]int
}

// UserOhayous is for the leaderboard
type UserOhayous struct {
	Username string
	Ohayous  int
}
