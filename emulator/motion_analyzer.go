package main

import (
	"fmt"
	"os"
	"strings"
)

// MotionPeriod represents a continuous period of motion or freeze
type MotionPeriod struct {
	Type       string // "motion" or "freeze"
	StartFrame int
	EndFrame   int
	Duration   int // in frames
}

// MotionAnalyzer tracks motion/freeze patterns across the demo
// Recommended threshold: 4 frames (filters animation technique, captures intentional pauses)
type MotionAnalyzer struct {
	periods          []MotionPeriod
	currentPeriod    *MotionPeriod
	lastChangedFrame int
	consecutiveStill int

	// Threshold: how many consecutive unchanged frames = freeze?
	// Analysis shows threshold=4 is optimal for CUBELON demo:
	// - Demo has no freeze periods between 4-10 frames
	// - Thresholds 4-8 produce identical results (314 periods)
	// - Minimum actual freeze duration: 11 frames
	freezeThreshold  int
}

func NewMotionAnalyzer(freezeThreshold int) *MotionAnalyzer {
	return &MotionAnalyzer{
		periods:         make([]MotionPeriod, 0),
		freezeThreshold: freezeThreshold,
		lastChangedFrame: -1,
		consecutiveStill: 0,
	}
}

// OnFrame is called each frame with whether it changed from previous
func (ma *MotionAnalyzer) OnFrame(frameNum int, changed bool) {
	if changed {
		// Frame changed - we're in motion
		ma.consecutiveStill = 0
		ma.lastChangedFrame = frameNum

		// Check if we need to start a new motion period
		if ma.currentPeriod == nil || ma.currentPeriod.Type == "freeze" {
			// End any freeze period
			if ma.currentPeriod != nil && ma.currentPeriod.Type == "freeze" {
				ma.currentPeriod.EndFrame = frameNum - 1
				ma.currentPeriod.Duration = ma.currentPeriod.EndFrame - ma.currentPeriod.StartFrame + 1
				ma.periods = append(ma.periods, *ma.currentPeriod)
			}
			// Start new motion period
			ma.currentPeriod = &MotionPeriod{
				Type:       "motion",
				StartFrame: frameNum,
			}
		}
	} else {
		// Frame didn't change
		ma.consecutiveStill++

		// Check if we've hit freeze threshold
		if ma.consecutiveStill >= ma.freezeThreshold {
			// We're now in a freeze period
			if ma.currentPeriod != nil && ma.currentPeriod.Type == "motion" {
				// End the motion period
				ma.currentPeriod.EndFrame = ma.lastChangedFrame
				ma.currentPeriod.Duration = ma.currentPeriod.EndFrame - ma.currentPeriod.StartFrame + 1
				ma.periods = append(ma.periods, *ma.currentPeriod)

				// Start freeze period (backdate to when motion actually stopped)
				ma.currentPeriod = &MotionPeriod{
					Type:       "freeze",
					StartFrame: ma.lastChangedFrame + 1,
				}
			} else if ma.currentPeriod == nil {
				// Demo starts with freeze
				ma.currentPeriod = &MotionPeriod{
					Type:       "freeze",
					StartFrame: frameNum - ma.consecutiveStill + 1,
				}
			}
		}
	}
}

// Finalize closes the last period
func (ma *MotionAnalyzer) Finalize(lastFrame int) {
	if ma.currentPeriod != nil {
		ma.currentPeriod.EndFrame = lastFrame
		ma.currentPeriod.Duration = ma.currentPeriod.EndFrame - ma.currentPeriod.StartFrame + 1
		ma.periods = append(ma.periods, *ma.currentPeriod)
		ma.currentPeriod = nil
	}
}

// PrintTimeline outputs the motion/freeze timeline
func (ma *MotionAnalyzer) PrintTimeline() {
	fmt.Println("\n=== MOTION/FREEZE TIMELINE ===")
	fmt.Println()

	totalDuration := 0.0
	for i, p := range ma.periods {
		seconds := float64(p.Duration) / 50.0
		totalDuration += seconds

		icon := "🎬"
		if p.Type == "freeze" {
			icon = "⏸️ "
		}

		fmt.Printf("%2d. %s %-6s frames %5d-%5d (%3d frames, %5.2fs, ends at %6.2fs)\n",
			i+1, icon, p.Type, p.StartFrame, p.EndFrame, p.Duration, seconds, totalDuration)
	}

	fmt.Printf("\nTotal: %d periods, %.2fs\n", len(ma.periods), totalDuration)

	// Statistics
	motionFrames := 0
	freezeFrames := 0
	motionPeriods := 0
	freezePeriods := 0

	for _, p := range ma.periods {
		if p.Type == "motion" {
			motionFrames += p.Duration
			motionPeriods++
		} else {
			freezeFrames += p.Duration
			freezePeriods++
		}
	}

	totalFrames := motionFrames + freezeFrames
	motionPercent := 100.0 * float64(motionFrames) / float64(totalFrames)

	fmt.Printf("\nStatistics:\n")
	fmt.Printf("  Motion: %d frames (%.1f%%) in %d periods\n", motionFrames, motionPercent, motionPeriods)
	fmt.Printf("  Freeze: %d frames (%.1f%%) in %d periods\n", freezeFrames, 100.0-motionPercent, freezePeriods)
	fmt.Printf("  Avg motion period: %.1f frames (%.2fs)\n",
		float64(motionFrames)/float64(motionPeriods), float64(motionFrames)/float64(motionPeriods)/50.0)
	if freezePeriods > 0 {
		fmt.Printf("  Avg freeze period: %.1f frames (%.2fs)\n",
			float64(freezeFrames)/float64(freezePeriods), float64(freezeFrames)/float64(freezePeriods)/50.0)
	}
}

// ExportScript outputs a compact script representation
func (ma *MotionAnalyzer) ExportScript(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "# CUBELON Demo Motion Script\n")
	fmt.Fprintf(f, "# Format: TYPE START_FRAME END_FRAME DURATION(frames) DURATION(seconds)\n")
	fmt.Fprintf(f, "#\n")
	fmt.Fprintf(f, "# Generated from ZX Spectrum 128K demo frame analysis\n")
	fmt.Fprintf(f, "# Threshold: %d consecutive unchanged frames = freeze\n", ma.freezeThreshold)
	fmt.Fprintf(f, "#\n\n")

	for _, p := range ma.periods {
		seconds := float64(p.Duration) / 50.0
		fmt.Fprintf(f, "%-6s %5d %5d %4d %.2f\n",
			strings.ToUpper(p.Type), p.StartFrame, p.EndFrame, p.Duration, seconds)
	}

	return nil
}

// ExportC64Data generates a C64-friendly binary table
func (ma *MotionAnalyzer) ExportC64Data(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "; CUBELON C64 Demo Sequence Table\n")
	fmt.Fprintf(f, "; Generated from ZX Spectrum 128K analysis\n")
	fmt.Fprintf(f, ";\n")
	fmt.Fprintf(f, "; Format: [frame_hi, frame_lo, type, duration_hi, duration_lo]\n")
	fmt.Fprintf(f, "; Type: 0=freeze, 1=motion\n")
	fmt.Fprintf(f, ";\n\n")

	fmt.Fprintf(f, ".export demo_sequence_table\n")
	fmt.Fprintf(f, "demo_sequence_table:\n")

	for i, p := range ma.periods {
		typeVal := byte(0)
		if p.Type == "motion" {
			typeVal = 1
		}

		startHi := byte(p.StartFrame >> 8)
		startLo := byte(p.StartFrame & 0xFF)
		durHi := byte(p.Duration >> 8)
		durLo := byte(p.Duration & 0xFF)

		comment := fmt.Sprintf("; %2d: %s %d-%d (%d frames)",
			i+1, p.Type, p.StartFrame, p.EndFrame, p.Duration)

		fmt.Fprintf(f, "    .byte $%02X, $%02X, $%02X, $%02X, $%02X  %s\n",
			startHi, startLo, typeVal, durHi, durLo, comment)
	}

	fmt.Fprintf(f, "    .byte $FF, $FF, $FF, $FF, $FF  ; End marker\n")

	return nil
}

// VisualizeTimeline creates a visual representation
func (ma *MotionAnalyzer) VisualizeTimeline(width int) {
	if len(ma.periods) == 0 {
		return
	}

	lastFrame := ma.periods[len(ma.periods)-1].EndFrame
	framesPerChar := float64(lastFrame) / float64(width)

	fmt.Println("\n=== VISUAL TIMELINE ===")
	fmt.Printf("Scale: each character = %.1f frames (%.2fs)\n\n", framesPerChar, framesPerChar/50.0)

	// Build character array
	timeline := make([]rune, width)
	for i := range timeline {
		timeline[i] = '·'
	}

	for _, p := range ma.periods {
		startChar := int(float64(p.StartFrame) / framesPerChar)
		endChar := int(float64(p.EndFrame) / framesPerChar)

		if endChar >= width {
			endChar = width - 1
		}

		char := '█' // motion
		if p.Type == "freeze" {
			char = '─'
		}

		for i := startChar; i <= endChar && i < width; i++ {
			timeline[i] = char
		}
	}

	// Print with frame markers
	for i := 0; i < width; i += 10 {
		frame := int(float64(i) * framesPerChar)
		fmt.Printf("%-10d", frame)
	}
	fmt.Println()

	fmt.Println(string(timeline))

	fmt.Println("\nLegend: █ = motion, ─ = freeze")
}
