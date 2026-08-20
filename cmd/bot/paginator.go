package main

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"strings"
	"sync"
	"time"
)

const pagePrefix = "page_"
const pageTTL = 15 * time.Minute

var pg = &paginator{state: map[string]*pageState{}}

type pageState struct {
	pages     []*discordgo.MessageEmbed
	index     int
	channelID string
	lastUsed  time.Time
}

type paginator struct {
	mu    sync.Mutex
	state map[string]*pageState
	ttl   time.Duration
}

func newPaginator() *paginator {
	return &paginator{
		state: map[string]*pageState{},
		ttl:   pageTTL,
	}
}

func (p *paginator) handleButton(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionMessageComponent {
		return
	}

	customID := i.MessageComponentData().CustomID
	if !strings.HasPrefix(customID, pagePrefix) {
		return
	}

	p.mu.Lock()
	st, ok := p.state[i.Message.ID]
	if !ok {
		p.mu.Unlock()
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredMessageUpdate,
		})
		return
	}

	switch customID {
	case pagePrefix + "prev":
		if st.index > 0 {
			st.index--
		}
	case pagePrefix + "next":
		if st.index < len(st.pages)-1 {
			st.index++
		}
	}
	st.lastUsed = time.Now()
	page := st.index
	embed := st.pages[page]
	total := len(st.pages)
	p.mu.Unlock()

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: buildComponents(page, total),
		},
	})
	if err != nil {
		logger.Errorf("paginator: update error: %v", err)
	}
}

func (p *paginator) EditWithPages(s *discordgo.Session, i *discordgo.InteractionCreate, pages []*discordgo.MessageEmbed) error {
	if len(pages) == 0 {
		return fmt.Errorf("paginator: need at least one page")
	}

	var comps []discordgo.MessageComponent
	if len(pages) > 1 {
		comps = buildComponents(0, len(pages))
	}

	msg, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds:     &[]*discordgo.MessageEmbed{pages[0]},
		Components: &comps,
	})
	if err != nil {
		return fmt.Errorf("paginator: edit failed: %w", err)
	}

	if len(pages) > 1 {
		p.mu.Lock()
		p.state[msg.ID] = &pageState{
			pages:     pages,
			index:     0,
			channelID: i.ChannelID,
			lastUsed:  time.Now(),
		}
		p.mu.Unlock()
	}
	return nil
}

func buildComponents(page, total int) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "◀ Prev",
					Style:    discordgo.SecondaryButton,
					CustomID: "page_prev",
					Disabled: page == 0,
				},
				discordgo.Button{
					Label:    fmt.Sprintf("%d / %d", page+1, total),
					Style:    discordgo.SecondaryButton,
					CustomID: "page_indicator",
					Disabled: true,
				},
				discordgo.Button{
					Label:    "Next ▶",
					Style:    discordgo.SecondaryButton,
					CustomID: "page_next",
					Disabled: page == total-1,
				},
			},
		},
	}
}

func (p *paginator) StartJanitor(s *discordgo.Session) {
	go func() {
		ticker := time.NewTicker(time.Minute * 60)
		defer ticker.Stop()
		for range ticker.C {
			p.sweep(s)
		}
	}()
}

func (p *paginator) sweep(s *discordgo.Session) {
	now := time.Now()

	type dead struct{ msgID, channelID string }
	var expired []dead

	p.mu.Lock()
	for id, st := range p.state {
		if now.Sub(st.lastUsed) > p.ttl {
			expired = append(expired, dead{id, st.channelID})
		}
	}
	p.mu.Unlock()

	for _, d := range expired {
		empty := []discordgo.MessageComponent{}
		_, err := s.ChannelMessageEditComplex(&discordgo.MessageEdit{
			Channel:    d.channelID,
			ID:         d.msgID,
			Components: &empty,
		})
		if err != nil {
			logger.Errorf("paginator: strip buttons on %s: %v", d.msgID, err)
		}
	}
}
