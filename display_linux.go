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
	black, red := splitDisplayLayers(img)
	if err := dev.display(black, red); err != nil {
		return err
	}
	return dev.sleep()
}

func (e *epd) init() error {
	e.reset()
	e.sendCommand(0x01)
	e.sendDataBytes([]byte{0x07, 0x07, 0x3f, 0x3f})
	e.sendCommand(0x06)
	e.sendDataBytes([]byte{0x17, 0x17, 0x28, 0x17})
	e.sendCommand(0x04)
	time.Sleep(100 * time.Millisecond)
	if err := e.waitUntilIdle(90 * time.Second); err != nil {
		return err
	}
	e.sendCommand(0x00)
	e.sendData(0x0F)
	e.sendCommand(0x61)
	e.sendDataBytes([]byte{0x03, 0x20, 0x01, 0xE0})
	e.sendCommand(0x15)
	e.sendData(0x00)
	e.sendCommand(0x50)
	e.sendDataBytes([]byte{0x11, 0x07})
	e.sendCommand(0x60)
	e.sendData(0x22)
	return nil
}

func (e *epd) display(black, red []byte) error {
	e.sendCommand(0x10)
	e.sendDataBytes(black)
	e.sendCommand(0x13)
	inverted := make([]byte, len(red))
	for i, b := range red {
		inverted[i] = ^b
	}
	e.sendDataBytes(inverted)
	e.sendCommand(0x12)
	time.Sleep(10 * time.Millisecond)
	return e.waitUntilIdle(180 * time.Second)
}

func (e *epd) sleep() error {
	e.sendCommand(0x50)
	e.sendData(0xF7)
	e.sendCommand(0x02)
	if err := e.waitUntilIdle(30 * time.Second); err != nil {
		return err
	}
	e.sendCommand(0x07)
	e.sendData(0xA5)
	return nil
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
		e.sendCommand(0x71)
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

func (e *epd) sendCommand(cmd byte) {
	e.dc.Low()
	_ = e.conn.Tx([]byte{cmd}, nil)
}

func (e *epd) sendData(value byte) {
	e.dc.High()
	_ = e.conn.Tx([]byte{value}, nil)
}

func (e *epd) sendDataBytes(data []byte) {
	e.dc.High()
	_ = e.conn.Tx(data, nil)
}
