package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/anthonynsimon/bild/clone"
	"github.com/gojek/darkroom/pkg/processor/native"
	"github.com/gojek/darkroom/pkg/service"
	"github.com/pkg/profile"
	"image"
	"image/png"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
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
	Name       string
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
	_, _ = fmt.Fprintf(w, "\nResults\n-------\n")
	for _, st := range s {
		_, _ = fmt.Fprintf(w, "Processor Name: %s", st.Name)
		_, _ = fmt.Fprintf(w, "Total: %f\n", st.Total.Seconds())
		_, _ = fmt.Fprintf(w, formatRow, float64(st.TimeAvg())/1e9)
		_, _ = fmt.Fprintf(w, "Minimum: %fs\n", st.Minimum.Seconds())
		_, _ = fmt.Fprintf(w, "Maximum: %fs\n", st.Maximum.Seconds())
		_, _ = fmt.Fprintf(w, "\n")
	}
	_, _ = fmt.Fprintln(w)
	return 1, nil
}

func Resize(files []string, compressionLevel int) (*ProcessorStat, *ProcessorStat) {
	s1, s2 := ProcessorStat{
		Minimum: math.MaxInt64,
		Maximum: math.MinInt64,
		Name:    "Uncompressed (Quality 100)",
	}, ProcessorStat{
		Minimum: math.MaxInt64,
		Maximum: math.MinInt64,
		Name:    fmt.Sprintf("Compressed (Quality %d)", compressionLevel),
	}

	ucp, cp := native.NewBildProcessorWithCompression(&native.CompressionOptions{
		JpegQuality:         100,
		PngCompressionLevel: png.BestCompression,
	}), native.NewBildProcessorWithCompression(&native.CompressionOptions{
		JpegQuality:         compressionLevel,
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
				uncompressedImg, _, _ = ucp.Decode(uncompressedData)
				uncompressedImg = clone.AsRGBA(uncompressedImg)
				y := uncompressedImg.Bounds().Dy()
				addLabel(uncompressedImg.(*image.RGBA), 10, 9*y/10, fmt.Sprintf("Quality 100: %d KB", len(uncompressedData)/1024))
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
				compressedImg, _, _ = cp.Decode(compressedData)
				compressedImg = clone.AsRGBA(compressedImg)
				y := compressedImg.Bounds().Dy()
				addLabel(compressedImg.(*image.RGBA), 10, 9*y/10, fmt.Sprintf("Quality %d: %d KB", compressionLevel, len(compressedData)/1024))
			}()
			wg.Wait()

			sideToSideImage := createImageSideToSide(uncompressedImg, compressedImg)
			data, err = ucp.Encode(sideToSideImage, "png")

			if *verbose {
				fmt.Printf("File %d w/: %s\nWithout compression: %d KB, After compression: %d KB\n\n",
					i+1, origPath, len(uncompressedData)/1024, len(compressedData)/1024)
			}
			newPath := OutPath + "/" + filepath.Base(origPath)
			fmt.Println("Writing to " + newPath)
			err = ioutil.WriteFile(OutPath+"/"+filepath.Base(origPath), data, 400)
			if err != nil {
				panic(err)
			}
		}(i, origPath)
	}
	globalWg.Wait()
	return &s1, &s2
}

var verbose = flag.Bool("verbose", true, "Print statistics for every single file processed")

var LIMIT = 10
var InPath = "./test-images"
var OutPath = "./compression-test-out"

func main() {
	defer profile.Start().Stop()

	flag.Parse()
	if len(flag.Args()) > 0 {
		InPath = flag.Args()[0]
	}

	files, _ := scanDir(InPath, []string{"jpg", "jpeg", "png"})
	if len(files) == 0 {
		fmt.Println("No supported files found in", InPath)
		return
	}
	fmt.Printf("Found %d image files in %s\n", len(files), InPath)
	if len(files) > LIMIT {
		files = files[0:LIMIT]
	}

	var results ProcessorStats
	prevPath := OutPath
	for i := 0; i <= 100; i += 5 {
		OutPath = fmt.Sprintf("%s/quality_%d", prevPath, i)
		_ = os.MkdirAll(OutPath, 0777)
		ps1, ps2 := Resize(files, i)
		results = append(results, ps1, ps2)
		f, err := os.Create(OutPath + "/" + "stats.txt")
		if err != nil {
			panic(err)
		}
		f.Sync()
		w := bufio.NewWriter(f)
		_, _ = results.WriteTo(w)
		w.Flush()
		f.Close()
	}
}
