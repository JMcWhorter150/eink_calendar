import calendar
import os
from dataclasses import dataclass
from datetime import date, datetime

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


ASSET_DIR = os.path.join(os.path.dirname(__file__), "assets")

CHARACTERS = [
    {"key": "cat", "label": "Mood Cat"},
    {"key": "fire", "label": "Fire Critter"},
    {"key": "ground", "label": "Ground Critter"},
    {"key": "electric", "label": "Electric Bird"},
    {"key": "goo", "label": "Goo Critter"},
]


ROTATION_ANCHOR = date(2026, 1, 1)
def _ensure_character_assets(cfg):
    os.makedirs(ASSET_DIR, exist_ok=True)

    def _save(img, name):
        path = os.path.join(ASSET_DIR, name)
        if not os.path.exists(path):
            img.save(path)

    def _base():
        return Image.new("RGB", (140, 140), cfg.white)

    # Mood cat
    img = _base()
    d = ImageDraw.Draw(img)
    d.ellipse((20, 25, 120, 125), outline=cfg.black, width=3)
    d.polygon([(28, 45), (16, 10), (48, 30)], outline=cfg.black)
    d.polygon([(112, 45), (124, 10), (92, 30)], outline=cfg.black)
    d.polygon([(28, 45), (22, 18), (42, 30)], outline=cfg.red, fill=cfg.red)
    d.polygon([(112, 45), (118, 18), (98, 30)], outline=cfg.red, fill=cfg.red)
    d.ellipse((45, 60, 55, 70), fill=cfg.black)
    d.ellipse((85, 60, 95, 70), fill=cfg.black)
    d.arc((50, 80, 90, 110), 200, 340, fill=cfg.black, width=3)
    d.ellipse((68, 80, 72, 84), fill=cfg.red)
    _save(img, "cat.png")

    # Fire critter
    img = _base()
    d = ImageDraw.Draw(img)
    d.ellipse((30, 40, 110, 120), outline=cfg.black, width=3)
    d.polygon([(70, 10), (55, 40), (85, 40)], outline=cfg.red, fill=cfg.red)
    d.polygon([(75, 60), (120, 75), (75, 90)], outline=cfg.black, fill=(200, 100, 0))
    d.ellipse((55, 70, 65, 80), fill=cfg.black)
    _save(img, "fire.png")

    # Ground critter
    img = _base()
    d = ImageDraw.Draw(img)
    d.ellipse((25, 30, 115, 120), outline=cfg.black, width=3)
    d.arc((40, 50, 100, 100), 0, 180, fill=cfg.black, width=3)
    d.arc((40, 40, 100, 80), 200, 340, fill=cfg.red, width=3)
    for x in (40, 60, 80):
        d.line((x, 110, x - 8, 130), fill=cfg.black, width=3)
    _save(img, "ground.png")

    # Electric bird
    img = _base()
    d = ImageDraw.Draw(img)
    d.polygon(
        [(20, 70), (60, 40), (70, 70), (90, 45), (110, 80), (70, 90)],
        outline=cfg.black,
        fill=(255, 220, 0),
    )
    d.polygon([(70, 90), (80, 120), (60, 120)], outline=cfg.black, fill=(255, 220, 0))
    d.ellipse((60, 60, 70, 70), fill=cfg.black)
    d.line((70, 90, 95, 115), fill=cfg.red, width=3)
    _save(img, "electric.png")

    # Goo critter
    img = _base()
    d = ImageDraw.Draw(img)
    d.ellipse((25, 50, 115, 120), outline=cfg.black, width=3)
    d.ellipse((40, 45, 100, 100), outline=cfg.black, width=3)
    d.ellipse((55, 70, 60, 75), fill=cfg.black)
    d.ellipse((80, 70, 85, 75), fill=cfg.black)
    d.line((60, 90, 85, 90), fill=cfg.black, width=3)
    d.ellipse((48, 88, 54, 94), fill=cfg.red)
    d.ellipse((90, 88, 96, 94), fill=cfg.red)
    _save(img, "goo.png")


def _select_character(today):
    quarter = (today.month - 1) // 3
    anchor_quarter = (ROTATION_ANCHOR.month - 1) // 3
    quarters_since = (today.year - ROTATION_ANCHOR.year) * 4 + (quarter - anchor_quarter)
    idx = quarters_since % len(CHARACTERS)
    return CHARACTERS[idx]


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


def _draw_mood_cat(draw, box, level, cfg):
    # Simple cat face with mouth curvature based on level
    x0, y0, x1, y1 = box
    cx = (x0 + x1) // 2
    cy = (y0 + y1) // 2
    r = min(x1 - x0, y1 - y0) // 2 - 4

    # Head
    draw.ellipse((cx - r, cy - r, cx + r, cy + r), outline=cfg.black, width=2)
    # Ears
    draw.polygon([(cx - r + 4, cy - r + 6), (cx - r - 6, cy - r - 12), (cx - r + 16, cy - r - 6)], outline=cfg.black)
    draw.polygon([(cx + r - 4, cy - r + 6), (cx + r + 6, cy - r - 12), (cx + r - 16, cy - r - 6)], outline=cfg.black)

    # Eyes
    eye_offset_x = r // 2
    eye_offset_y = r // 5
    draw.ellipse((cx - eye_offset_x - 6, cy - eye_offset_y - 4, cx - eye_offset_x + 6, cy - eye_offset_y + 4), fill=cfg.black)
    draw.ellipse((cx + eye_offset_x - 6, cy - eye_offset_y - 4, cx + eye_offset_x + 6, cy - eye_offset_y + 4), fill=cfg.black)

    # Mouth: map level 0..9 to curvature
    # negative for frown, positive for smile
    curvature = (level - 4.5) / 4.5  # -1..1
    mouth_y = cy + r // 3
    mouth_w = r // 2
    mouth_h = int(r // 3 * abs(curvature))
    if curvature >= 0:
        # smile arc
        draw.arc((cx - mouth_w, mouth_y - mouth_h, cx + mouth_w, mouth_y + mouth_h), 200, 340, fill=cfg.black, width=2)
    else:
        # frown arc
        draw.arc((cx - mouth_w, mouth_y - mouth_h, cx + mouth_w, mouth_y + mouth_h), 20, 160, fill=cfg.black, width=2)


def _draw_character(img, draw, box, level, cfg, today):
    _ensure_character_assets(cfg)
    character = _select_character(today)
    key = character["key"]
    if key == "cat":
        _draw_mood_cat(draw, box, level, cfg)
        return character

    path = os.path.join(ASSET_DIR, f"{key}.png")
    try:
        char_img = Image.open(path).convert("RGB")
        w = box[2] - box[0]
        h = box[3] - box[1]
        char_img = char_img.resize((w, h))
        img.paste(char_img, (box[0], box[1]))
    except OSError:
        _draw_mood_cat(draw, box, level, cfg)
        character = {"key": "cat", "label": "Mood Cat"}
    return character

def _draw_book(draw, x, y, w, h, cfg):
    # Three horizontal lines
    spacing = h // 4
    for i in range(3):
        y_line = y + i * spacing
        draw.line((x, y_line, x + w, y_line), fill=cfg.black, width=2)


def _draw_hatch(draw, x, y, w, h, cfg):
    # Diagonal red hatch
    step = 5
    for i in range(-h, w, step):
        draw.line((x + i, y, x + i + h, y + h), fill=cfg.red, width=1)


def _draw_bolt(draw, x, y, w, h, cfg):
    # Simple lightning bolt
    points = [
        (x + w * 0.2, y),
        (x + w * 0.6, y),
        (x + w * 0.4, y + h * 0.55),
        (x + w * 0.8, y + h * 0.55),
        (x + w * 0.3, y + h),
        (x + w * 0.5, y + h * 0.45),
    ]
    draw.line(points, fill=cfg.black, width=2)


def render_month(year, month, month_data, ytd_totals, mood_level, today=None, cfg=None):
    cfg = cfg or RenderConfig()
    img = Image.new("RGB", (cfg.width, cfg.height), cfg.white)
    draw = ImageDraw.Draw(img)

    if today is None:
        today = date.today()

    title_font = _load_font(28, bold=True)
    small_font = _load_font(14)
    medium_font = _load_font(18)

    month_name = date(year, month, 1).strftime("%B %Y")
    draw.text((cfg.margin, cfg.margin), month_name, font=title_font, fill=cfg.black)

    ytd_text = f"YTD totals  Read: {ytd_totals['read']}  Journal: {ytd_totals['journal']}  Workout: {ytd_totals['workout']}"
    draw.text((cfg.margin, cfg.margin + 36), ytd_text, font=small_font, fill=cfg.black)

    # Mood cat box
    cat_box = (cfg.width - 160, cfg.margin - 49, cfg.width - 20, cfg.margin + 91)
    character = _draw_character(img, draw, cat_box, mood_level, cfg, today)

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
                _draw_book(draw, icon_x, icon_y, icon_w, icon_h, cfg)
            if journal:
                _draw_hatch(draw, icon_x + icon_w + 8, icon_y, icon_w, icon_h, cfg)
            if workout:
                _draw_bolt(draw, icon_x + 2 * (icon_w + 8), icon_y, icon_w, icon_h, cfg)

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
