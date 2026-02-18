import os
import sqlite3
from datetime import date, datetime, timedelta

DB_PATH = os.path.join(os.path.dirname(__file__), "habit.db")


def _connect():
    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    return conn


def init_db():
    conn = _connect()
    try:
        cur = conn.cursor()
        cur.execute(
            """
            CREATE TABLE IF NOT EXISTS daily (
                day TEXT PRIMARY KEY,
                read INTEGER NOT NULL DEFAULT 0,
                journal INTEGER NOT NULL DEFAULT 0,
                workout INTEGER NOT NULL DEFAULT 0
            )
            """
        )
        cur.execute(
            """
            CREATE TABLE IF NOT EXISTS meta (
                k TEXT PRIMARY KEY,
                v TEXT
            )
            """
        )
        conn.commit()
    finally:
        conn.close()


def _day_iso(day_value):
    if isinstance(day_value, date):
        return day_value.isoformat()
    if isinstance(day_value, datetime):
        return day_value.date().isoformat()
    if isinstance(day_value, str):
        return day_value
    raise ValueError("day must be date, datetime, or ISO string")


def set_habit(day_value, habit, value):
    if habit not in ("read", "journal", "workout"):
        raise ValueError("invalid habit")
    day_iso = _day_iso(day_value)
    value_int = 1 if int(value) else 0

    conn = _connect()
    try:
        cur = conn.cursor()
        cur.execute("INSERT OR IGNORE INTO daily(day) VALUES (?)", (day_iso,))
        cur.execute(
            f"UPDATE daily SET {habit} = ? WHERE day = ?",
            (value_int, day_iso),
        )
        conn.commit()
    finally:
        conn.close()


def toggle_habit(day_value, habit):
    if habit not in ("read", "journal", "workout"):
        raise ValueError("invalid habit")
    day_iso = _day_iso(day_value)

    conn = _connect()
    try:
        cur = conn.cursor()
        cur.execute("INSERT OR IGNORE INTO daily(day) VALUES (?)", (day_iso,))
        cur.execute(f"SELECT {habit} FROM daily WHERE day = ?", (day_iso,))
        row = cur.fetchone()
        current = int(row[0]) if row and row[0] is not None else 0
        new_value = 0 if current else 1
        cur.execute(
            f"UPDATE daily SET {habit} = ? WHERE day = ?",
            (new_value, day_iso),
        )
        conn.commit()
        return new_value
    finally:
        conn.close()


def get_month(year, month):
    # Returns dict day(int) -> (read, journal, workout)
    first = date(year, month, 1)
    if month == 12:
        next_month = date(year + 1, 1, 1)
    else:
        next_month = date(year, month + 1, 1)
    days_in_month = (next_month - first).days

    data = {d: (0, 0, 0) for d in range(1, days_in_month + 1)}

    conn = _connect()
    try:
        cur = conn.cursor()
        cur.execute(
            """
            SELECT day, read, journal, workout
            FROM daily
            WHERE day >= ? AND day < ?
            """,
            (first.isoformat(), next_month.isoformat()),
        )
        for row in cur.fetchall():
            day_num = int(row["day"].split("-")[-1])
            data[day_num] = (int(row["read"]), int(row["journal"]), int(row["workout"]))
    finally:
        conn.close()

    return data


def ytd_counts(year):
    start = date(year, 1, 1).isoformat()
    end = date(year + 1, 1, 1).isoformat()

    conn = _connect()
    try:
        cur = conn.cursor()
        cur.execute(
            """
            SELECT
                COALESCE(SUM(read), 0) AS read,
                COALESCE(SUM(journal), 0) AS journal,
                COALESCE(SUM(workout), 0) AS workout
            FROM daily
            WHERE day >= ? AND day < ?
            """,
            (start, end),
        )
        row = cur.fetchone()
        return {
            "read": int(row["read"]),
            "journal": int(row["journal"]),
            "workout": int(row["workout"]),
        }
    finally:
        conn.close()


def last_n_days_rows(end_day, n):
    end_iso = _day_iso(end_day)
    end_dt = datetime.fromisoformat(end_iso).date()
    start_dt = end_dt - timedelta(days=n - 1)

    conn = _connect()
    try:
        cur = conn.cursor()
        cur.execute(
            """
            SELECT day, read, journal, workout
            FROM daily
            WHERE day >= ? AND day <= ?
            """,
            (start_dt.isoformat(), end_dt.isoformat()),
        )
        rows = {row["day"]: (int(row["read"]), int(row["journal"]), int(row["workout"])) for row in cur.fetchall()}
    finally:
        conn.close()

    result = []
    for i in range(n):
        day = start_dt + timedelta(days=i)
        day_iso = day.isoformat()
        read, journal, workout = rows.get(day_iso, (0, 0, 0))
        result.append((day_iso, read, journal, workout))
    return result


def set_meta(k, v):
    conn = _connect()
    try:
        cur = conn.cursor()
        cur.execute("INSERT OR REPLACE INTO meta(k, v) VALUES (?, ?)", (k, v))
        conn.commit()
    finally:
        conn.close()


def get_meta(k, default=None):
    conn = _connect()
    try:
        cur = conn.cursor()
        cur.execute("SELECT v FROM meta WHERE k = ?", (k,))
        row = cur.fetchone()
        if row is None:
            return default
        return row["v"]
    finally:
        conn.close()


def delete_meta(k):
    conn = _connect()
    try:
        cur = conn.cursor()
        cur.execute("DELETE FROM meta WHERE k = ?", (k,))
        conn.commit()
    finally:
        conn.close()
