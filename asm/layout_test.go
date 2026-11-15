package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func assembleSource(t *testing.T, source string) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.s")
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig("../c64port/c64.cfg")
	if err != nil {
		t.Fatal(err)
	}
	return NewAssembler(config, filepath.Dir(path)).AssembleFile(path)
}

func TestSymbolsMayNotBeDefinedTwice(t *testing.T) {
	for name, source := range map[string]string{
		"constants":          "VALUE = 1\nVALUE = 2\n",
		"equal constants":    "VALUE = 1\nVALUE = 1\n",
		"labels":             ".segment \"CODE\"\nagain:\n    rts\nagain:\n    rts\n",
		"proc and constant":  ".segment \"CODE\"\n.proc same\n    rts\n.endproc\nsame = 3\n",
		"label and constant": ".segment \"CODE\"\nsame = 3\nsame:\n    rts\n",
	} {
		err := assembleSource(t, source)
		if err == nil || !strings.Contains(err.Error(), "defined twice") {
			t.Errorf("%s: error %v, want one about a symbol defined twice", name, err)
		}
	}
}

func TestLocalLabelsOfDifferentProcsAreNotDuplicates(t *testing.T) {
	source := ".segment \"CODE\"\n.proc first\n@done:\n    rts\n.endproc\n.proc second\n@done:\n    rts\n.endproc\n"
	if err := assembleSource(t, source); err != nil {
		t.Fatal(err)
	}
}

func TestBplCountdownFromAboveTheNegativeLimitFails(t *testing.T) {
	for name, source := range map[string]string{
		"x": ".segment \"CODE\"\nstart:\n    ldx #143\n@byte:\n    lda $2000,x\n    sta $3000,x\n    dex\n    bpl @byte\n    rts\n",
		"y": ".segment \"CODE\"\nstart:\n    ldy #255\n@byte:\n    sta $3000,y\n    dey\n    bpl @byte\n    rts\n",
	} {
		err := assembleSource(t, source)
		if err == nil || !strings.Contains(err.Error(), "negative flag") {
			t.Errorf("%s: error %v, want one about the negative flag", name, err)
		}
	}
}

func TestBplCountdownThatWorksIsAccepted(t *testing.T) {
	for name, source := range map[string]string{
		"largest count": ".segment \"CODE\"\nstart:\n    ldx #128\n@byte:\n    sta $3000,x\n    dex\n    bpl @byte\n    rts\n",
		"small count":   ".segment \"CODE\"\nstart:\n    ldx #23\n@byte:\n    sta $3000,x\n    dex\n    bpl @byte\n    rts\n",
		"loaded again":  ".segment \"CODE\"\nstart:\n    ldx #200\n@byte:\n    ldx #5\n    dex\n    bpl @byte\n    rts\n",
		"counted up":    ".segment \"CODE\"\nstart:\n    ldx #200\n@byte:\n    sta $3000,x\n    inx\n    bpl @byte\n    rts\n",
		"count to zero": ".segment \"CODE\"\nstart:\n    ldx #143\n@byte:\n    sta $3000,x\n    dex\n    bne @byte\n    rts\n",
	} {
		if err := assembleSource(t, source); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
