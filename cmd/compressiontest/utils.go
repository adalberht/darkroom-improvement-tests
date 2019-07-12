package main

import (
	"github.com/anthonynsimon/bild/clone"
	"image"
	"image/color"
	"sync"
)

func createImageSideToSide(img1, img2 image.Image) image.Image {
	if img1.Bounds().Dy() > img2.Bounds().Dy() {
		return createImageSideToSide(img2, img1)
	}
	img1 = clone.AsRGBA(img1)
	img2 = clone.AsRGBA(img2)
	img := image.NewRGBA(image.Rectangle{
		Min: image.Point{X: 0, Y: 0},
		Max: image.Point{X: img1.Bounds().Dx() + img2.Bounds().Dx(), Y: img1.Bounds().Dy()},
	})
	var wg sync.WaitGroup
	wg.Add(img1.Bounds().Dy() + img2.Bounds().Dy())
	for y := 0; y < img1.Bounds().Dy(); y++ {
		go func(y int) {
			defer wg.Done()
			for x := 0; x < img1.Bounds().Dx(); x++ {
				r, g, b, a := img1.At(x, y).RGBA()
				img.Set(x, y, color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(a)})
			}
		}(y)
	}
	for y := 0; y < img2.Bounds().Dy(); y++ {
		go func(y int) {
			defer wg.Done()
			for x := 0; x < img2.Bounds().Dx(); x++ {
				img.Set(x+img1.Bounds().Dx(), y, img2.At(x, y))
			}
		}(y)
	}
	wg.Wait()
	return img
}
