package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ItsLukV/guild-tracker/internal/store"
)

var (
	colorRe = regexp.MustCompile("§.") // § + one rune
	dnRe    = regexp.MustCompile(`^(.+): (.+) \((\d+)\)$`)
)

func (p *TreasureParticipants) ParseDisplayName() (string, int, error) {
	clean := strings.TrimSpace(colorRe.ReplaceAllString(p.DisplayName, ""))
	m := dnRe.FindStringSubmatch(clean)
	if m == nil {
		return "", -1, fmt.Errorf("unexpected display_name format: %q", clean)
	}
	lvl, err := strconv.Atoi(m[3])
	if err != nil {
		return "", -1, err
	}
	return m[2], lvl, nil
}

func (p *TreasureParticipants) toStore(DungeonRunID string) store.Participant {
	class, lvl, err := p.ParseDisplayName()
	if err != nil {
		return store.Participant{}
	}
	return store.Participant{
		DungeonRunID:   DungeonRunID,
		PlayerUUID:     p.PlayerUUID,
		ClassMilestone: p.ClassMilestone,
		Class:          class,
		Level:          lvl,
	}
}
