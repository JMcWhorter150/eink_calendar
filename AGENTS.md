# AGENTS.md — Habit ePaper Implementation Guide (Codex Agent)

## Agent role
You are implementing a Raspberry Pi e-paper habit tracker with FastAPI + SQLite + Pillow rendering and Waveshare display output.

You must:
- Follow PLANS.md milestone order
- Keep changes minimal and verifiable
- Prefer correctness and robustness over cleverness
- Avoid long refactors; land incremental commits

---

## Constraints & assumptions
- Hardware: Raspberry Pi Zero W + Waveshare 7.5" tri-color B/W/R (800x480)
- SPI enabled and Waveshare library already functional on device
- Display driver module is expected to be: `waveshare_epd.epd7in5b_V2`
- Python 3 available (prefer 3.9+). Do not require heavy dependencies.

---

## Implementation rules
1. Do not implement partial refresh.
2. Enforce display refresh throttling (300s minimum interval).
3. Display update must not block the FastAPI request handler thread.
   - Use a background worker thread that coalesces refresh requests.
4. Use SQLite directly (no ORM).
5. All endpoints must be idempotent and return JSON.

---

## Step-by-step tasks (do in order)

### Task A — Create project files
- Create directory: `habit_epaper/`
- Create:
  - `requirements.txt`
  - `db.py`
  - `render.py`
  - `epaper_display.py`
  - `app.py`

### Task B — Database layer (`db.py`)
Implement:
- `init_db()`
- `set_habit(date, habit, value)`
- `toggle_habit(date, habit)`
- `get_month(year, month)` -> dict day(int)->(read, journal, workout)
- `ytd_counts(year)` -> dict habit->int
- `last_n_days_rows(end_day, n)` -> list of (day_iso, read, journal, workout) with zeros filled
- meta helpers:
  - `set_meta(k, v)`
  - `get_meta(k, default=None)`
  - OPTIONAL (recommended): `delete_meta(k)` to truly clear override

**Verify**
- Run `python -c "from habit_epaper.db import init_db; init_db(); print('ok')"`

### Task C — Rendering (`render.py`)
Implement:
- RenderConfig (800x480, red accent)
- Weighted 14-day mood logic:
  - weights 1..14, newest weighted more
  - map to 0..9
- Draw:
  - month title
  - YTD totals line
  - grid with weekday headers
  - today highlight in red
  - habit marks:
    - read: black “book lines”
    - journal: red hatch
    - workout: black bolt
  - mood cat (10-state)

**Verify**
- Add a temporary test snippet or function that outputs an `out.png` for inspection:
  - month grid is aligned
  - marks appear
  - cat drawn
- Do not commit a permanent debug script unless asked; keep tests minimal.

### Task D — Display integration (`epaper_display.py`)
Implement:
- RGB -> two 1-bit images (black layer and red layer)
- Determine “red-ish” pixels via thresholding (simple heuristic)
- Send to panel using:
  - `epd = epd7in5b_V2.EPD()`
  - `epd.init(); epd.Clear(); epd.display(epd.getbuffer(black), epd.getbuffer(red)); epd.sleep()`

**Critical**
- If `epd.display` signature differs, adapt accordingly by inspecting `inspect.signature`.
- Do NOT assume single-buffer display for tri-color.

**Verify**
- Provide a one-shot manual call path from `app.py` `/refresh`.
- Ensure it doesn’t hang at “e-Paper busy”.

### Task E — FastAPI server (`app.py`)
Implement endpoints:
- `GET /health`
- `POST /habit/set`
- `POST /habit/toggle`
- `POST /mood/override`
- `POST /mood/clear_override`
- `POST /refresh`

Implement background refresh worker:
- Global: `_refresh_requested`, `_last_refresh_ts`
- `MIN_SECONDS_BETWEEN_REFRESH = 300`
- Worker loop:
  - sleep(1)
  - if refresh requested AND interval elapsed:
    - render current month
    - compute mood (override OR weighted 14 days)
    - compute YTD totals
    - display image

**Verify**
- Run server locally
- Use curl to toggle habits and call refresh

### Task F — Systemd deployment files (document + provide exact contents)
Create/Install:
- `/etc/systemd/system/habit-epaper.service`
- `/etc/systemd/system/habit-epaper-refresh.service`
- `/etc/systemd/system/habit-epaper-refresh.timer`

Timer:
- `OnCalendar=*-*-* 06:00:00`
- `Persistent=true`

Service must:
- start after network-online
- restart on failure
- run uvicorn on port 8000
- run with appropriate permissions for SPI/GPIO (typically via sudo)

**Verify**
- `systemctl enable --now habit-epaper.service`
- `systemctl enable --now habit-epaper-refresh.timer`
- `list-timers` shows next run at 6:00

---

## Curl examples (must work)
- Toggle read:
  - `curl -X POST 'http://<pi>:8000/habit/toggle?habit=read'`
- Set workout:
  - `curl -X POST 'http://<pi>:8000/habit/set?habit=workout&value=1'`
- Override mood:
  - `curl -X POST 'http://<pi>:8000/mood/override?level=7'`
- Refresh:
  - `curl -X POST 'http://<pi>:8000/refresh'`

---

## Troubleshooting guidance (for agent)
If display hangs:
1. Confirm correct driver module for tri-color: `epd7in5b_V2`
2. Inspect `display()` signature:
   - `python -c "from waveshare_epd import epd7in5b_V2; import inspect; epd=epd7in5b_V2.EPD(); print(inspect.signature(epd.display))"`
3. Confirm BUSY pin toggles during init if needed.
4. Verify SPI devices exist: `/dev/spidev0.0`, `/dev/spidev0.1`

---

## Completion checklist
- [ ] All python files exist and import cleanly
- [ ] Local render produces a sensible calendar image
- [ ] FastAPI endpoints work
- [ ] Display updates
- [ ] Coalescing (5 min) works (multiple calls don’t force immediate refresh)
- [ ] Daily 6am timer works
- [ ] Boot service works

---

## Output format expectations
When done, provide:
- Final code in repo
- Exact systemd unit contents
- Exact commands to install + enable services
- A short “how to use” section with curl examples and tailscale notes

