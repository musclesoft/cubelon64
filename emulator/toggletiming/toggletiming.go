package toggletiming

import "fmt"

// MaxDeviationBaseline is the committed upper bound, in cycles, on how late the demo's voice 1
// waveform write follows the CIA 1 Timer A underflow that calls for it. It must not increase;
// lower it when the cause is fixed.
const MaxDeviationBaseline = 539

// AverageDeviationBaseline is the committed upper bound, in cycles, on the average lateness of
// voice 1 waveform writes. It must not increase; lower it when the cause is fixed.
const AverageDeviationBaseline = 105.0

// MissedBaseline is the committed upper bound on underflows that no waveform write answers
// before the next underflow. It must not increase; lower it when the cause is fixed.
const MissedBaseline = 2

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
// the demo runs with interrupts masked.
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
	Missed         int
	TotalDeviation uint64
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
	fmt.Printf("  deviation: average %.2f cycles (baseline %.2f), max %d (baseline %d) at PAL frame %d\n",
		result.Average, AverageDeviationBaseline, result.Max, MaxDeviationBaseline, result.MaxFrame)
	if result.Max > MaxDeviationBaseline {
		panic(fmt.Sprintf("toggle timing: a toggle was %d cycles late (baseline %d)", result.Max, MaxDeviationBaseline))
	}
	if result.Average > AverageDeviationBaseline {
		panic(fmt.Sprintf("toggle timing: toggles were %.2f cycles late on average (baseline %.2f)", result.Average, AverageDeviationBaseline))
	}
	if result.Missed > MissedBaseline {
		panic(fmt.Sprintf("toggle timing: %d toggles were never written (baseline %d)", result.Missed, MissedBaseline))
	}
}
