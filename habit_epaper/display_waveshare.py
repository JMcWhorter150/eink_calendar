"""One-shot Waveshare display helper used by the Go refresh worker."""

import os
from pathlib import Path
import sys


WIDTH = 800
HEIGHT = 480
EXPECTED_LAYER_BYTES = WIDTH * HEIGHT // 8
DEFAULT_WAVESHARE_LIB = (
    "/home/josephmcwhorter/e-Paper/RaspberryPi_JetsonNano/python/lib"
)


def _read_layer(path):
    layer = bytearray(Path(path).read_bytes())
    if len(layer) != EXPECTED_LAYER_BYTES:
        raise ValueError(
            f"{path} contains {len(layer)} bytes; expected {EXPECTED_LAYER_BYTES}"
        )
    return layer


def main():
    if len(sys.argv) != 3:
        raise SystemExit("usage: display_waveshare.py BLACK_LAYER RED_LAYER")

    waveshare_lib = os.environ.get(
        "HABIT_WAVESHARE_LIB", DEFAULT_WAVESHARE_LIB
    )
    sys.path.insert(0, waveshare_lib)

    from waveshare_epd import epd7in5b_V2

    black = _read_layer(sys.argv[1])
    red = _read_layer(sys.argv[2])

    epd = epd7in5b_V2.EPD()
    print("Waveshare display: init", flush=True)
    epd.init()
    print("Waveshare display: clear", flush=True)
    epd.Clear()
    print("Waveshare display: image", flush=True)
    epd.display(black, red)
    print("Waveshare display: sleep", flush=True)
    epd.sleep()
    print("Waveshare display: done", flush=True)


if __name__ == "__main__":
    main()
