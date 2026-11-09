package main

import (
	"fmt"
	"math"
	"os"

	"cubelon-emu/audiotrace"
)

const (
	maxLag      = 4
	centsLimit  = 50.0
	levelLimit  = 0.25
	channelName = "ABC"
)

type channelStats struct {
	activityMismatches int
	compared           int
	firstDivergence    int
	levelMismatches    int
	octaveShifted      int
	pitchMismatches    int
	pitchCompared      int
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: compare_audio <zx.csv> <c64.csv>")
		os.Exit(2)
	}
	zxRows, err := audiotrace.ReadCSV(os.Args[1])
	if err != nil {
		panic(err)
	}
	c64Rows, err := audiotrace.ReadCSV(os.Args[2])
	if err != nil {
		panic(err)
	}
	zxByFrame := indexByFrame(zxRows)
	c64ByFrame := indexByFrame(c64Rows)

	bestLag := 0
	bestMismatches := math.MaxInt
	fmt.Println("activity mismatches by C64 lag (C64 frame = ZX frame + lag):")
	for lag := -maxLag; lag <= maxLag; lag++ {
		total := 0
		for _, stats := range compare(zxByFrame, c64ByFrame, lag) {
			total += stats.activityMismatches
		}
		fmt.Printf("  lag %+d: %d\n", lag, total)
		if total < bestMismatches {
			bestLag = lag
			bestMismatches = total
		}
	}

	fmt.Printf("\nat best lag %+d (pitch %.0f cents, level %.2f; octave-shifted pitches are counted apart):\n", bestLag, centsLimit, levelLimit)
	for channel, stats := range compare(zxByFrame, c64ByFrame, bestLag) {
		fmt.Printf("  %c: %d frames, activity %d, level %d, pitch %d/%d (+%d octave-shifted), first divergence %d\n",
			channelName[channel], stats.compared, stats.activityMismatches, stats.levelMismatches,
			stats.pitchMismatches, stats.pitchCompared, stats.octaveShifted, stats.firstDivergence)
	}
}

func indexByFrame(rows []audiotrace.Row) map[int]audiotrace.Row {
	byFrame := make(map[int]audiotrace.Row, len(rows))
	for _, row := range rows {
		byFrame[row.Frame] = row
	}
	return byFrame
}

func compare(zxByFrame map[int]audiotrace.Row, c64ByFrame map[int]audiotrace.Row, lag int) [3]channelStats {
	var stats [3]channelStats
	for channel := range stats {
		stats[channel].firstDivergence = -1
	}
	for frame, zxRow := range zxByFrame {
		c64Row, found := c64ByFrame[frame+lag]
		if !found {
			continue
		}
		for channel := range stats {
			compareChannel(&stats[channel], frame, zxRow.Channels[channel], c64Row.Channels[channel])
		}
	}
	return stats
}

func compareChannel(stats *channelStats, frame int, zx audiotrace.Channel, c64 audiotrace.Channel) {
	stats.compared++
	zxActive := zx.Level > 0
	c64Active := c64.Level > 0
	diverged := false
	if zxActive != c64Active {
		stats.activityMismatches++
		diverged = true
	} else if zxActive {
		if math.Abs(zx.Level-c64.Level) > levelLimit {
			stats.levelMismatches++
		}
		if zx.Hz > 0 && c64.Hz > 0 {
			stats.pitchCompared++
			cents := 1200 * math.Log2(c64.Hz/zx.Hz)
			if math.Abs(cents) > centsLimit {
				if math.Abs(cents-1200*math.Round(cents/1200)) <= centsLimit {
					stats.octaveShifted++
				} else {
					stats.pitchMismatches++
					diverged = true
				}
			}
		}
	}
	if diverged && (stats.firstDivergence < 0 || frame < stats.firstDivergence) {
		stats.firstDivergence = frame
	}
}
