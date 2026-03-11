# Habit ePaper

This repository now runs as a native Go service for lower resident memory on a Raspberry Pi Zero.

It provides:

- SQLite habit storage
- `800x480` month rendering for the Waveshare `7.5"` tri-color `epd7in5b_V2`
- A background refresh worker with a `300s` minimum refresh interval
- A simple mobile frontend at `/` with four full-screen buttons: `Workout`, `Read`, `Journal`, and `Refresh`

## Build

On the Pi:

```bash
cd /opt/habit-epaper
go build -o habit-epaper .
```

Local preview render:

```bash
go run . -render out.png
```

## Run

Without touching the panel:

```bash
HABIT_DISABLE_DISPLAY=1 go run . -addr 127.0.0.1:8000
```

With the panel attached on the Pi:

```bash
sudo ./habit-epaper
```

## API

Toggle read:

```bash
curl -X POST 'http://<pi>:8000/habit/toggle?habit=read'
```

Set workout:

```bash
curl -X POST 'http://<pi>:8000/habit/set?habit=workout&value=1'
```

Override mood:

```bash
curl -X POST 'http://<pi>:8000/mood/override?level=7'
```

Clear mood override:

```bash
curl -X POST 'http://<pi>:8000/mood/clear_override'
```

Queue refresh:

```bash
curl -X POST 'http://<pi>:8000/refresh'
```

## Mobile UI

Open `http://<pi>:8000/` from your phone or Tailscale IP.

The page is a 2x2 grid:

- top left: `Workout`
- top right: `Read`
- bottom left: `Journal`
- bottom right: `Refresh`

Each habit button toggles today's value and queues a refresh. The refresh button only queues a refresh.

## Install On Pi

Copy the repo to `/opt/habit-epaper`, then:

```bash
cd /opt/habit-epaper
go build -o habit-epaper .
sudo cp systemd/habit-epaper.service /etc/systemd/system/
sudo cp systemd/habit-epaper-refresh.service /etc/systemd/system/
sudo cp systemd/habit-epaper-refresh.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now habit-epaper.service
sudo systemctl enable --now habit-epaper-refresh.timer
systemctl list-timers --all | grep habit-epaper
```

## Tailscale

If Tailscale is running on the Pi, use:

```text
http://<tailscale-ip>:8000/
```

That gives you the same 4-button mobile control page without exposing the service publicly.
