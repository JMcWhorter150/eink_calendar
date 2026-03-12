//go:build linux

package main

import (
	"errors"
	"image"
	"time"

	"github.com/stianeikeland/go-rpio/v4"
	"periph.io/x/conn/v3/physic"
	"periph.io/x/conn/v3/spi"
	"periph.io/x/conn/v3/spi/spireg"
	"periph.io/x/host/v3"
)

const (
	rstPin  = 17
	dcPin   = 25
	busyPin = 24
)

type epd struct {
	port spi.PortCloser
	conn spi.Conn
	rst  rpio.Pin
	dc   rpio.Pin
	busy rpio.Pin
}

func displayImage(img image.Image) error {
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
	}
	dev.rst.Output()
	dev.dc.Output()
	dev.busy.Input()

	if err := dev.init(); err != nil {
		return err
	}
	if err := dev.clear(); err != nil {
		return err
	}
	black, red := splitDisplayLayers(img)
	if err := dev.display(black, red); err != nil {
		return err
	}
	return dev.sleep()
}

func (e *epd) init() error {
	e.reset()
	if err := e.sendCommand(0x01); err != nil {
		return err
	}
	if err := e.sendDataBytes([]byte{0x07, 0x07, 0x3f, 0x3f}); err != nil {
		return err
	}
	if err := e.sendCommand(0x06); err != nil {
		return err
	}
	if err := e.sendDataBytes([]byte{0x17, 0x17, 0x28, 0x17}); err != nil {
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
	if err := e.sendDataBytes([]byte{0x03, 0x20, 0x01, 0xE0}); err != nil {
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
	if err := e.sendDataBytes([]byte{0x11, 0x07}); err != nil {
		return err
	}
	if err := e.sendCommand(0x60); err != nil {
		return err
	}
	return e.sendData(0x22)
}

func (e *epd) clear() error {
	rowBytes := canvasWidth / 8
	height := canvasHeight

	if err := e.sendCommand(0x10); err != nil {
		return err
	}
	for i := 0; i < rowBytes*height; i++ {
		if err := e.sendData(0xFF); err != nil {
			return err
		}
	}
	if err := e.sendCommand(0x13); err != nil {
		return err
	}
	for i := 0; i < rowBytes*height; i++ {
		if err := e.sendData(0x00); err != nil {
			return err
		}
	}
	if err := e.sendCommand(0x12); err != nil {
		return err
	}
	time.Sleep(10 * time.Millisecond)
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
	time.Sleep(10 * time.Millisecond)
	return e.waitUntilIdle(180 * time.Second)
}

func (e *epd) sleep() error {
	if err := e.sendCommand(0x50); err != nil {
		return err
	}
	if err := e.sendData(0xF7); err != nil {
		return err
	}
	if err := e.sendCommand(0x02); err != nil {
		return err
	}
	if err := e.waitUntilIdle(30 * time.Second); err != nil {
		return err
	}
	if err := e.sendCommand(0x07); err != nil {
		return err
	}
	return e.sendData(0xA5)
}

func (e *epd) reset() {
	e.rst.High()
	time.Sleep(200 * time.Millisecond)
	e.rst.Low()
	time.Sleep(5 * time.Millisecond)
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
			return errors.New("timed out waiting for e-paper busy pin")
		}
		time.Sleep(50 * time.Millisecond)
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

func (e *epd) sendDataBytes(data []byte) error {
	e.dc.High()
	return e.conn.Tx(data, nil)
}
