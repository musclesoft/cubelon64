package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const backwardBranchBody = `@top:
    nop
    nop
    bne @top
    rts
`

const earlyBranchThenLongTailBody = `    bne @skip
    nop
@skip:
    nop
    nop
    nop
    nop
    nop
    nop
    nop
    nop
    nop
    nop
    rts
`

func assembleMarkedProc(t *testing.T, fillerBytes int, body string) *Assembler {
	t.Helper()
	source := fmt.Sprintf(`.segment "CODE"
.res %d
.align no_cross_page_branch
.proc marked
%s.endproc
after:
    rts
`, fillerBytes, body)
	path := filepath.Join(t.TempDir(), "source.s")
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig("../c64port/c64.cfg")
	if err != nil {
		t.Fatal(err)
	}
	assembler := NewAssembler(config, filepath.Dir(path))
	if err := assembler.AssembleFile(path); err != nil {
		t.Fatal(err)
	}
	return assembler
}

func TestNoCrossPageBranchMovesAProcWhoseBranchWouldCross(t *testing.T) {
	assembler := assembleMarkedProc(t, 0xFC, backwardBranchBody)
	if got := assembler.symbols["marked"]; got != 0x0900 {
		t.Fatalf("marked at $%04X, want $0900", got)
	}
}

func TestNoCrossPageBranchPadsNothingWhenNoBranchCrosses(t *testing.T) {
	assembler := assembleMarkedProc(t, 10, backwardBranchBody)
	if got := assembler.symbols["marked"]; got != 0x0801+10 {
		t.Fatalf("marked at $%04X, want $%04X", got, 0x0801+10)
	}
}

func TestNoCrossPageBranchPlacesFollowingCodeAfterThePad(t *testing.T) {
	assembler := assembleMarkedProc(t, 0xFC, backwardBranchBody)
	if got := assembler.symbols["after"]; got != 0x0900+5 {
		t.Fatalf("after at $%04X, want $%04X", got, 0x0900+5)
	}
}

func TestNoCrossPageBranchLeavesAProcStraddlingAPageWhenNoBranchCrosses(t *testing.T) {
	assembler := assembleMarkedProc(t, 0xFB, earlyBranchThenLongTailBody)
	if got := assembler.symbols["marked"]; got != 0x08FC {
		t.Fatalf("marked at $%04X, want $08FC", got)
	}
}
