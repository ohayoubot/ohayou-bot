package bot

import "strings"

// Message is a line the bot saw: a command directed toward it, or, for
// watchers, any other line in a channel it sits in.
type Message struct {
	Prefix  string   // the configured command prefix, e.g. "!"
	Command string   // the command name without the prefix, e.g. "ohayou". empty for watchers
	Text    string   // the message as it arrived, unsplit
	Args    []string // the full message split on whitespace (Args[0] is the raw command word)
	Target  string   // channel the message arrived on, or the sender's nick for a PM
	Nick    string
	Host    string
	Admin   bool // whether the sender is a configured admin
}

func (m *Message) HasArgs() bool { return len(m.Args) > 1 }

// FromChannel returns whether the message came from a channel or pm
func (m *Message) FromChannel() bool { return strings.HasPrefix(m.Target, "#") }

type Handler func(m *Message)

// Watcher sees the lines that aren't commands, for the plugins that react to
// what a channel is saying rather than to being asked.
type Watcher func(m *Message)

// Command connects the command to its handler func.
type Command struct {
	Name    string  // without the prefix
	Admin   bool    // if true, only admins may invoke it
	Handler Handler // invoked in its own goroutine
}
