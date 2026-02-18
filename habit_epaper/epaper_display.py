import inspect

from PIL import Image


def split_black_red(img, red_threshold=150, red_delta=40, black_threshold=200):
    rgb = img.convert("RGB")
    w, h = rgb.size

    black = Image.new("1", (w, h), 255)
    red = Image.new("1", (w, h), 255)

    pixels = rgb.load()
    for y in range(h):
        for x in range(w):
            r, g, b = pixels[x, y]
            is_red = r > red_threshold and r > g + red_delta and r > b + red_delta
            if is_red:
                red.putpixel((x, y), 0)
                black.putpixel((x, y), 255)
                continue
            # Perceived brightness
            gray = int(0.299 * r + 0.587 * g + 0.114 * b)
            if gray < black_threshold:
                black.putpixel((x, y), 0)
                red.putpixel((x, y), 255)
            else:
                black.putpixel((x, y), 255)
                red.putpixel((x, y), 255)

    return black, red


def display_image(img):
    from waveshare_epd import epd7in5b_V2

    black, red = split_black_red(img)
    epd = epd7in5b_V2.EPD()
    epd.init()
    epd.Clear()

    black_buf = epd.getbuffer(black)
    red_buf = epd.getbuffer(red)

    sig = inspect.signature(epd.display)
    params = list(sig.parameters)
    if len(params) == 2:
        epd.display(black_buf, red_buf)
    elif len(params) == 1:
        epd.display([black_buf, red_buf])
    else:
        raise RuntimeError(f"Unexpected epd.display signature: {sig}")

    epd.sleep()
