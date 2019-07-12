## Darkroom Improvement Tests Repository
This repository is used for doing research for [Darkroom](github.com/gojek/darkroom)
improvement.

## Instructions
1. `git clone github.com/adalberht/darkroom-improvement-tests`
2. `cd darkroom-improvement-tests`
3. Download [Image Manipulation Dataset](https://www5.cs.fau.de/research/data/image-manipulation/) and extract it in this folder: 
`
wget https://www5.cs.fau.de/fileadmin/research/datasets/image_forensics_dataset/forensics_database/precomputed/orig.zip &&
unzip orig.zip -d ./test-images
`
4. `go mod download`
5. `go run cmd/compressiontest/main.go orig`
