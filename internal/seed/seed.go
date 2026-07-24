// Package seed loads data from the static data files
package seed

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ohayoubot/ohayou-bot/internal/store"
)

func LoadItems(path string) ([]store.Item, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read items: %w", err)
	}
	var items []store.Item
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parse items: %w", err)
	}
	return items, nil
}

func LoadFortunes(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open fortunes: %w", err)
	}
	defer f.Close()

	var fortunes []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fortunes = append(fortunes, line)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read fortunes: %w", err)
	}
	return fortunes, nil
}
