package game

// Event-state accessors. The double-ohayou flag and the cat-adoption window are
// toggled from timer goroutines and read from command handlers, so all access
// goes through evtMu.

func (g *Game) getDouble() bool {
	g.evtMu.Lock()
	defer g.evtMu.Unlock()
	return g.doubleOhayou
}

func (g *Game) setDouble(v bool) {
	g.evtMu.Lock()
	defer g.evtMu.Unlock()
	g.doubleOhayou = v
}

func (g *Game) getCanAdopt() bool {
	g.evtMu.Lock()
	defer g.evtMu.Unlock()
	return g.canAdoptCat
}

func (g *Game) setCanAdopt(v bool) {
	g.evtMu.Lock()
	defer g.evtMu.Unlock()
	g.canAdoptCat = v
}
