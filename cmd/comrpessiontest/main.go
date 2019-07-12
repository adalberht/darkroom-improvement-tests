package main

import (
	//"bytes"
	"flag"
	"fmt"
	"github.com/anthonynsimon/bild/clone"
	"github.com/gojek/darkroom/pkg/processor/native"
	"github.com/gojek/darkroom/pkg/service"
	"github.com/pkg/profile"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/ioutil"
	"math"
	"os"
	"strings"
	"sync"
	"time"
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

func scanDir(path string) (files []string, hello error) {
	entries, err := ioutil.ReadDir(path)
	if err != nil {
		return
	}
	for _, r := range entries {
		n := strings.ToLower(r.Name())
		if strings.HasSuffix(n, ".jpg") {
			files = append(files, path+"/"+r.Name())
		}
	}
	return
}

type ProcessorStat struct {
	Total      time.Duration // Total duration for all files
	Minimum    time.Duration
	Maximum    time.Duration
	Processed  int // Number of processed files
	PercentSum float64
}

func (s ProcessorStat) TimeAvg() time.Duration {
	return s.Total / time.Duration(s.Processed)
}

func (s ProcessorStat) SizeAvg() float64 {
	return s.PercentSum / float64(s.Processed)
}

type ProcessorStats []*ProcessorStat

func (s ProcessorStats) WriteTo(w io.Writer) {
	formatRow := "Time (file avg): %15.3fs\n"
	fmt.Fprintf(w, "\nResults\n-------\n")
	for _, st := range s {
		fmt.Fprintf(w, "Total: %f\n", st.Total.Seconds())
		fmt.Fprintf(w, formatRow, float64(st.TimeAvg())/1e9)
		fmt.Fprintf(w, "Minimum: %fs\n", st.Minimum.Seconds())
		fmt.Fprintf(w, "Maximum: %fs\n", st.Maximum.Seconds())
		fmt.Fprintf(w, "\n")
	}
	fmt.Fprintln(w)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func Resize(m service.Manipulator, files []string) (*ProcessorStat, *ProcessorStat) {
	s1, s2 := ProcessorStat{
		Minimum: math.MaxInt64,
		Maximum: math.MinInt64,
	}, ProcessorStat{
		Minimum: math.MaxInt64,
		Maximum: math.MinInt64,
	}

	ucp, cp := native.NewBildProcessor(), native.NewBildProcessorWithCompression(&native.CompressionOptions{
		JpegQuality:         30,
		PngCompressionLevel: png.BestCompression,
	})
	m1, m2 := service.NewManipulator(ucp), service.NewManipulator(cp)

	var globalWg sync.WaitGroup
	var mutex1, mutex2 sync.Mutex
	globalWg.Add(len(files))
	for i, origPath := range files {
		go func(i int, origPath string) {
			defer globalWg.Done()

			ratio := 50.0
			data, err := ioutil.ReadFile(origPath)
			if err != nil {
				panic(err)
			}
			//img, _, _ := image.Decode(bytes.NewReader(data))
			//w, h := img.Bounds().Dx(), img.Bounds().Dy()

			params := make(map[string]string)
			params["w"] = fmt.Sprintf("%d", 300)
			//params["h"] = fmt.Sprintf("%d", h/4)
			//params["mono"] = "000000"
			//params["fit"] = "crop"
			//params["crop"] = "right"

			ps := service.ProcessSpec{
				ImageData: data,
				Params:    params,
			}

			var wg sync.WaitGroup
			var uncompressedImg, compressedImg image.Image
			var uncompressedData, compressedData []byte

			wg.Add(2)
			go func() {
				defer wg.Done()
				imgStart := time.Now()
				uncompressedData, err = m1.Process(ps)
				if err != nil {
					panic(err)
				}

				mutex1.Lock()
				dur := time.Since(imgStart)
				s1.PercentSum += ratio
				s1.Processed++
				s1.Total += dur
				if dur < s1.Minimum {
					s1.Minimum = dur
				}
				if dur > s1.Maximum {
					s1.Maximum = dur
				}
				mutex1.Unlock()

				uncompressedImg, _, _ = ucp.Decode(uncompressedData)
				uncompressedData, _ = ucp.Encode(uncompressedImg, "jpg")
			}()

			go func() {
				defer wg.Done()

				imgStart := time.Now()
				compressedData, err = m2.Process(ps)
				if err != nil {
					panic(err)
				}
				dur := time.Since(imgStart)

				mutex2.Lock()
				s2.PercentSum += ratio
				s2.Processed++
				s2.Total += dur
				if dur < s2.Minimum {
					s2.Minimum = dur
				}
				if dur > s2.Maximum {
					s2.Maximum = dur
				}
				mutex2.Unlock()

				compressedImg, _, _ = cp.Decode(compressedData)
				compressedData, _ = cp.Encode(compressedImg, "jpg")
			}()
			wg.Wait()

			sideToSideImage := createImageSideToSide(uncompressedImg, compressedImg)
			data, err = ucp.Encode(sideToSideImage, "png")
			newPath := strings.Replace(origPath, "speedtest/orig", "compressiontest/orig_compressed", 1)
			if *verbose {
				fmt.Printf("File %d w/: %s\nWithout compression: %d KB, After compression: %d KB\n\n", i+1, newPath, len(uncompressedData)/1024, len(compressedData)/1024)
			}
			ioutil.WriteFile(newPath, data, 400)
		}(i, origPath)
	}
	globalWg.Wait()
	return &s1, &s2
}

var verbose = flag.Bool("verbose", true, "Print statistics for every single file processed")

var LIMIT = 1

func main() {
	defer profile.Start().Stop()

	flag.Parse()
	dir := "/Users/pt.gojekindonesia/go/src/github.com/gojek/darkroom/cmd/speedtest/orig"
	fmt.Println(dir)
	if len(flag.Args()) > 0 {
		dir = flag.Args()[0]
	}
	files, _ := scanDir(dir)
	if len(files) == 0 {
		fmt.Println("no jpg files found in", dir)
		return
	}
	fmt.Printf("Found %d image files in %s\n", len(files), dir)
	if len(files) > LIMIT {
		files = files[0:LIMIT]
	}

	var results ProcessorStats
	ps1, ps2 := Resize(service.NewManipulator(native.NewBildProcessor()), files)
	results = append(results, ps1, ps2)
	results.WriteTo(os.Stdout)
}
