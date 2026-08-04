package ohayou

// Event-state accessors. The double-ohayou flag and the cat-adoption window are
// toggled from timer goroutines and read from command handlers, so all access
// goes through evtMu.

func (g *Plugin) getDouble() bool {
	g.evtMu.Lock()
	defer g.evtMu.Unlock()
	return g.doubleOhayou
}

func (g *Plugin) setDouble(v bool) {
	g.evtMu.Lock()
	defer g.evtMu.Unlock()
	g.doubleOhayou = v
}

func (g *Plugin) getCanAdopt() bool {
	g.evtMu.Lock()
	defer g.evtMu.Unlock()
	return g.canAdoptCat
}

func (g *Plugin) setCanAdopt(v bool) {
	g.evtMu.Lock()
	defer g.evtMu.Unlock()
	g.canAdoptCat = v
}
