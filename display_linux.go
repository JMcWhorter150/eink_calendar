//go:build linux

package main

import (
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/stianeikeland/go-rpio/v4"
	"periph.io/x/conn/v3/physic"
	"periph.io/x/conn/v3/spi"
	"periph.io/x/conn/v3/spi/spireg"
	"periph.io/x/host/v3"
)

const (
	rstPin      = 17
	dcPin       = 25
	busyPin     = 24
	pwrPin      = 18
	maxSPIWrite = 4096
)

type epd struct {
	port spi.PortCloser
	conn spi.Conn
	rst  rpio.Pin
	dc   rpio.Pin
	busy rpio.Pin
	pwr  rpio.Pin
}

func displayImage(img image.Image) error {
	if os.Getenv("HABIT_NATIVE_DISPLAY") == "1" {
		return displayImageNative(img)
	}

	black, red := splitDisplayLayers(img)
	for i := range black {
		// The vendor display method inverts the black buffer, but transmits
		// the red buffer as supplied.
		black[i] = ^black[i]
		red[i] = ^red[i]
	}

	tempDir, err := os.MkdirTemp("", "habit-epaper-display-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	blackPath := filepath.Join(tempDir, "black.bin")
	redPath := filepath.Join(tempDir, "red.bin")
	if err := os.WriteFile(blackPath, black, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(redPath, red, 0o600); err != nil {
		return err
	}

	python := os.Getenv("HABIT_PYTHON")
	if python == "" {
		python = "/usr/bin/python3"
	}
	helper := os.Getenv("HABIT_WAVESHARE_HELPER")
	if helper == "" {
		helper = "habit_epaper/display_waveshare.py"
	}

	output, err := exec.Command(python, helper, blackPath, redPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Waveshare Python driver failed: %w: %s", err, output)
	}
	return nil
}

func displayImageNative(img image.Image) error {
	if _, err := host.Init(); err != nil {
		return err
	}
	if err := rpio.Open(); err != nil {
		return err
	}
	defer rpio.Close()

	port, err := spireg.Open("/dev/spidev0.0")
	if err != nil {
		return err
	}
	defer port.Close()

	conn, err := port.Connect(4*physic.MegaHertz, spi.Mode0, 8)
	if err != nil {
		return err
	}

	dev := &epd{
		port: port,
		conn: conn,
		rst:  rpio.Pin(rstPin),
		dc:   rpio.Pin(dcPin),
		busy: rpio.Pin(busyPin),
		pwr:  rpio.Pin(pwrPin),
	}
	dev.rst.Output()
	dev.dc.Output()
	dev.busy.Input()
	dev.busy.PullDown()
	dev.pwr.Output()

	// Start every operation from a known power state. This also recovers a
	// controller left busy after an interrupted or timed-out refresh.
	dev.pwr.Low()
	time.Sleep(2 * time.Second)
	dev.pwr.High()
	defer dev.pwr.Low()
	time.Sleep(500 * time.Millisecond)

	if err := dev.init(); err != nil {
		return fmt.Errorf("initialize panel: %w", err)
	}
	if err := dev.clear(); err != nil {
		return fmt.Errorf("clear panel: %w", err)
	}
	black, red := splitDisplayLayers(img)
	if err := dev.display(black, red); err != nil {
		return fmt.Errorf("display image: %w", err)
	}
	if err := dev.sleep(); err != nil {
		return fmt.Errorf("sleep panel: %w", err)
	}
	return nil
}

func (e *epd) init() error {
	e.reset()
	if err := e.sendCommand(0x01); err != nil {
		return err
	}
	if err := e.sendDataValues(0x07, 0x07, 0x3f, 0x3f); err != nil {
		return err
	}
	if err := e.sendCommand(0x06); err != nil {
		return err
	}
	if err := e.sendDataValues(0x17, 0x17, 0x28, 0x17); err != nil {
		return err
	}
	if err := e.sendCommand(0x04); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	if err := e.waitUntilIdle(90 * time.Second); err != nil {
		return err
	}
	if err := e.sendCommand(0x00); err != nil {
		return err
	}
	if err := e.sendData(0x0F); err != nil {
		return err
	}
	if err := e.sendCommand(0x61); err != nil {
		return err
	}
	if err := e.sendDataValues(0x03, 0x20, 0x01, 0xE0); err != nil {
		return err
	}
	if err := e.sendCommand(0x15); err != nil {
		return err
	}
	if err := e.sendData(0x00); err != nil {
		return err
	}
	if err := e.sendCommand(0x50); err != nil {
		return err
	}
	if err := e.sendDataValues(0x11, 0x07); err != nil {
		return err
	}
	if err := e.sendCommand(0x60); err != nil {
		return err
	}
	return e.sendData(0x22)
}

func (e *epd) clear() error {
	rowBytes := canvasWidth / 8
	whiteBlackLayer := make([]byte, rowBytes*canvasHeight)
	whiteRedLayer := make([]byte, rowBytes*canvasHeight)
	for i := range whiteBlackLayer {
		whiteBlackLayer[i] = 0xFF
	}

	if err := e.sendCommand(0x10); err != nil {
		return err
	}
	if err := e.sendDataBytes(whiteBlackLayer); err != nil {
		return err
	}
	if err := e.sendCommand(0x13); err != nil {
		return err
	}
	if err := e.sendDataBytes(whiteRedLayer); err != nil {
		return err
	}
	if err := e.sendCommand(0x12); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return e.waitUntilIdle(180 * time.Second)
}

func (e *epd) display(black, red []byte) error {
	if err := e.sendCommand(0x10); err != nil {
		return err
	}
	if err := e.sendDataBytes(black); err != nil {
		return err
	}
	if err := e.sendCommand(0x13); err != nil {
		return err
	}
	inverted := make([]byte, len(red))
	for i, b := range red {
		inverted[i] = ^b
	}
	if err := e.sendDataBytes(inverted); err != nil {
		return err
	}
	if err := e.sendCommand(0x12); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return e.waitUntilIdle(180 * time.Second)
}

func (e *epd) sleep() error {
	if err := e.sendCommand(0x02); err != nil {
		return err
	}
	if err := e.waitUntilIdle(30 * time.Second); err != nil {
		return err
	}
	if err := e.sendCommand(0x07); err != nil {
		return err
	}
	if err := e.sendData(0xA5); err != nil {
		return err
	}
	time.Sleep(2 * time.Second)
	return nil
}

func (e *epd) reset() {
	e.rst.High()
	time.Sleep(200 * time.Millisecond)
	e.rst.Low()
	time.Sleep(4 * time.Millisecond)
	e.rst.High()
	time.Sleep(200 * time.Millisecond)
}

func (e *epd) waitUntilIdle(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if err := e.sendCommand(0x71); err != nil {
			return err
		}
		if e.busy.Read() == rpio.High {
			time.Sleep(20 * time.Millisecond)
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf(
				"e-paper busy timeout after %s waiting for BUSY GPIO %d to go high",
				timeout,
				busyPin,
			)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (e *epd) sendCommand(cmd byte) error {
	e.dc.Low()
	return e.conn.Tx([]byte{cmd}, nil)
}

func (e *epd) sendData(value byte) error {
	e.dc.High()
	return e.conn.Tx([]byte{value}, nil)
}

func (e *epd) sendDataValues(values ...byte) error {
	for _, value := range values {
		if err := e.sendData(value); err != nil {
			return err
		}
	}
	return nil
}

func (e *epd) sendDataBytes(data []byte) error {
	e.dc.High()
	packets := make([]spi.Packet, 0, (len(data)+maxSPIWrite-1)/maxSPIWrite)
	for len(data) > 0 {
		chunkSize := maxSPIWrite
		if len(data) < chunkSize {
			chunkSize = len(data)
		}
		packets = append(packets, spi.Packet{W: data[:chunkSize], KeepCS: true})
		data = data[chunkSize:]
	}
	if len(packets) == 0 {
		return nil
	}
	packets[len(packets)-1].KeepCS = false
	return e.conn.TxPackets(packets)
}
