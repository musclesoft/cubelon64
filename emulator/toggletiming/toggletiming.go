package toggletiming

import (
	"fmt"
	"math"
)

// MaxJitterBaseline is the committed upper bound, in cycles, on the spread between the earliest
// and the latest delay from a CIA 1 Timer A underflow to the voice 1 waveform write it calls
// for. The earliest delay is a constant offset that does not make the waveform toggle unevenly;
// the spread does. It must not increase; lower it when the cause is fixed.
const MaxJitterBaseline = 209

// AverageJitterBaseline is the committed upper bound, in cycles, on the average delay above the
// earliest delay. It must not increase; lower it when the cause is fixed.
const AverageJitterBaseline = 54.26

// MissedBaseline is the committed upper bound on underflows that no waveform write answers
// before the next underflow. It must not increase; lower it when the cause is fixed.
const MissedBaseline = 1

const sidCyclesPerFrame = 312 * 63

type underflow struct {
	cycle       uint64
	expected    byte
	alternating bool
}

type ctrlWrite struct {
	cycle    uint64
	value    byte
	previous byte
}

// Recorder collects CIA 1 Timer A underflows and voice 1 control register writes while the demo
// runs, and measures afterwards how long each toggle took to reach the SID. The underflow is
// when the toggle is supposed to happen; the delay is interrupt latency plus any stretch where
// the demo runs with interrupts masked. Only the variation of the delay matters, not its floor.
type Recorder struct {
	readCtrl   func() (ctrlA, ctrlB byte)
	underflows []underflow
	writes     []ctrlWrite
	lastCtrl   byte
}

// New returns a Recorder. readCtrl returns the CtrlA and CtrlB bytes the player's CIA handler
// chooses between: CtrlA while the Timer A pin is high, CtrlB while it is low.
func New(readCtrl func() (ctrlA, ctrlB byte)) *Recorder {
	return &Recorder{readCtrl: readCtrl}
}

// Underflow is the Timer A underflow callback.
func (r *Recorder) Underflow(cycle uint64, pinHigh bool) {
	ctrlA, ctrlB := r.readCtrl()
	expected := ctrlB
	if pinHigh {
		expected = ctrlA
	}
	r.underflows = append(r.underflows, underflow{
		alternating: ctrlA&0x01 != 0 && ctrlB != 0 && ctrlA != ctrlB,
		cycle:       cycle,
		expected:    expected,
	})
}

// CtrlWrite records a write of value to the voice 1 control register at cycle.
func (r *Recorder) CtrlWrite(cycle uint64, value byte) {
	r.writes = append(r.writes, ctrlWrite{cycle: cycle, previous: r.lastCtrl, value: value})
	r.lastCtrl = value
}

type Result struct {
	Answered       int
	Average        float64
	Max            uint64
	MaxFrame       int
	Min            uint64
	Missed         int
	TotalDeviation uint64
}

func (r Result) MaxJitter() uint64 {
	return r.Max - r.Min
}

func (r Result) AverageJitter() float64 {
	return r.Average - float64(r.Min)
}

// Measure matches every underflow that should change the waveform with the first write of the
// expected waveform before the next underflow.
func (r *Recorder) Measure() Result {
	var result Result
	next := 0
	for i, current := range r.underflows {
		for next < len(r.writes) && r.writes[next].cycle < current.cycle {
			next++
		}
		if !current.alternating {
			continue
		}
		windowEnd := ^uint64(0)
		if i+1 < len(r.underflows) {
			windowEnd = r.underflows[i+1].cycle
		}
		answer, found := r.firstExpectedWrite(next, current, windowEnd)
		if !found {
			result.Missed++
			continue
		}
		deviation := answer.cycle - current.cycle
		if result.Answered == 0 || deviation < result.Min {
			result.Min = deviation
		}
		result.Answered++
		result.TotalDeviation += deviation
		if deviation > result.Max {
			result.Max = deviation
			result.MaxFrame = int(current.cycle / sidCyclesPerFrame)
		}
	}
	if result.Answered > 0 {
		result.Average = float64(result.TotalDeviation) / float64(result.Answered)
	}
	return result
}

func (r *Recorder) firstExpectedWrite(start int, current underflow, windowEnd uint64) (ctrlWrite, bool) {
	for _, write := range r.writes[start:] {
		if write.cycle >= windowEnd {
			break
		}
		if write.value == current.expected && write.previous != write.value {
			return write, true
		}
	}
	return ctrlWrite{}, false
}

// Validate prints the measurement and panics if it is worse than the committed baselines.
func (r *Recorder) Validate() {
	result := r.Measure()
	fmt.Printf("\n=== Validating Voice 1 Toggle Timing Against CIA Underflows ===\n")
	fmt.Printf("  %d toggles answered, %d missed (baseline %d)\n", result.Answered, result.Missed, MissedBaseline)
	fmt.Printf("  delay: min %d, average %.2f, max %d (worst at PAL frame %d) cycles\n",
		result.Min, result.Average, result.Max, result.MaxFrame)
	fmt.Printf("  jitter: average %.2f cycles (baseline %.2f), max %d (baseline %d)\n",
		result.AverageJitter(), AverageJitterBaseline, result.MaxJitter(), MaxJitterBaseline)
	if result.MaxJitter() > MaxJitterBaseline {
		panic(fmt.Sprintf("toggle timing: %d cycles of jitter (baseline %d)", result.MaxJitter(), MaxJitterBaseline))
	}
	if result.AverageJitter() > AverageJitterBaseline {
		panic(fmt.Sprintf("toggle timing: %.2f cycles of average jitter (baseline %.2f)", result.AverageJitter(), AverageJitterBaseline))
	}
	if result.Missed > MissedBaseline {
		panic(fmt.Sprintf("toggle timing: %d toggles were never written (baseline %d)", result.Missed, MissedBaseline))
	}
	announceImprovements(result)
}

func announceImprovements(result Result) {
	if result.MaxJitter() < MaxJitterBaseline {
		announce(fmt.Sprintf("max jitter is %d cycles, below the baseline of %d: set MaxJitterBaseline to %d", result.MaxJitter(), MaxJitterBaseline, result.MaxJitter()))
	}
	if result.AverageJitter() < AverageJitterBaseline-0.01 {
		announce(fmt.Sprintf("average jitter is %.2f cycles, below the baseline of %.2f: set AverageJitterBaseline to %.2f", result.AverageJitter(), AverageJitterBaseline, math.Ceil(result.AverageJitter()*100)/100))
	}
	if result.Missed < MissedBaseline {
		announce(fmt.Sprintf("%d toggles were never written, below the baseline of %d: set MissedBaseline to %d", result.Missed, MissedBaseline, result.Missed))
	}
}

func announce(message string) {
	fmt.Printf("\n🚀🚀🚀 TOGGLE TIMING IMPROVED 🚀🚀🚀\n  %s\n  Keep this improvement: commit the change and update the baseline so it cannot regress.\n\n", message)
}
