package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config represents the linker configuration
type Config struct {
	Memory   map[string]MemoryRegion
	Segments map[string]SegmentConfig
}

type MemoryRegion struct {
	Start  uint16
	Size   uint16
	Type   string // rw, ro
	File   bool   // write to output file
	Define bool
	Fill   bool
}

type SegmentConfig struct {
	Load     string // memory region to load into
	Type     string // ro, rw, bss, zp
	Define   bool
	Optional bool
	Align    int    // alignment requirement (e.g., 256 for page alignment)
}

// LoadConfig parses a simplified c64.cfg format
func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	config := &Config{
		Memory:   make(map[string]MemoryRegion),
		Segments: make(map[string]SegmentConfig),
	}

	scanner := bufio.NewScanner(f)
	var currentSection string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Remove inline comments
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if idx := strings.Index(line, ";"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}

		// Section headers
		if strings.HasPrefix(line, "MEMORY") {
			currentSection = "MEMORY"
			continue
		}
		if strings.HasPrefix(line, "SEGMENTS") {
			currentSection = "SEGMENTS"
			continue
		}

		// Skip section delimiters
		if line == "{" || line == "}" {
			continue
		}

		// Parse entries based on current section
		if currentSection == "MEMORY" {
			if err := parseMemoryLine(line, config); err != nil {
				return nil, fmt.Errorf("parsing memory line: %w", err)
			}
		} else if currentSection == "SEGMENTS" {
			if err := parseSegmentLine(line, config); err != nil {
				return nil, fmt.Errorf("parsing segment line: %w", err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return config, nil
}

func parseMemoryLine(line string, config *Config) error {
	// Format: NAME: start = $XXXX, size = $XXXX, type = ro/rw, file = %O/%s, define = yes, fill = yes
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return nil // not a memory definition
	}

	name := strings.TrimSpace(parts[0])
	attrs := parts[1]

	region := MemoryRegion{}

	// Parse attributes
	for _, attr := range strings.Split(attrs, ",") {
		attr = strings.TrimSpace(attr)
		kv := strings.SplitN(attr, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])

		switch key {
		case "start":
			v, err := parseHexOrDec(val)
			if err != nil {
				return err
			}
			region.Start = uint16(v)
		case "size":
			v, err := parseHexOrDec(val)
			if err != nil {
				return err
			}
			region.Size = uint16(v)
		case "type":
			region.Type = val
		case "file":
			region.File = (val == "%O" || val == "yes")
			// Empty string means no file output
			if val == `""` || val == "''" {
				region.File = false
			}
		case "define":
			region.Define = (val == "yes")
		case "fill":
			region.Fill = (val == "yes")
		}
	}

	config.Memory[name] = region
	return nil
}

func parseSegmentLine(line string, config *Config) error {
	// Format: NAME: load = REGION, type = ro/rw/bss/zp, define = yes
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return nil
	}

	name := strings.TrimSpace(parts[0])
	attrs := parts[1]

	segment := SegmentConfig{}

	// Parse attributes
	for _, attr := range strings.Split(attrs, ",") {
		attr = strings.TrimSpace(attr)
		kv := strings.SplitN(attr, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])

		switch key {
		case "load":
			segment.Load = val
		case "type":
			segment.Type = val
		case "define":
			segment.Define = (val == "yes")
		case "optional":
			segment.Optional = (val == "yes")
		case "align":
			align, err := parseHexOrDec(val)
			if err != nil {
				return err
			}
			segment.Align = align
		}
	}

	config.Segments[name] = segment
	return nil
}

func parseHexOrDec(s string) (int, error) {
	s = strings.TrimSpace(s)

	// Handle simple arithmetic expressions (e.g., "$A000 - $0801")
	if strings.Contains(s, "-") {
		parts := strings.Split(s, "-")
		if len(parts) == 2 {
			left, err := parseHexOrDec(strings.TrimSpace(parts[0]))
			if err != nil {
				return 0, err
			}
			right, err := parseHexOrDec(strings.TrimSpace(parts[1]))
			if err != nil {
				return 0, err
			}
			return left - right, nil
		}
	}
	if strings.Contains(s, "+") {
		parts := strings.Split(s, "+")
		if len(parts) == 2 {
			left, err := parseHexOrDec(strings.TrimSpace(parts[0]))
			if err != nil {
				return 0, err
			}
			right, err := parseHexOrDec(strings.TrimSpace(parts[1]))
			if err != nil {
				return 0, err
			}
			return left + right, nil
		}
	}

	if strings.HasPrefix(s, "$") {
		val, err := strconv.ParseInt(s[1:], 16, 64)
		return int(val), err
	}
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		val, err := strconv.ParseInt(s[2:], 16, 64)
		return int(val), err
	}
	val, err := strconv.ParseInt(s, 10, 64)
	return int(val), err
}
