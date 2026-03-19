package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	output := flag.String("o", "", "Output PRG file")
	configPath := flag.String("C", "", "Linker config file (c64.cfg)")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <source.s>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	sourcePath := flag.Arg(0)

	// Default output name
	if *output == "" {
		base := filepath.Base(sourcePath)
		ext := filepath.Ext(base)
		*output = base[:len(base)-len(ext)] + ".prg"
	}

	// Default config
	if *configPath == "" {
		*configPath = "c64.cfg"
	}

	// Load linker config
	config, err := LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Assemble source file
	asm := NewAssembler(config, filepath.Dir(sourcePath))
	if err := asm.AssembleFile(sourcePath); err != nil {
		fmt.Fprintf(os.Stderr, "Error assembling: %v\n", err)
		os.Exit(1)
	}

	// Generate PRG output
	if err := asm.WritePRG(*output); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing PRG: %v\n", err)
		os.Exit(1)
	}

	// Generate metadata JSON if there are assertions
	metaPath := *output + ".meta.json"
	if err := asm.WriteMeta(metaPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing metadata: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Assembled %s -> %s\n", sourcePath, *output)
}
