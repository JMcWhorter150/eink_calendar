import calendar
from dataclasses import dataclass
from datetime import date

from PIL import Image, ImageDraw, ImageFont


@dataclass
class RenderConfig:
    width: int = 800
    height: int = 480
    margin: int = 20
    red: tuple = (200, 0, 0)
    black: tuple = (0, 0, 0)
    white: tuple = (255, 255, 255)


def _load_font(size, bold=False):
    # Try common DejaVu fonts; fall back to default bitmap font.
    candidates = [
        ("/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf" if bold else "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"),
        ("/usr/share/fonts/truetype/freefont/FreeSansBold.ttf" if bold else "/usr/share/fonts/truetype/freefont/FreeSans.ttf"),
    ]
    for path in candidates:
        try:
            return ImageFont.truetype(path, size=size)
        except OSError:
            continue
    return ImageFont.load_default()


def compute_weighted_mood(last_14_rows):
    # last_14_rows: list of (day_iso, read, journal, workout), oldest->newest
    if not last_14_rows:
        return 0
    weights = list(range(1, 15))
    total_weight = sum(weights)
    score = 0.0
    for i, row in enumerate(last_14_rows[-14:]):
        _, read, journal, workout = row
        daily = (read + journal + workout) / 3.0
        score += daily * weights[i]
    normalized = score / total_weight
    level = int(round(normalized * 9))
    return max(0, min(9, level))


def _draw_streak_widget(draw, box, streak_current, streak_best, today_score, cfg):
    x0, y0, x1, y1 = box
    draw.rectangle((x0, y0, x1, y1), outline=cfg.black, width=1)
    draw.line((x0, y0 + 24, x1, y0 + 24), fill=cfg.black, width=1)

    title_font = _load_font(14, bold=True)
    value_font = _load_font(13)
    draw.text((x0 + 8, y0 + 4), "Streak", font=title_font, fill=cfg.black)
    draw.text((x0 + 60, y0 + 4), f"{streak_current}d / best {streak_best}d", font=value_font, fill=cfg.black)
    draw.text((x0 + 8, y0 + 30), "Today", font=title_font, fill=cfg.black)
    draw.text((x0 + 60, y0 + 30), f"{today_score}/3", font=value_font, fill=cfg.red if today_score < 3 else cfg.black)


def _draw_read_dots(draw, x, y, w, h, cfg):
    radius = max(2, min(w, h) // 7)
    centers = (
        (x + w * 0.2, y + h * 0.5),
        (x + w * 0.5, y + h * 0.5),
        (x + w * 0.8, y + h * 0.5),
    )
    for cx, cy in centers:
        draw.ellipse((cx - radius, cy - radius, cx + radius, cy + radius), fill=cfg.black)


def _draw_journal_slashes(draw, x, y, w, h, cfg):
    step = max(4, w // 4)
    for i in range(-h, w + h, step):
        draw.line((x + i, y + h, x + i + h, y), fill=cfg.red, width=2)


def _draw_workout_checker(draw, x, y, w, h, cfg):
    draw.rectangle((x, y, x + w, y + h), outline=cfg.black, width=1)
    cols = 4
    rows = 3
    cell_w = max(2, w // cols)
    cell_h = max(2, h // rows)
    for r in range(rows):
        for c in range(cols):
            if (r + c) % 2 == 0:
                cx0 = x + c * cell_w + 1
                cy0 = y + r * cell_h + 1
                cx1 = min(x + (c + 1) * cell_w - 1, x + w - 1)
                cy1 = min(y + (r + 1) * cell_h - 1, y + h - 1)
                if cx1 > cx0 and cy1 > cy0:
                    draw.rectangle((cx0, cy0, cx1, cy1), fill=cfg.black)


def render_month(year, month, month_data, ytd_totals, mood_level, today=None, cfg=None, streak_current=0, streak_best=0, today_score=0):
    cfg = cfg or RenderConfig()
    img = Image.new("RGB", (cfg.width, cfg.height), cfg.white)
    draw = ImageDraw.Draw(img)

    if today is None:
        today = date.today()

    title_font = _load_font(28, bold=True)
    small_font = _load_font(14)
    month_name = date(year, month, 1).strftime("%B %Y")
    draw.text((cfg.margin, cfg.margin), month_name, font=title_font, fill=cfg.black)

    ytd_text = f"YTD totals  Read: {ytd_totals['read']}  Journal: {ytd_totals['journal']}  Workout: {ytd_totals['workout']}"
    draw.text((cfg.margin, cfg.margin + 36), ytd_text, font=small_font, fill=cfg.black)

    widget_box = (cfg.width - 260, cfg.margin, cfg.width - cfg.margin, cfg.margin + 52)
    _draw_streak_widget(draw, widget_box, streak_current, streak_best, today_score, cfg)

    # Grid
    grid_top = cfg.margin + 80
    grid_left = cfg.margin
    grid_right = cfg.width - cfg.margin
    grid_bottom = cfg.height - cfg.margin

    cal = calendar.Calendar(firstweekday=6)  # Sunday
    weeks = cal.monthdayscalendar(year, month)
    num_weeks = len(weeks)
    header_height = 24
    cell_w = (grid_right - grid_left) // 7
    cell_h = (grid_bottom - grid_top - header_height) // max(num_weeks, 1)

    # Weekday headers
    weekdays = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]
    for i, name in enumerate(weekdays):
        x = grid_left + i * cell_w + 4
        y = grid_top
        draw.text((x, y), name, font=small_font, fill=cfg.black)

    # Grid lines
    for i in range(8):
        x = grid_left + i * cell_w
        draw.line((x, grid_top + header_height, x, grid_top + header_height + cell_h * num_weeks), fill=cfg.black, width=1)
    for r in range(num_weeks + 1):
        y = grid_top + header_height + r * cell_h
        draw.line((grid_left, y, grid_right, y), fill=cfg.black, width=1)

    # Days and marks
    for r, week in enumerate(weeks):
        for c, day_num in enumerate(week):
            if day_num == 0:
                continue
            x0 = grid_left + c * cell_w
            y0 = grid_top + header_height + r * cell_h
            x1 = x0 + cell_w
            y1 = y0 + cell_h

            # Day number
            draw.text((x0 + 4, y0 + 2), str(day_num), font=small_font, fill=cfg.black)

            # Today highlight
            if today.year == year and today.month == month and today.day == day_num:
                draw.rectangle((x0 + 1, y0 + 1, x1 - 1, y1 - 1), outline=cfg.red, width=2)

            read, journal, workout = month_data.get(day_num, (0, 0, 0))

            icon_w = cell_w // 4
            icon_h = cell_h // 4
            icon_y = y1 - icon_h - 6
            icon_x = x0 + 6

            if read:
                _draw_read_dots(draw, icon_x, icon_y, icon_w, icon_h, cfg)
            if journal:
                _draw_journal_slashes(draw, icon_x + icon_w + 8, icon_y, icon_w, icon_h, cfg)
            if workout:
                _draw_workout_checker(draw, icon_x + 2 * (icon_w + 8), icon_y, icon_w, icon_h, cfg)

    return img


def render_test_png(year=None, month=None, path="out.png"):
    today = date.today()
    year = year or today.year
    month = month or today.month
    # Dummy data for visual inspection
    month_data = {d: (d % 2, (d + 1) % 2, (d + 2) % 2) for d in range(1, 29)}
    ytd_totals = {"read": 100, "journal": 80, "workout": 60}
    mood_level = 6
    img = render_month(year, month, month_data, ytd_totals, mood_level, today=today)
    img.save(path)
    return path
