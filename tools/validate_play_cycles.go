package main

import "fmt"

// handlerCyclesMaxBaseline is the committed upper bound on the cycles spent in interrupt
// handlers during any single frame.
const handlerCyclesMaxBaseline = 2331

// handlerCyclesAverageBaseline is the committed upper bound on the average cycles per frame
// spent in interrupt handlers over the whole song.
const handlerCyclesAverageBaseline = 879.23

// ValidatePlayCycles panics if the interrupt handlers' worst frame or their average per frame
// exceeds the committed baselines. The handlers are the raster tick that runs the player
// plus every CIA toggle fire, wherever it lands in the frame.
func ValidatePlayCycles(run IRQRun) {
	fmt.Printf("\n=== Validating Interrupt Handler Cycles ===\n")
	average := float64(run.HandlerCyclesTotal) / float64(len(run.Frames))
	fmt.Printf("  per frame: average %.2f (baseline %.2f), max %d (baseline %d)\n",
		average, handlerCyclesAverageBaseline, run.HandlerCyclesMax, handlerCyclesMaxBaseline)
	if run.HandlerCyclesMax > handlerCyclesMaxBaseline {
		panic(fmt.Sprintf("ValidatePlayCycles: %d handler cycles in the worst frame (baseline %d)", run.HandlerCyclesMax, handlerCyclesMaxBaseline))
	}
	if average > handlerCyclesAverageBaseline {
		panic(fmt.Sprintf("ValidatePlayCycles: %.2f handler cycles per frame on average (baseline %.2f)", average, handlerCyclesAverageBaseline))
	}
}
