package main

import "fmt"

// playCyclesMaxBaseline is the committed upper bound on the cycles the play routine
// takes in any single frame.
const playCyclesMaxBaseline = 2090

// playCyclesTotalBaseline is the committed upper bound on the cycles the play routine
// takes over the whole song.
const playCyclesTotalBaseline = 4636850

// ValidatePlayCycles panics if the play routine's worst frame or its total over the song
// exceeds the committed baselines. Interrupts that preempt the routine are counted.
func ValidatePlayCycles(run IRQRun) {
	fmt.Printf("\n=== Validating Play Routine Cycles ===\n")
	fmt.Printf("  max per frame: %d (baseline %d), total: %d (baseline %d)\n",
		run.PlayCyclesMax, playCyclesMaxBaseline, run.PlayCyclesTotal, playCyclesTotalBaseline)
	if run.PlayCyclesMax > playCyclesMaxBaseline {
		panic(fmt.Sprintf("ValidatePlayCycles: %d cycles in the worst frame (baseline %d)", run.PlayCyclesMax, playCyclesMaxBaseline))
	}
	if run.PlayCyclesTotal > playCyclesTotalBaseline {
		panic(fmt.Sprintf("ValidatePlayCycles: %d cycles overall (baseline %d)", run.PlayCyclesTotal, playCyclesTotalBaseline))
	}
}
