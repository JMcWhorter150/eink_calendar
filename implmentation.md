Systemd Unit Contents

  /etc/systemd/system/habit-epaper.service

  [Unit]
  Description=Habit ePaper FastAPI Service
  After=network-online.target
  Wants=network-online.target

  [Service]
  Type=simple
  WorkingDirectory=/home/jmcwhorter/Projects/calendar_display/
  eink_calendar
  ExecStart=/usr/bin/uvicorn habit_epaper.app:app --host 0.0.0.0 --port
  8000
  Restart=on-failure
  RestartSec=3
  User=pi
  Group=pi
  SupplementaryGroups=spi,gpio
  Environment=PYTHONUNBUFFERED=1

  [Install]
  WantedBy=multi-user.target

  /etc/systemd/system/habit-epaper-refresh.service

  [Unit]
  Description=Habit ePaper Daily Refresh

  [Service]
  Type=oneshot
  ExecStart=/usr/bin/curl -s -X POST http://127.0.0.1:8000/refresh

  /etc/systemd/system/habit-epaper-refresh.timer

  [Unit]
  Description=Habit ePaper Daily Refresh Timer

  [Timer]
  OnCalendar=*-*-* 06:00:00
  Persistent=true

  [Install]
  WantedBy=timers.target

  Commands to Install + Enable

  sudo tee /etc/systemd/system/habit-epaper.service >/dev/null <<'EOF'
  [Unit]
  Description=Habit ePaper FastAPI Service
  After=network-online.target
  Wants=network-online.target

  [Service]
  Type=simple
  WorkingDirectory=/home/jmcwhorter/Projects/calendar_display/
  eink_calendar
  ExecStart=/usr/bin/uvicorn habit_epaper.app:app --host 0.0.0.0 --port
  8000
  Restart=on-failure
  RestartSec=3
  User=pi
  Group=pi
  SupplementaryGroups=spi,gpio
  Environment=PYTHONUNBUFFERED=1

  [Install]
  WantedBy=multi-user.target
  EOF

  sudo tee /etc/systemd/system/habit-epaper-refresh.service >/dev/null
  <<'EOF'
  [Unit]
  Description=Habit ePaper Daily Refresh

  [Service]
  Type=oneshot
  ExecStart=/usr/bin/curl -s -X POST http://127.0.0.1:8000/refresh
  EOF

  sudo tee /etc/systemd/system/habit-epaper-refresh.timer >/dev/null
  <<'EOF'
  [Unit]
  Description=Habit ePaper Daily Refresh Timer

  [Timer]
  OnCalendar=*-*-* 06:00:00
  Persistent=true

  [Install]
  WantedBy=timers.target
  EOF

  sudo systemctl daemon-reload
  sudo systemctl enable --now habit-epaper.service
  sudo systemctl enable --now habit-epaper-refresh.timer

  How to Use

  - Install deps:

  pip install -r habit_epaper/requirements.txt

  - Local run:

  uvicorn habit_epaper.app:app --host 127.0.0.1 --port 8000

  - Optional: disable display for local dev

  HABIT_EPAPER_DISABLE_DISPLAY=1 uvicorn habit_epaper.app:app --host
  127.0.0.1 --port 8000

  Curl Examples

  curl -X POST 'http://<pi>:8000/habit/toggle?habit=read'
  curl -X POST 'http://<pi>:8000/habit/set?habit=workout&value=1'
  curl -X POST 'http://<pi>:8000/mood/override?level=7'
  curl -X POST 'http://<pi>:8000/refresh'

  Tailscale Note
  Use your Tailscale IP in place of <pi> to reach the service remotely.

  Optional Local Render Check

  - You can generate a quick visual with:

  python -c "from habit_epaper.render import render_test_png;
  print(render_test_png())"

  If you want, I can add a minimal README update or run a local render
  test and report layout observations.
