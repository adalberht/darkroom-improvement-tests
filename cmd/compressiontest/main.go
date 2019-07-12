package main

import (
	//"bytes"
	"flag"
	"fmt"
	"github.com/gojek/darkroom/pkg/processor/native"
	"github.com/gojek/darkroom/pkg/service"
	"github.com/pkg/profile"
	"image"
	"image/png"
	"io"
	"io/ioutil"
	"math"
	"os"
	"strings"
	"sync"
	"time"
)

func scanDir(path string, exts []string) (files []string, hello error) {
	entries, err := ioutil.ReadDir(path)
	if err != nil {
		return
	}
	for _, r := range entries {
		n := strings.ToLower(r.Name())
		for _, ext := range exts {
			if strings.HasSuffix(n, "."+ext) {
				files = append(files, path+"/"+r.Name())
			}
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

func (s ProcessorStats) WriteTo(w io.Writer) (int64, error) {
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
	return 1, nil
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
	var dir string
	if len(flag.Args()) > 0 {
		dir = flag.Args()[0]
	} else {
		dir = "./orig"
	}

	files, _ := scanDir(dir, []string{"jpg", "jpeg", "png"})
	if len(files) == 0 {
		fmt.Println("No supported files found in", dir)
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
