package main

import (
	"fmt"
	"strings"
)

// A loop that counts an index register down to zero and repeats with BPL only works for counts up to 128: the
// negative flag is set from 129 on, so with a count of 129 or more the loop exits after its first pass. The
// check follows the immediate loaded into X and Y and fails a backward BPL to a label after which the register
// counts down with DEX or DEY (and is not loaded again) from such a count.
type countdownCheck struct {
	immediate [2]int
	history   []string
	labels    map[string]labelEntry
}

type labelEntry struct {
	historyIndex int
	immediate    [2]int
}

const indexRegisters = "XY"

func newCountdownCheck() *countdownCheck {
	return &countdownCheck{immediate: [2]int{-1, -1}, labels: map[string]labelEntry{}}
}

func (check *countdownCheck) label(name string) {
	check.labels[name] = labelEntry{historyIndex: len(check.history), immediate: check.immediate}
}

func (check *countdownCheck) instruction(inst *InstructionStmt, target string, value func(string) (int, error)) error {
	mnemonic := strings.ToUpper(inst.Mnemonic)
	if mnemonic == "BPL" {
		if err := check.backwardBranch(inst, target); err != nil {
			return err
		}
	}
	check.history = append(check.history, mnemonic)
	for register := range indexRegisters {
		name := string(indexRegisters[register])
		switch mnemonic {
		case "LD" + name:
			check.immediate[register] = -1
			if inst.AddressMode == Immediate && !strings.HasPrefix(inst.Operand, "#<") && !strings.HasPrefix(inst.Operand, "#>") {
				if loaded, err := value(strings.TrimPrefix(inst.Operand, "#")); err == nil {
					check.immediate[register] = loaded & 0xFF
				}
			}
		case "TA" + name, "IN" + name, "DE" + name, "TS" + name:
			check.immediate[register] = -1
		}
	}
	return nil
}

func (check *countdownCheck) backwardBranch(inst *InstructionStmt, target string) error {
	entry, known := check.labels[target]
	if !known {
		return nil
	}
	body := check.history[entry.historyIndex:]
	for register := range indexRegisters {
		name := string(indexRegisters[register])
		counts, loaded := false, false
		for _, mnemonic := range body {
			counts = counts || mnemonic == "DE"+name
			loaded = loaded || mnemonic == "LD"+name || mnemonic == "TA"+name || mnemonic == "TS"+name
		}
		if counts && !loaded && entry.immediate[register] > 128 {
			return fmt.Errorf("BPL %s counts %s down from #%d: the negative flag is set from 129 up, so the loop exits after one pass; count to zero and use BNE", inst.Operand, name, entry.immediate[register])
		}
	}
	return nil
}
