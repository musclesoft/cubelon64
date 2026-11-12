package main

import "fmt"

const noCrossDirective = "no_cross_page_branch"

const (
	noCrossMeasure = iota + 1
	noCrossPlace
)

type noCrossBranch struct {
	next    int
	operand string
	scope   string
}

type noCrossProc struct {
	branches []noCrossBranch
	depth    int
	pad      int
	size     int
	start    int

	relativeBranches [][2]int
}

func isBranchMnemonic(mnemonic string) bool {
	switch mnemonic {
	case "BCC", "BCS", "BEQ", "BMI", "BNE", "BPL", "BVC", "BVS":
		return true
	}
	return false
}

func (a *Assembler) startNoCrossPass(phase int) {
	a.noCrossPhase = phase
	a.noCrossIndex = 0
	a.noCrossNext = nil
	a.noCrossOpen = nil
}

func (a *Assembler) noCrossAlignPass1() error {
	switch a.noCrossPhase {
	case noCrossMeasure:
		proc := &noCrossProc{start: a.pc}
		a.noCrossProcs = append(a.noCrossProcs, proc)
		a.noCrossNext = proc
	case noCrossPlace:
		proc := a.noCrossProcs[a.noCrossIndex]
		a.noCrossIndex++
		pad, err := proc.padding(a.pc)
		if err != nil {
			return err
		}
		a.pc += pad
		proc.pad = pad
		proc.start = a.pc
		a.noCrossNext = proc
	}
	if a.currentSeg != nil && a.currentSeg.Pass1Start == 0 {
		a.currentSeg.Alignment = lcm(a.currentSeg.Alignment, 256)
	}
	return nil
}

func (a *Assembler) noCrossProcStart(address int) error {
	proc := a.noCrossNext
	a.noCrossNext = nil
	a.noCrossOpen = proc
	if proc == nil {
		return nil
	}
	proc.depth = a.procDepth
	if address != proc.start {
		return fmt.Errorf(".align %s must come directly before .proc", noCrossDirective)
	}
	return nil
}

func (a *Assembler) noCrossProcEnd() {
	proc := a.noCrossOpen
	if proc == nil || proc.depth != a.procDepth {
		return
	}
	a.noCrossOpen = nil
	if a.noCrossPhase == noCrossMeasure {
		proc.size = a.pc - proc.start
	}
}

func (a *Assembler) noCrossRecordBranch(inst *InstructionStmt) {
	if a.noCrossPhase == noCrossMeasure && a.noCrossOpen != nil && isBranchMnemonic(inst.Mnemonic) {
		a.noCrossOpen.branches = append(a.noCrossOpen.branches, noCrossBranch{
			next:    a.pc + 2,
			operand: inst.Operand,
			scope:   a.lastLabel,
		})
	}
}

func (a *Assembler) resolveNoCrossBranches() error {
	for _, proc := range a.noCrossProcs {
		for _, branch := range proc.branches {
			a.lastLabel = branch.scope
			target, err := a.evalExpr(branch.operand)
			if err != nil {
				return fmt.Errorf("resolving branch target %s: %w", branch.operand, err)
			}
			if target >= proc.start && target < proc.start+proc.size {
				proc.relativeBranches = append(proc.relativeBranches, [2]int{branch.next - proc.start, target - proc.start})
			}
		}
	}
	return nil
}

func (p *noCrossProc) padding(pc int) (int, error) {
	for pad := 0; pad < 256; pad++ {
		if p.fitsAt(pc + pad) {
			return pad, nil
		}
	}
	return 0, fmt.Errorf("no placement keeps every branch of the proc at $%04X within a page", p.start)
}

func (p *noCrossProc) fitsAt(start int) bool {
	for _, branch := range p.relativeBranches {
		if (start+branch[0])>>8 != (start+branch[1])>>8 {
			return false
		}
	}
	return true
}

func (a *Assembler) noCrossAlignPass2() {
	proc := a.noCrossProcs[a.noCrossIndex]
	a.noCrossIndex++
	for i := 0; i < proc.pad; i++ {
		a.emitByte(0)
	}
	a.pc += proc.pad
	proc.start = a.currentSeg.Start + len(a.currentSeg.Data)
	a.noCrossNext = proc
}

func (a *Assembler) checkNoCrossBranch(inst *InstructionStmt) error {
	proc := a.noCrossOpen
	if proc == nil || !isBranchMnemonic(inst.Mnemonic) {
		return nil
	}
	next := a.currentSeg.Start + len(a.currentSeg.Data) + 2
	target, err := a.evalExpr(inst.Operand)
	if err != nil {
		return err
	}
	if target >= proc.start && target < proc.start+proc.size && next>>8 != target>>8 {
		return fmt.Errorf("%s %s at $%04X crosses a page to $%04X inside a proc marked %s",
			inst.Mnemonic, inst.Operand, next-2, target, noCrossDirective)
	}
	return nil
}
