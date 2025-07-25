package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	inputFile := flag.String("input-file", "", "Input file")
	outputJSON := flag.String("output-json", "", "Output JSON file")
	outputDir := flag.String("output-dir", "", "Output directory")
	flag.Parse()

	if *inputFile == "" || *outputJSON == "" || *outputDir == "" {
		log.Fatalf("Usage: %s --input-file <input_file> --output-json <output_json> --output-dir <output_dir>", os.Args[0])
	}

	mapping := map[string]string{}
	if data, err := os.ReadFile(*outputJSON); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &mapping)
	}

	filename := filepath.Base(*inputFile)
	ext := strings.TrimPrefix(filepath.Ext(filename), ".")
	filenameNoExt := strings.TrimSuffix(filename, filepath.Ext(filename))

	f, err := os.Open(*inputFile)
	if err != nil {
		log.Fatalf("Failed to open %s: %v", filename, err)
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		log.Fatalf("Failed to hash %s: %v", filename, err)
	}

	hash := hex.EncodeToString(h.Sum(nil))
	hashFilename := filenameNoExt + "." + hash + "." + ext

	if mapping[filename] == hashFilename {
		return
	}
	mapping[filename] = hashFilename

	in, err := os.Open(*inputFile)
	if err != nil {
		log.Fatalf("Failed to open %s for copy: %v", filename, err)
	}
	defer in.Close()

	outPath := filepath.Join(*outputDir, hashFilename)
	out, err := os.Create(outPath)
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer func() {
		out.Sync()
		out.Close()
	}()

	if _, err := io.Copy(out, in); err != nil {
		log.Fatalf("Failed to copy %s: %v", filename, err)
	}

	if outData, err := json.MarshalIndent(mapping, "", "  "); err == nil {
		_ = os.WriteFile(*outputJSON, outData, 0644)
	}
}
