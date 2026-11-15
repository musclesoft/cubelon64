package main

import (
	"reflect"
	"testing"
)

func TestConfiguredSegmentsFollowTheirRegionsAddresses(t *testing.T) {
	config := &Config{
		Memory: map[string]MemoryRegion{
			"LOW":  {Start: 0x4400},
			"MID":  {Start: 0x5000},
			"HIGH": {Start: 0x8000},
		},
		Segments: map[string]SegmentConfig{
			"AFTER":  {Load: "HIGH"},
			"BEFORE": {Load: "LOW"},
			"MIDDLE": {Load: "MID"},
			"TIE":    {Load: "MID"},
			"ZZZ":    {Load: "LOW"},
		},
	}
	assembler := NewAssembler(config, ".")
	for name := range config.Segments {
		assembler.segments[name] = &Segment{Name: name}
	}
	got := assembler.configuredSegments(nil)
	want := []string{"BEFORE", "ZZZ", "MIDDLE", "TIE", "AFTER"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("segment order %v, want %v", got, want)
	}
}
