//go:build !linux

package main

import (
	"errors"
	"image"
)

func displayImage(_ image.Image) error {
	return errors.New("display output is only supported on linux/raspberry pi builds")
}
