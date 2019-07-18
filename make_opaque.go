package main

import (
	"bytes"
	"github.com/anthonynsimon/bild/clone"
	"image"
	"image/png"
	"io/ioutil"
)

func main() {
	f, _ := ioutil.ReadFile("to-be-uploaded/sailing.png")
	img, _, _ := image.Decode(bytes.NewReader(f))
	rgba := clone.AsRGBA(img)
	for x := 0; x < img.Bounds().Dx(); x++ {
		for y := img.Bounds().Dy() - 1; y >= 9*img.Bounds().Dy()/10; y-- {
			rgba.Set(x, y, image.Transparent.C)
		}
	}
	buff := &bytes.Buffer{}
	err := png.Encode(buff, rgba)
	if err != nil {
		panic(err)
	}
	bytes := buff.Bytes()
	err = ioutil.WriteFile("to-be-uploaded/sailing_transparent.png", bytes, 777)
	if err != nil {
		panic(err)
	}
}
