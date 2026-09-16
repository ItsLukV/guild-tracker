package main

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/ItsLukV/guild-tracker/internal/logging"
	"github.com/ItsLukV/guild-tracker/internal/market"
	"github.com/ItsLukV/guild-tracker/internal/store"
	"github.com/ItsLukV/guild-tracker/internal/utils"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/components"
	. "maragu.dev/gomponents/html"
	. "maragu.dev/gomponents/http"
)

func main() {
	if err := start(); err != nil {
		panic(err)
	}
}

func start() error {
	logger := logging.New()
	m := market.NewCache()
	m.StartAutoRefresh(time.Minute*60, logger.Errorf)
	db, err := gorm.Open(sqlite.Open("chest_tracker.db"), &gorm.Config{})
	if err != nil {
		return err
	}

	app := &App{
		db:     db,
		logger: logger,
		market: m,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", Adapt(app.handleLeaderboard))
	mux.HandleFunc("/leaderboard", Adapt(app.handleLeaderboardFragment))

	logger.Info("Starting on http://localhost:8080")
	if err := http.ListenAndServe("localhost:8080", mux); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

type App struct {
	db     *gorm.DB
	logger *zap.SugaredLogger
	market *market.Cache
}

type board struct {
	label string
	query func(app *App, r *http.Request) ([]LeaderboardRow, error)
}

var boards = map[string]board{
	"runs": board{
		label: "Runs",
		query: (*App).runsBoard,
	},
	"profit": board{
		label: "Profit",
		query: (*App).chestProfit,
	},
}

type timeOption struct {
	label string
	mode  store.Duration
}

var timeOptions = map[string]timeOption{
	"total":   {"All Time", store.Total},
	"daily":   {"Daily", store.Day},
	"weekly":  {"Weekly", store.Week},
	"monthly": {"Monthly", store.Month},
	"yearly":  {"Yearly", store.Year},
}

func resolveTime(r *http.Request) (string, timeOption) {
	key := r.URL.Query().Get("time")
	t, ok := timeOptions[key]
	if !ok {
		key = "total"
		t = timeOptions["total"]
	}
	return key, t
}

type LeaderboardRow struct {
	Name  string
	Value string
}

func (a *App) chestProfit(r *http.Request) ([]LeaderboardRow, error) {
	_, t := resolveTime(r)

	profit, err := store.TotalProfitByPlayer(a.db, a.market, t.mode)
	if err != nil {
		return nil, err
	}

	out := make([]LeaderboardRow, 0)
	for _, p := range profit {
		out = append(out, LeaderboardRow{
			Name:  p.Username,
			Value: utils.ShortNumber(p.Profit),
		})
	}
	return out, nil
}

func (a *App) runsBoard(r *http.Request) ([]LeaderboardRow, error) {
	_, t := resolveTime(r)

	runs, err := store.TotalRunsByPlayer(a.db, t.mode)
	if err != nil {
		return nil, err
	}
	out := make([]LeaderboardRow, 0)
	for _, run := range runs {
		out = append(out, LeaderboardRow{
			Name:  run.Username,
			Value: strconv.FormatInt(run.Count, 10),
		})
	}
	return out, nil
}

func page(leaderboardNode Node) Node {
	return HTML5(HTML5Props{
		Title: "meow",
		Head: []Node{
			Script(Src("https://cdn.tailwindcss.com?plugins=forms,typography")),
			Script(Src("https://unpkg.com/htmx.org")),
		},
		Body: []Node{
			leaderboardNode,
		},
	})
}

func leaderboardTable(rows []LeaderboardRow, title, current, currentTime string) Node {
	return Div(
		ID("leaderboard"),
		H1(Text(title)),
		Div(Class("not-prose flex gap-4"), // put the two selects side by side
			boardSelect(current),
			timeSelect(currentTime),
		),
		Table(
			THead(
				Tr(
					Th(Text("Name")),
					Th(Text("Value")),
				),
			),
			TBody(
				Map(rows, func(row LeaderboardRow) Node {
					return Tr(
						Td(Text(row.Name)),
						Td(Text(row.Value)),
					)
				}),
			),
		),
	)
}

func boardSelect(current string) Node {
	keys := make([]string, 0, len(boards))
	for key := range boards {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var opts []Node
	for _, key := range keys {
		opts = append(opts, Option(
			Value(key),
			Text(boards[key].label),
			If(key == current, Selected()),
		))
	}
	return Select(
		Class("not-prose border rounded p-2 mb-4 w-64"),
		Attr("hx-get", "/leaderboard"),
		Attr("hx-target", "#leaderboard"),
		Attr("hx-swap", "outerHTML"),
		Attr("hx-include", "[name='time']"),
		Name("board"),
		Group(opts),
	)
}

func timeSelect(current string) Node {
	keys := make([]string, 0, len(timeOptions))
	for key := range timeOptions {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var opts []Node
	for _, key := range keys {
		opts = append(opts, Option(
			Value(key),
			Text(timeOptions[key].label),
			If(key == current, Selected()),
		))
	}
	return Select(
		Class("not-prose border rounded p-2 mb-4 w-64"),
		Attr("hx-get", "/leaderboard"),
		Attr("hx-target", "#leaderboard"),
		Attr("hx-swap", "outerHTML"),
		Attr("hx-include", "[name='board']"),
		Name("time"),
		Group(opts),
	)
}

func leaderboardElement(rows []LeaderboardRow, title, current, currentTime string) Node {
	return Div(
		Class("max-w-7xl mx-auto p-4 prose lg:prose-lg xl:prose-xl"),
		leaderboardTable(rows, title, current, currentTime),
	)
}

func (a *App) handleLeaderboard(w http.ResponseWriter, r *http.Request) (Node, error) {
	key := r.URL.Query().Get("board")
	if key == "" {
		key = "runs"
	}
	b, ok := boards[key]
	if !ok {
		a.logger.Errorf("could not find %s query param", r.URL.Query().Get("board"))
		return nil, fmt.Errorf("could not find %s query param", r.URL.Query().Get("board"))
	}

	timeKey, _ := resolveTime(r)

	rows, err := b.query(a, r)
	if err != nil {
		a.logger.Errorw("leaderboard failed", "board", b.label, "err", err)
		return nil, err
	}

	return page(leaderboardElement(rows, b.label, key, timeKey)), nil
}

func (a *App) handleLeaderboardFragment(w http.ResponseWriter, r *http.Request) (Node, error) {
	key := r.URL.Query().Get("board")
	if key == "" {
		key = "runs"
	}

	b, ok := boards[key]
	if !ok {
		a.logger.Errorf("could not find %s query param", r.URL.Query().Get("board"))
		return nil, fmt.Errorf("could not find %s query param", r.URL.Query().Get("board"))
	}

	timeKey, _ := resolveTime(r)

	rows, err := b.query(a, r)
	if err != nil {
		a.logger.Errorw("leaderboard fragment failed", "board", b.label, "err", err)
		return nil, err
	}

	return leaderboardTable(rows, b.label, key, timeKey), nil
}
