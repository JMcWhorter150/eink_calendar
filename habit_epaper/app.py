import os
import threading
import time
from datetime import date, datetime

from typing import Optional

from fastapi import FastAPI, HTTPException

from . import db, render
from .epaper_display import display_image

app = FastAPI()

MIN_SECONDS_BETWEEN_REFRESH = 300

_refresh_requested = False
_last_refresh_ts = 0.0
_lock = threading.Lock()


def _parse_day(day_str):
    if day_str is None:
        return date.today()
    try:
        return datetime.fromisoformat(day_str).date()
    except ValueError:
        raise HTTPException(status_code=400, detail="Invalid day format; use YYYY-MM-DD")


def _compute_mood_level(today):
    override = db.get_meta("mood_override")
    if override is not None:
        try:
            level = int(override)
            return max(0, min(9, level))
        except ValueError:
            return 0

    rows = db.last_n_days_rows(today, 14)
    return render.compute_weighted_mood(rows)


def _compute_streak_stats(today):
    n_days = max(1, today.timetuple().tm_yday)
    rows = db.last_n_days_rows(today, n_days)

    best = 0
    current = 0
    running = 0
    for _, read, journal, workout in rows:
        is_complete = int(read) + int(journal) + int(workout) == 3
        if is_complete:
            running += 1
            if running > best:
                best = running
        else:
            running = 0
    current = running
    return {"current": current, "best": best}


def _do_refresh():
    today = date.today()
    month_data = db.get_month(today.year, today.month)
    ytd_totals = db.ytd_counts(today.year)
    mood_level = _compute_mood_level(today)
    streak_stats = _compute_streak_stats(today)
    today_score = sum(month_data.get(today.day, (0, 0, 0)))

    img = render.render_month(
        today.year,
        today.month,
        month_data,
        ytd_totals,
        mood_level,
        today=today,
        streak_current=streak_stats["current"],
        streak_best=streak_stats["best"],
        today_score=today_score,
    )

    if os.environ.get("HABIT_EPAPER_DISABLE_DISPLAY"):
        return
    display_image(img)


def _refresh_worker():
    global _refresh_requested, _last_refresh_ts
    while True:
        time.sleep(1)
        with _lock:
            requested = _refresh_requested
            elapsed = time.time() - _last_refresh_ts
            if requested and elapsed >= MIN_SECONDS_BETWEEN_REFRESH:
                _refresh_requested = False
                _last_refresh_ts = time.time()
                should_run = True
            else:
                should_run = False
        if should_run:
            try:
                _do_refresh()
            except Exception:
                # Avoid crashing the worker; errors will show in logs.
                pass


@app.on_event("startup")
def _startup():
    db.init_db()
    t = threading.Thread(target=_refresh_worker, daemon=True)
    t.start()


@app.get("/health")
def health():
    return {"ok": True}


@app.post("/habit/set")
def habit_set(habit: str, value: int, day: Optional[str] = None):
    day_value = _parse_day(day)
    try:
        db.set_habit(day_value, habit, value)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc))
    return {"ok": True, "habit": habit, "value": 1 if int(value) else 0, "day": day_value.isoformat()}


@app.post("/habit/toggle")
def habit_toggle(habit: str, day: Optional[str] = None):
    day_value = _parse_day(day)
    try:
        new_value = db.toggle_habit(day_value, habit)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc))
    return {"ok": True, "habit": habit, "value": new_value, "day": day_value.isoformat()}


@app.post("/mood/override")
def mood_override(level: int):
    if level < 0 or level > 9:
        raise HTTPException(status_code=400, detail="level must be 0..9")
    db.set_meta("mood_override", str(level))
    return {"ok": True, "level": level}


@app.post("/mood/clear_override")
def mood_clear_override():
    db.delete_meta("mood_override")
    return {"ok": True}


@app.post("/refresh")
def refresh():
    global _refresh_requested
    with _lock:
        _refresh_requested = True
    return {"ok": True, "queued": True}
