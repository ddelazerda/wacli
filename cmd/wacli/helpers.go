package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/steipete/wacli/internal/store"
	"golang.org/x/term"
)

func isTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func parseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("time is required")
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("unsupported time format %q (use RFC3339 or YYYY-MM-DD)", s)
}

// resolveJIDValue resolves a value that may be either a JID (contains "@") or
// an alias. Used for --chat, --to, and --from flags.
func resolveJIDValue(db *store.DB, value string) (string, error) {
	if strings.Contains(value, "@") {
		return value, nil
	}
	jid, err := db.ResolveAlias(value)
	if err != nil {
		return "", fmt.Errorf("failed to resolve alias %q: %w", value, err)
	}
	if jid == "" {
		return "", fmt.Errorf("alias %q not found (expected a JID containing '@' or a known alias)", value)
	}
	return jid, nil
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}
