# PLANS.md — Habit ePaper (Waveshare 7.5" B/W/R) Calendar + FastAPI

## Goal
Build a Raspberry Pi service that:
- Tracks daily yes/no habits (read, journal, workout) in SQLite
- Renders a current-month calendar (800x480) to Waveshare 7.5" tri-color B/W/R
- Shows YTD totals and a 10-state mood cat (weighted last 14 days) with manual override
- Exposes a FastAPI API for updates via curl
- Updates display no more than once every 5 minutes (coalesced)
- Forces a refresh daily at 6:00 AM via systemd timer
- Runs on boot via systemd service

## Non-goals (v1)
- Partial refresh
- Web UI
- Multiple months navigation
- Complex metrics beyond yes/no

---

## Repo layout (target)
habit_epaper/
app.py
db.py
render.py
epaper_display.py
requirements.txt
PLANS.md
AGENTS.md


---

## Design decisions
- Database: SQLite with two tables:
  - `daily(day TEXT PK, read INT, journal INT, workout INT)`
  - `meta(k TEXT PK, v TEXT)` for mood override
- Rendering: Pillow drawing to RGB image, then split into black/red 1-bit layers
- Display driver: `waveshare_epd.epd7in5b_V2`
- Refresh policy: background worker thread coalesces requests and enforces 300s minimum interval
- Daily refresh: systemd timer hitting localhost `/refresh`

---

## Milestones
### M1 — Core app runs locally (no display)
- [ ] `requirements.txt` created
- [ ] `db.py` created and initializes DB
- [ ] `render.py` renders an image file (PNG) for visual inspection
- [ ] `app.py` serves endpoints and writes DB correctly

**Verification**
- Run:
  - `pip install -r habit_epaper/requirements.txt`
  - `python -c "from habit_epaper.db import init_db; init_db(); print('ok')"`
  - Render test (agent may add a temporary script): save `out.png` and open to verify calendar layout
  - Start API: `uvicorn habit_epaper.app:app --host 127.0.0.1 --port 8000`
  - Curl:
    - `curl -X POST 'http://127.0.0.1:8000/habit/toggle?habit=read'`
    - `curl -X POST 'http://127.0.0.1:8000/mood/override?level=7'`

---

### M2 — Display integration works
- [ ] `epaper_display.py` splits RGB -> (black, red) and calls EPD correctly
- [ ] `app.py` refresh worker calls render + display
- [ ] A manual curl to `/refresh` updates the panel

**Verification**
- On Pi:
  - `sudo uvicorn habit_epaper.app:app --host 0.0.0.0 --port 8000`
  - `curl -X POST 'http://127.0.0.1:8000/refresh'`
  - Confirm display changes
- Confirm no permanent `e-Paper busy`

---

### M3 — Systemd boot + daily refresh
- [ ] `/etc/systemd/system/habit-epaper.service` created
- [ ] `/etc/systemd/system/habit-epaper-refresh.service` created
- [ ] `/etc/systemd/system/habit-epaper-refresh.timer` created
- [ ] Service enabled and running
- [ ] Timer enabled and next run shows at 06:00

**Verification**
- `sudo systemctl daemon-reload`
- `sudo systemctl enable --now habit-epaper.service`
- `sudo systemctl status habit-epaper.service --no-pager`
- `sudo systemctl enable --now habit-epaper-refresh.timer`
- `systemctl list-timers --all | grep habit-epaper`
- `journalctl -u habit-epaper.service -n 200 --no-pager`

---

## Acceptance criteria (Definition of Done)
- API supports:
  - `GET /health`
  - `POST /habit/set?habit=...&value=...&day=YYYY-MM-DD(optional)`
  - `POST /habit/toggle?habit=...&day=YYYY-MM-DD(optional)`
  - `POST /mood/override?level=0..9`
  - `POST /mood/clear_override`
  - `POST /refresh`
- Display shows:
  - Current month grid
  - Today highlighted
  - Marks for each habit per day
  - Mood cat (10 levels)
  - YTD totals for each habit
- Refresh behavior:
  - No more than once per 300 seconds, even with repeated API calls
  - A refresh is guaranteed by 6:00 AM daily timer
- Runs on boot via systemd

---

## Operational notes
- Use Tailscale IP to curl remotely: `http://<tailscale-ip>:8000/...`
- Prefer 5-minute coalescing to avoid excessive tri-color refreshes
- If display API differs (some Waveshare variants), adapt `epaper_display.py` based on:
  - `inspect.signature(epd.display)`
  - `inspect.signature(epd.getbuffer)`

