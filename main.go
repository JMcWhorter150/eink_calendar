package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-sqlite3"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	canvasWidth              = 800
	canvasHeight             = 480
	minSecondsBetweenRefresh = 300
	defaultAddr              = ":8000"
	defaultDBPath            = "habit_epaper/habit.db"
	displayBlackThreshold    = 220
	displayRedThreshold      = 120
	displayRedDelta          = 20
)

var (
	colorWhite = color.RGBA{255, 255, 255, 255}
	colorBlack = color.RGBA{0, 0, 0, 255}
	colorRed   = color.RGBA{255, 0, 0, 255}
)

type habitRow struct {
	DayISO  string `json:"day"`
	Read    int    `json:"read"`
	Journal int    `json:"journal"`
	Workout int    `json:"workout"`
}

type totals struct {
	Read    int `json:"read"`
	Journal int `json:"journal"`
	Workout int `json:"workout"`
}

type server struct {
	db               *sql.DB
	renderer         *renderer
	addr             string
	dbPath           string
	refreshMu        sync.Mutex
	refreshRequested bool
	lastRefresh      time.Time
	lastRefreshError string
}

type renderer struct {
	titleFace  font.Face
	headerFace font.Face
	bodyFace   font.Face
	smallFace  font.Face
	crispFace  font.Face
}

type faceSpec struct {
	size float64
	src  []byte
}

type stateResponse struct {
	Day              string `json:"day"`
	Read             int    `json:"read"`
	Journal          int    `json:"journal"`
	Workout          int    `json:"workout"`
	RefreshQueued    bool   `json:"refresh_queued"`
	LastRefresh      string `json:"last_refresh,omitempty"`
	LastRefreshError string `json:"last_refresh_error,omitempty"`
}

func main() {
	addr := flag.String("addr", envOr("HABIT_ADDR", defaultAddr), "HTTP listen address")
	dbPath := flag.String("db", envOr("HABIT_DB_PATH", defaultDBPath), "SQLite database path")
	renderOnly := flag.String("render", "", "Render current month to a PNG and exit")
	flag.Parse()

	db, err := openDB(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	rndr, err := newRenderer()
	if err != nil {
		log.Fatalf("load fonts: %v", err)
	}

	s := &server{
		db:       db,
		renderer: rndr,
		addr:     *addr,
		dbPath:   *dbPath,
	}

	if err := initDB(db); err != nil {
		log.Fatalf("init db: %v", err)
	}

	if *renderOnly != "" {
		if err := s.renderPreview(*renderOnly); err != nil {
			log.Fatalf("render preview: %v", err)
		}
		log.Printf("wrote %s", *renderOnly)
		return
	}

	go s.refreshWorker()

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/state", s.handleState)
	mux.HandleFunc("/habit/set", s.handleHabitSet)
	mux.HandleFunc("/habit/toggle", s.handleHabitToggle)
	mux.HandleFunc("/mood/override", s.handleMoodOverride)
	mux.HandleFunc("/mood/clear_override", s.handleMoodClearOverride)
	mux.HandleFunc("/refresh", s.handleRefresh)

	log.Printf("habit-epaper listening on %s", s.addr)
	if err := http.ListenAndServe(s.addr, loggingMiddleware(mux)); err != nil {
		log.Fatal(err)
	}
}

func openDB(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	cfg := sqlite3.SQLiteDriver{}
	sql.Register("sqlite3_habit_epaper", &cfg)
	db, err := sql.Open("sqlite3_habit_epaper", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

func initDB(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS daily (
			day TEXT PRIMARY KEY,
			read INTEGER NOT NULL DEFAULT 0,
			journal INTEGER NOT NULL DEFAULT 0,
			workout INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS meta (
			k TEXT PRIMARY KEY,
			v TEXT
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func newRenderer() (*renderer, error) {
	makeFace := func(spec faceSpec) (font.Face, error) {
		ttf, err := opentype.Parse(spec.src)
		if err != nil {
			return nil, err
		}
		return opentype.NewFace(ttf, &opentype.FaceOptions{
			Size:    spec.size,
			DPI:     72,
			Hinting: font.HintingFull,
		})
	}

	titleFace, err := makeFace(faceSpec{size: 28, src: gobold.TTF})
	if err != nil {
		return nil, err
	}
	headerFace, err := makeFace(faceSpec{size: 16, src: gobold.TTF})
	if err != nil {
		return nil, err
	}
	bodyFace, err := makeFace(faceSpec{size: 14, src: goregular.TTF})
	if err != nil {
		return nil, err
	}
	smallFace, err := makeFace(faceSpec{size: 12, src: goregular.TTF})
	if err != nil {
		return nil, err
	}

	return &renderer{
		titleFace:  titleFace,
		headerFace: headerFace,
		bodyFace:   bodyFace,
		smallFace:  smallFace,
		crispFace:  basicfont.Face7x13,
	}, nil
}

func (s *server) renderPreview(path string) error {
	now := time.Now()
	img, err := s.renderCurrentMonth(now)
	if err != nil {
		return err
	}
	return writePNG(path, img)
}

func (s *server) renderCurrentMonth(now time.Time) (image.Image, error) {
	monthData, err := getMonth(s.db, now.Year(), now.Month())
	if err != nil {
		return nil, err
	}
	ytd, err := ytdCounts(s.db, now.Year())
	if err != nil {
		return nil, err
	}
	streakCurrent, streakBest, err := computeStreaks(s.db, now)
	if err != nil {
		return nil, err
	}
	todayState, err := getDay(s.db, now.Format(time.DateOnly))
	if err != nil {
		return nil, err
	}
	img := s.renderer.renderMonth(now, monthData, ytd, streakCurrent, streakBest, todayState.Read+todayState.Journal+todayState.Workout)
	return img, nil
}

func (s *server) refreshWorker() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if !s.shouldRefreshNow() {
			continue
		}
		if err := s.performRefresh(); err != nil {
			log.Printf("refresh failed: %v", err)
			s.setRefreshError(err.Error())
		} else {
			s.setRefreshError("")
		}
	}
}

func (s *server) shouldRefreshNow() bool {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	if !s.refreshRequested {
		return false
	}
	if !s.lastRefresh.IsZero() && time.Since(s.lastRefresh) < minSecondsBetweenRefresh*time.Second {
		return false
	}
	s.refreshRequested = false
	s.lastRefresh = time.Now()
	return true
}

func (s *server) setRefreshRequested() {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	s.refreshRequested = true
}

func (s *server) setRefreshError(msg string) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	s.lastRefreshError = msg
}

func (s *server) snapshotRefreshState() (queued bool, last time.Time, lastErr string) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	return s.refreshRequested, s.lastRefresh, s.lastRefreshError
}

func (s *server) performRefresh() error {
	now := time.Now()
	img, err := s.renderCurrentMonth(now)
	if err != nil {
		return err
	}
	if preview := os.Getenv("HABIT_PREVIEW_PATH"); preview != "" {
		if err := writePNG(preview, img); err != nil {
			return err
		}
	}
	if strings.EqualFold(os.Getenv("HABIT_DISABLE_DISPLAY"), "1") || strings.EqualFold(os.Getenv("HABIT_DISABLE_DISPLAY"), "true") {
		return nil
	}
	return displayImage(img)
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	queued, last, lastErr := s.snapshotRefreshState()
	data := map[string]any{
		"Today":            time.Now().Format("Mon Jan 2"),
		"RefreshQueued":    queued,
		"LastRefresh":      displayTime(last),
		"LastRefreshError": lastErr,
	}
	if err := indexTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) handleState(w http.ResponseWriter, r *http.Request) {
	day, err := parseDay(r.URL.Query().Get("day"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	row, err := getDay(s.db, day.Format(time.DateOnly))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	queued, last, lastErr := s.snapshotRefreshState()
	resp := stateResponse{
		Day:              day.Format(time.DateOnly),
		Read:             row.Read,
		Journal:          row.Journal,
		Workout:          row.Workout,
		RefreshQueued:    queued,
		LastRefresh:      displayTime(last),
		LastRefreshError: lastErr,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) handleHabitSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	day, err := parseDay(r.URL.Query().Get("day"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	habit, err := normalizeHabit(r.URL.Query().Get("habit"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	value, err := parseBoolInt(r.URL.Query().Get("value"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := setHabit(s.db, day.Format(time.DateOnly), habit, value); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.setRefreshRequested()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "habit": habit, "value": value, "day": day.Format(time.DateOnly)})
}

func (s *server) handleHabitToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	day, err := parseDay(r.URL.Query().Get("day"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	habit, err := normalizeHabit(r.URL.Query().Get("habit"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	value, err := toggleHabit(s.db, day.Format(time.DateOnly), habit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.setRefreshRequested()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "habit": habit, "value": value, "day": day.Format(time.DateOnly)})
}

func (s *server) handleMoodOverride(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	level, err := strconv.Atoi(r.URL.Query().Get("level"))
	if err != nil || level < 0 || level > 9 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "level must be 0..9"})
		return
	}
	if err := setMeta(s.db, "mood_override", strconv.Itoa(level)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.setRefreshRequested()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "level": level})
}

func (s *server) handleMoodClearOverride(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if err := deleteMeta(s.db, "mood_override"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.setRefreshRequested()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	s.setRefreshRequested()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true, "queued": true})
}

func parseDay(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		now := time.Now()
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), nil
	}
	day, err := time.ParseInLocation(time.DateOnly, raw, time.Local)
	if err != nil {
		return time.Time{}, errors.New("invalid day format; use YYYY-MM-DD")
	}
	return day, nil
}

func normalizeHabit(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "read", "journal", "workout":
		return strings.ToLower(strings.TrimSpace(raw)), nil
	default:
		return "", errors.New("invalid habit")
	}
}

func parseBoolInt(raw string) (int, error) {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("value must be 0 or 1")
	}
	if value == 0 {
		return 0, nil
	}
	if value == 1 {
		return 1, nil
	}
	return 0, errors.New("value must be 0 or 1")
}

func setHabit(db *sql.DB, dayISO, habit string, value int) error {
	if _, err := db.Exec(`INSERT OR IGNORE INTO daily(day) VALUES (?)`, dayISO); err != nil {
		return err
	}
	_, err := db.Exec(fmt.Sprintf(`UPDATE daily SET %s = ? WHERE day = ?`, habit), value, dayISO)
	return err
}

func toggleHabit(db *sql.DB, dayISO, habit string) (int, error) {
	if _, err := db.Exec(`INSERT OR IGNORE INTO daily(day) VALUES (?)`, dayISO); err != nil {
		return 0, err
	}
	row, err := getDay(db, dayISO)
	if err != nil {
		return 0, err
	}
	current := map[string]int{"read": row.Read, "journal": row.Journal, "workout": row.Workout}[habit]
	next := 1
	if current == 1 {
		next = 0
	}
	return next, setHabit(db, dayISO, habit, next)
}

func getDay(db *sql.DB, dayISO string) (habitRow, error) {
	row := habitRow{DayISO: dayISO}
	err := db.QueryRow(`SELECT read, journal, workout FROM daily WHERE day = ?`, dayISO).Scan(&row.Read, &row.Journal, &row.Workout)
	if errors.Is(err, sql.ErrNoRows) {
		return row, nil
	}
	return row, err
}

func getMonth(db *sql.DB, year int, month time.Month) (map[int]habitRow, error) {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	next := first.AddDate(0, 1, 0)
	rows, err := db.Query(`SELECT day, read, journal, workout FROM daily WHERE day >= ? AND day < ?`, first.Format(time.DateOnly), next.Format(time.DateOnly))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	data := make(map[int]habitRow)
	for rows.Next() {
		var row habitRow
		if err := rows.Scan(&row.DayISO, &row.Read, &row.Journal, &row.Workout); err != nil {
			return nil, err
		}
		parts := strings.Split(row.DayISO, "-")
		dayNum, err := strconv.Atoi(parts[len(parts)-1])
		if err != nil {
			return nil, err
		}
		data[dayNum] = row
	}
	return data, rows.Err()
}

func ytdCounts(db *sql.DB, year int) (totals, error) {
	start := time.Date(year, time.January, 1, 0, 0, 0, 0, time.Local)
	end := start.AddDate(1, 0, 0)
	var out totals
	err := db.QueryRow(`
		SELECT COALESCE(SUM(read), 0), COALESCE(SUM(journal), 0), COALESCE(SUM(workout), 0)
		FROM daily
		WHERE day >= ? AND day < ?`,
		start.Format(time.DateOnly), end.Format(time.DateOnly),
	).Scan(&out.Read, &out.Journal, &out.Workout)
	return out, err
}

func lastNDaysRows(db *sql.DB, end time.Time, n int) ([]habitRow, error) {
	start := end.AddDate(0, 0, -(n - 1))
	rows, err := db.Query(`SELECT day, read, journal, workout FROM daily WHERE day >= ? AND day <= ?`, start.Format(time.DateOnly), end.Format(time.DateOnly))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	found := map[string]habitRow{}
	for rows.Next() {
		var row habitRow
		if err := rows.Scan(&row.DayISO, &row.Read, &row.Journal, &row.Workout); err != nil {
			return nil, err
		}
		found[row.DayISO] = row
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]habitRow, 0, n)
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		iso := day.Format(time.DateOnly)
		if row, ok := found[iso]; ok {
			out = append(out, row)
		} else {
			out = append(out, habitRow{DayISO: iso})
		}
	}
	return out, nil
}

func setMeta(db *sql.DB, key, value string) error {
	_, err := db.Exec(`INSERT INTO meta(k, v) VALUES (?, ?) ON CONFLICT(k) DO UPDATE SET v = excluded.v`, key, value)
	return err
}

func getMeta(db *sql.DB, key string) (string, bool, error) {
	var value string
	err := db.QueryRow(`SELECT v FROM meta WHERE k = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return value, err == nil, err
}

func deleteMeta(db *sql.DB, key string) error {
	_, err := db.Exec(`DELETE FROM meta WHERE k = ?`, key)
	return err
}

func computeMoodLevel(db *sql.DB, today time.Time) (int, error) {
	if raw, ok, err := getMeta(db, "mood_override"); err != nil {
		return 0, err
	} else if ok {
		level, err := strconv.Atoi(raw)
		if err == nil {
			if level < 0 {
				level = 0
			}
			if level > 9 {
				level = 9
			}
			return level, nil
		}
	}

	rows, err := lastNDaysRows(db, today, 14)
	if err != nil {
		return 0, err
	}
	return weightedMood(rows), nil
}

func weightedMood(rows []habitRow) int {
	if len(rows) == 0 {
		return 0
	}
	totalWeight := 0.0
	score := 0.0
	start := 1
	if len(rows) < 14 {
		start = 15 - len(rows)
	}
	for idx, row := range rows {
		weight := float64(start + idx)
		totalWeight += weight
		daily := float64(row.Read+row.Journal+row.Workout) / 3.0
		score += daily * weight
	}
	if totalWeight == 0 {
		return 0
	}
	level := int(math.Round((score / totalWeight) * 9))
	if level < 0 {
		return 0
	}
	if level > 9 {
		return 9
	}
	return level
}

func computeStreaks(db *sql.DB, today time.Time) (current int, best int, err error) {
	rows, err := lastNDaysRows(db, today, today.YearDay())
	if err != nil {
		return 0, 0, err
	}
	run := 0
	for _, row := range rows {
		if row.Read+row.Journal+row.Workout == 3 {
			run++
			if run > best {
				best = run
			}
		} else {
			run = 0
		}
	}
	return run, best, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func displayTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.RequestURI(), time.Since(start).Round(time.Millisecond))
	})
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, img)
}

func (r *renderer) renderMonth(today time.Time, monthData map[int]habitRow, ytd totals, streakCurrent, streakBest, todayScore int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, canvasWidth, canvasHeight))
	draw.Draw(img, img.Bounds(), &image.Uniform{colorWhite}, image.Point{}, draw.Src)

	r.drawText(img, 20, 48, today.Format("January 2006"), colorBlack, r.titleFace)
	r.drawText(img, 20, 76, "Current Month", colorRed, r.crispFace)
	r.drawSummaryPanel(img, image.Rect(500, 18, 780, 138), ytd, streakCurrent, streakBest, todayScore)

	gridTop := 158
	gridLeft := 20
	gridRight := canvasWidth - 20
	gridBottom := canvasHeight - 20
	headerHeight := 24
	weeks := monthWeeks(today.Year(), today.Month())
	cellW := (gridRight - gridLeft) / 7
	cellH := (gridBottom - gridTop - headerHeight) / len(weeks)

	weekdays := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	for i, label := range weekdays {
		x := gridLeft + (i * cellW)
		r.drawCenteredText(img, x, gridTop+18, cellW, label, colorRed, r.crispFace)
	}

	drawLine(img, gridLeft, gridTop+headerHeight, gridRight, gridTop+headerHeight, colorBlack, 1)
	for i := 0; i <= 7; i++ {
		x := gridLeft + i*cellW
		drawLine(img, x, gridTop+headerHeight, x, gridTop+headerHeight+cellH*len(weeks), colorBlack, 1)
	}
	for row := 0; row <= len(weeks); row++ {
		y := gridTop + headerHeight + row*cellH
		drawLine(img, gridLeft, y, gridRight, y, colorBlack, 1)
	}

	for rowIdx, week := range weeks {
		for colIdx, dayNum := range week {
			if dayNum == 0 {
				continue
			}
			x0 := gridLeft + colIdx*cellW
			y0 := gridTop + headerHeight + rowIdx*cellH
			x1 := x0 + cellW
			y1 := y0 + cellH

			r.drawText(img, x0+6, y0+18, strconv.Itoa(dayNum), colorBlack, r.crispFace)
			if dayNum == today.Day() {
				drawRect(img, image.Rect(x0+1, y0+1, x1-1, y1-1), colorRed, 2)
				fillRect(img, image.Rect(x0+cellW-14, y0+2, x0+cellW-4, y0+12), colorRed)
			}

			row := monthData[dayNum]
			iconY := y1 - 20
			if row.Read == 1 {
				drawBookLines(img, x0+8, iconY, 18, 12)
			}
			if row.Journal == 1 {
				drawJournalHatch(img, x0+34, iconY-1, 18, 14)
			}
			if row.Workout == 1 {
				drawBolt(img, x0+62, iconY-2, 14, 16)
			}
		}
	}

	return img
}

func (r *renderer) drawSummaryPanel(img draw.Image, rect image.Rectangle, ytd totals, current, best, todayScore int) {
	drawRect(img, rect, colorRed, 2)
	drawLine(img, rect.Min.X, rect.Min.Y+28, rect.Max.X, rect.Min.Y+28, colorRed, 2)
	r.drawText(img, rect.Min.X+10, rect.Min.Y+18, "TODAY", colorRed, r.crispFace)
	r.drawText(img, rect.Min.X+92, rect.Min.Y+18, fmt.Sprintf("%d / 3", todayScore), colorBlack, r.headerFace)
	r.drawText(img, rect.Min.X+160, rect.Min.Y+18, "STREAK", colorRed, r.crispFace)
	r.drawText(img, rect.Min.X+246, rect.Min.Y+18, fmt.Sprintf("%dd", current), colorBlack, r.headerFace)

	r.drawText(img, rect.Min.X+10, rect.Min.Y+48, "BEST", colorRed, r.crispFace)
	r.drawText(img, rect.Min.X+68, rect.Min.Y+48, fmt.Sprintf("%dd", best), colorBlack, r.bodyFace)
	r.drawText(img, rect.Min.X+128, rect.Min.Y+48, "YTD", colorRed, r.crispFace)

	r.drawText(img, rect.Min.X+10, rect.Min.Y+74, "READ", colorRed, r.crispFace)
	r.drawText(img, rect.Min.X+72, rect.Min.Y+74, strconv.Itoa(ytd.Read), colorBlack, r.bodyFace)
	r.drawText(img, rect.Min.X+110, rect.Min.Y+74, "JOURNAL", colorRed, r.crispFace)
	r.drawText(img, rect.Min.X+204, rect.Min.Y+74, strconv.Itoa(ytd.Journal), colorBlack, r.bodyFace)

	r.drawText(img, rect.Min.X+10, rect.Min.Y+100, "WORKOUT", colorRed, r.crispFace)
	r.drawText(img, rect.Min.X+112, rect.Min.Y+100, strconv.Itoa(ytd.Workout), colorBlack, r.bodyFace)
	if todayScore < 3 {
		fillRect(img, image.Rect(rect.Max.X-28, rect.Min.Y+8, rect.Max.X-10, rect.Min.Y+26), colorRed)
	}
}

func monthWeeks(year int, month time.Month) [][]int {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	daysInMonth := first.AddDate(0, 1, -1).Day()
	offset := int(first.Weekday())
	weeks := [][]int{}
	week := make([]int, 7)
	for day := 1; day <= daysInMonth; day++ {
		index := (offset + day - 1) % 7
		week[index] = day
		if index == 6 || day == daysInMonth {
			weeks = append(weeks, week)
			week = make([]int, 7)
		}
	}
	return weeks
}

func drawBookLines(img draw.Image, x, y, w, h int) {
	drawLine(img, x, y, x+w, y, colorBlack, 3)
	drawLine(img, x, y+h/2, x+w, y+h/2, colorBlack, 3)
	drawLine(img, x, y+h, x+w, y+h, colorBlack, 3)
}

func drawJournalHatch(img draw.Image, x, y, w, h int) {
	for step := -h; step < w+h; step += 4 {
		drawLine(img, x+step, y+h, x+step+h, y, colorRed, 3)
	}
}

func drawBolt(img draw.Image, x, y, w, h int) {
	points := []image.Point{
		image.Pt(x+6, y),
		image.Pt(x, y+h/2),
		image.Pt(x+5, y+h/2),
		image.Pt(x+2, y+h),
		image.Pt(x+w, y+h/3),
		image.Pt(x+7, y+h/3),
	}
	for i := 0; i < len(points); i++ {
		next := points[(i+1)%len(points)]
		drawLine(img, points[i].X, points[i].Y, next.X, next.Y, colorBlack, 3)
	}
}

func (r *renderer) drawText(img draw.Image, x, y int, text string, col color.Color, face font.Face) {
	drawer := font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P(x, y),
	}
	drawer.DrawString(text)
}

func (r *renderer) drawCenteredText(img draw.Image, x, baseline, width int, text string, col color.Color, face font.Face) {
	drawer := font.Drawer{Face: face}
	textWidth := drawer.MeasureString(text).Round()
	r.drawText(img, x+(width-textWidth)/2, baseline, text, col, face)
}

func drawRect(img draw.Image, rect image.Rectangle, col color.Color, width int) {
	for i := 0; i < width; i++ {
		drawLine(img, rect.Min.X+i, rect.Min.Y+i, rect.Max.X-i, rect.Min.Y+i, col, 1)
		drawLine(img, rect.Min.X+i, rect.Max.Y-i, rect.Max.X-i, rect.Max.Y-i, col, 1)
		drawLine(img, rect.Min.X+i, rect.Min.Y+i, rect.Min.X+i, rect.Max.Y-i, col, 1)
		drawLine(img, rect.Max.X-i, rect.Min.Y+i, rect.Max.X-i, rect.Max.Y-i, col, 1)
	}
}

func fillRect(img draw.Image, rect image.Rectangle, col color.Color) {
	draw.Draw(img, rect, &image.Uniform{col}, image.Point{}, draw.Src)
}

func drawLine(img draw.Image, x0, y0, x1, y1 int, col color.Color, width int) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		for wx := -width / 2; wx <= width/2; wx++ {
			for wy := -width / 2; wy <= width/2; wy++ {
				setPixel(img, x0+wx, y0+wy, col)
			}
		}
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func setPixel(img draw.Image, x, y int, col color.Color) {
	if image.Pt(x, y).In(img.Bounds()) {
		img.Set(x, y, col)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func splitDisplayLayers(img image.Image) ([]byte, []byte) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	rowBytes := (width + 7) / 8
	black := bytes.Repeat([]byte{0xFF}, rowBytes*height)
	red := bytes.Repeat([]byte{0xFF}, rowBytes*height)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			rr := int(r >> 8)
			gg := int(g >> 8)
			bb := int(b >> 8)
			idx := y*rowBytes + x/8
			mask := byte(0x80 >> uint(x%8))

			if rr > displayRedThreshold && rr > gg+displayRedDelta && rr > bb+displayRedDelta {
				red[idx] &^= mask
				continue
			}
			gray := int(0.299*float64(rr) + 0.587*float64(gg) + 0.114*float64(bb))
			if gray < displayBlackThreshold {
				black[idx] &^= mask
			}
		}
	}
	return black, red
}

var indexTemplate = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
  <title>Habit ePaper</title>
  <style>
    :root {
      --bg: #f4f0e6;
      --ink: #181512;
      --paper: #fff9ef;
      --read: #eadfc2;
      --journal: #f2c7be;
      --workout: #cfdcce;
      --refresh: #d6dbe8;
      --active: #181512;
      --inactive: rgba(24,21,18,0.28);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      font-family: Georgia, "Times New Roman", serif;
      background:
        radial-gradient(circle at top, rgba(255,255,255,0.75), transparent 30%),
        linear-gradient(160deg, #ede5d2, var(--bg));
      color: var(--ink);
      display: grid;
      grid-template-rows: auto 1fr;
    }
    .status {
      padding: 12px 16px;
      background: rgba(255,249,239,0.92);
      border-bottom: 1px solid rgba(24,21,18,0.14);
      font-size: 14px;
      line-height: 1.4;
    }
    .grid {
      display: grid;
      grid-template-columns: 1fr 1fr;
      grid-template-rows: 1fr 1fr;
      min-height: calc(100vh - 70px);
    }
    button {
      border: 1px solid rgba(24,21,18,0.12);
      font: inherit;
      font-size: 11vw;
      font-weight: 700;
      letter-spacing: 0.04em;
      color: var(--ink);
      padding: 24px;
      text-transform: uppercase;
    }
    .read { background: var(--read); }
    .journal { background: var(--journal); }
    .workout { background: var(--workout); }
    .refresh { background: var(--refresh); }
    .active { box-shadow: inset 0 0 0 10px var(--active); }
    .inactive { box-shadow: inset 0 0 0 10px transparent; }
    .meta { font-size: 13px; opacity: 0.78; }
    @media (min-width: 700px) {
      body { place-items: center; }
      .status, .grid {
        width: min(520px, 100vw);
      }
      .grid { min-height: 720px; }
      button { font-size: 52px; }
    }
  </style>
</head>
<body>
  <div class="status">
    <div><strong>Today:</strong> <span id="day">{{.Today}}</span></div>
    <div class="meta" id="meta">Queued: {{.RefreshQueued}} | Last refresh: {{if .LastRefresh}}{{.LastRefresh}}{{else}}never{{end}}{{if .LastRefreshError}} | Error: {{.LastRefreshError}}{{end}}</div>
  </div>
  <div class="grid">
    <button class="workout inactive" data-habit="workout">Workout</button>
    <button class="read inactive" data-habit="read">Read</button>
    <button class="journal inactive" data-habit="journal">Journal</button>
    <button class="refresh inactive" data-action="refresh">Refresh</button>
  </div>
  <script>
    const buttons = {
      read: document.querySelector('[data-habit="read"]'),
      journal: document.querySelector('[data-habit="journal"]'),
      workout: document.querySelector('[data-habit="workout"]'),
      refresh: document.querySelector('[data-action="refresh"]'),
    };
    const meta = document.getElementById('meta');

    async function post(url) {
      const res = await fetch(url, { method: 'POST' });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'request failed');
      return data;
    }

    function setActive(button, active) {
      button.classList.toggle('active', !!active);
      button.classList.toggle('inactive', !active);
    }

    async function syncState() {
      const res = await fetch('/state');
      const data = await res.json();
      setActive(buttons.read, data.read === 1);
      setActive(buttons.journal, data.journal === 1);
      setActive(buttons.workout, data.workout === 1);
      setActive(buttons.refresh, data.refresh_queued);
      const errorText = data.last_refresh_error ? ' | Error: ' + data.last_refresh_error : '';
      meta.textContent = 'Queued: ' + data.refresh_queued + ' | Last refresh: ' + (data.last_refresh || 'never') + errorText;
    }

    document.querySelectorAll('[data-habit]').forEach((button) => {
      button.addEventListener('click', async () => {
        await post('/habit/toggle?habit=' + button.dataset.habit);
        await syncState();
      });
    });
    buttons.refresh.addEventListener('click', async () => {
      await post('/refresh');
      await syncState();
    });
    syncState();
    setInterval(syncState, 10000);
  </script>
</body>
</html>`))
