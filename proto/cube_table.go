package main

import (
	"fmt"
	"math"
	"strings"
)

const (
	cubeDataPath       = "../c64port/src/cube_data.inc"
	cubeHalfLoop       = 55
	cubeLastLoopState  = 165
	cubeLoopStates     = 2 * cubeHalfLoop
	cubeRecordTurn     = 24 * cubeRecordUnits
	cubeRecordUnits    = 8
	cubeVertices       = 8
	cubeVerticesFlipYZ = 6
)

// The corners of the cube on the screen for one state: the 8 vertices, numbered x + 2y + 4z for
// the body corner at (-1 or +1) on each axis, as x and y in 1/8 pixel, which is a quarter of a
// subpixel of the 48x48 grid. The hidden vertices stay in, so the table moves smoothly.
type cubeRecord [cubeVertices][2]byte

func (camera cubeCamera) vertexRecord(rotation cubeMatrix) cubeRecord {
	var record cubeRecord
	for i := range record {
		vertex := cubeVector{-1, -1, -1}
		for axis := 0; axis < cubeAxes; axis++ {
			if i>>axis&1 == 1 {
				vertex[axis] = 1
			}
		}
		point := camera.project(rotation, vertex)
		for k := range point {
			record[i][k] = byte(math.Max(0, math.Min(255, math.Round(point[k]*cubeRecordUnits))))
		}
	}
	return record
}

func cubeStoredRecords(fit cubeFit) *[cubeHalfLoop]cubeRecord {
	var stored [cubeHalfLoop]cubeRecord
	for state := range stored {
		stored[state] = fit.Camera.vertexRecord(fit.Motion.rotationAt(float64(state)))
	}
	return &stored
}

// The second half of the animation is the first turned by 180 degrees on the screen, with the
// body's y and z axes reversed, which swaps the vertex numbers and keeps every shade in place
func (record cubeRecord) turned() cubeRecord {
	var turned cubeRecord
	for i := range turned {
		turned[i] = [2]byte{byte(cubeRecordTurn - int(record[i^cubeVerticesFlipYZ][0])), byte(cubeRecordTurn - int(record[i^cubeVerticesFlipYZ][1]))}
	}
	return turned
}

func cubeStateRecord(stored *[cubeHalfLoop]cubeRecord, state int) cubeRecord {
	inLoop := state % cubeLoopStates
	if inLoop < cubeHalfLoop {
		return stored[inLoop]
	}
	return stored[inLoop-cubeHalfLoop].turned()
}

// The 50 Hz frames between two states are halfway between them
func cubeFrameRecord(stored *[cubeHalfLoop]cubeRecord, frame int) cubeRecord {
	record := cubeStateRecord(stored, frame/2)
	if frame%2 == 0 {
		return record
	}
	next := cubeStateRecord(stored, frame/2+1)
	for i := range record {
		for k := range record[i] {
			record[i][k] = byte((int(record[i][k]) + int(next[i][k]) + 1) / 2)
		}
	}
	return record
}

func cubeFaceVertices(axis int, sign float64) [4]int {
	var vertices [4]int
	first, second := (axis+1)%cubeAxes, (axis+2)%cubeAxes
	for i, corner := range [4][2]int{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
		var bits [cubeAxes]int
		if sign > 0 {
			bits[axis] = 1
		}
		if corner[0] > 0 {
			bits[first] = 1
		}
		if corner[1] > 0 {
			bits[second] = 1
		}
		vertices[i] = bits[0] | bits[1]<<1 | bits[2]<<2
	}
	return vertices
}

func (record cubeRecord) faceCorners(axis int, sign float64) [4][2]float64 {
	var corners [4][2]float64
	for i, vertex := range cubeFaceVertices(axis, sign) {
		corners[i] = [2]float64{float64(record[vertex][0]) / cubeRecordUnits, float64(record[vertex][1]) / cubeRecordUnits}
	}
	return corners
}

// A perspective camera sees a face exactly when its corners run counterclockwise on the screen
// (clockwise for the faces at -1 of an axis, whose corners are listed from the other side), in
// the screen's y-down coordinates
func cubeFaceSeen(corners [4][2]float64, sign float64) bool {
	area := 0.0
	for i := range corners {
		next := corners[(i+1)%len(corners)]
		area += corners[i][0]*next[1] - next[0]*corners[i][1]
	}
	return area*sign > 0
}

func (record cubeRecord) seenFaces() int {
	mask := 0
	for axis := 0; axis < cubeAxes; axis++ {
		for index, sign := range [2]float64{-1, 1} {
			if cubeFaceSeen(record.faceCorners(axis, sign), sign) {
				mask |= 1 << (2*axis + index)
			}
		}
	}
	return mask
}

func (camera cubeCamera) seenFaces(rotation cubeMatrix) int {
	mask := 0
	for axis := 0; axis < cubeAxes; axis++ {
		for index, sign := range [2]float64{-1, 1} {
			var normal cubeVector
			normal[axis] = sign
			if applyCube(rotation, normal)[2] > camera.Perspective {
				mask |= 1 << (2*axis + index)
			}
		}
	}
	return mask
}

// What the C64 draws from a record: the same one-pass render as the model, with the faces
// found from the record's own corners
func (record cubeRecord) levels(turned bool) cubeLevels {
	var counts cubeCounts
	for axis := 0; axis < cubeAxes; axis++ {
		for _, sign := range [2]float64{-1, 1} {
			corners := record.faceCorners(axis, sign)
			if !cubeFaceSeen(corners, sign) {
				continue
			}
			spans := cubeFaceSpans(corners)
			counts.addFace(&spans, cubeShadeOfAxis[axis], turned)
		}
	}
	var levels cubeLevels
	for y := range levels {
		for x := range levels[y] {
			levels[y][x] = morphCoverageLevel(counts[y][x])
		}
	}
	return levels
}

// Draws every ZX state of the scene from the stored records alone, the second half and the
// repeat included, and counts what the validator counts
func validateCubeRecords(fit cubeFit, stored *[cubeHalfLoop]cubeRecord) {
	ranges := [][2]int{{0, cubeHalfLoop - 1}, {cubeHalfLoop, cubeLoopStates - 1}, {cubeLoopStates, cubeLastLoopState}}
	for _, states := range ranges {
		total, worst, worstState := 0.0, 0.0, 0
		for state := states[0]; state <= states[1]; state++ {
			screen, err := loadReferenceScreen(cubeFirstFrame + cubeFrameStep*state)
			if err != nil {
				fmt.Printf("No ZX reference for state %d: %v\n", state, err)
				return
			}
			target := screen.tile(cubeBlockOrigin, cubeBlockOrigin)
			levels := cubeStateRecord(stored, state).levels(state%cubeLoopStates >= cubeHalfLoop)
			weight := cubeValidatorMismatches(&levels, &target)
			total += weight
			if weight > worst {
				worst, worstState = weight, state
			}
		}
		fmt.Printf("Vertex table, ZX states %3d-%3d: validator total %.1f, worst block %.1f (state %d)\n", states[0], states[1], total, worst, worstState)
	}
	differing := 0
	for state := range stored {
		if stored[state].seenFaces() != fit.Camera.seenFaces(fit.Motion.rotationAt(float64(state))) {
			differing++
		}
	}
	fmt.Printf("Faces seen by the corners' winding differ from the model's in %d of %d states\n", differing, cubeHalfLoop)
}

func writeCubeData(stored *[cubeHalfLoop]cubeRecord) {
	var out strings.Builder
	out.WriteString("; Generated by proto (go run -C proto . cube), do not edit\n")
	out.WriteString("; The corners of the cube on the screen for the first 55 states of the animation (every\n")
	out.WriteString("; second frame from ZX frame 2829): per state the 8 vertices as x, y in 1/8 pixel of the\n")
	out.WriteString("; 24x24 tile, vertex number x + 2y + 4z for the body corner at -1 or +1 on each axis. Frames\n")
	out.WriteString("; between states are the mean of the two states. State N + 55 is state N turned by 180\n")
	out.WriteString("; degrees on the screen (x, y -> 192 - x, 192 - y) with vertex v read at v xor 6. A face is\n")
	out.WriteString("; seen when its corners run counterclockwise on the screen (clockwise for the faces at -1)\n\n")
	out.WriteString(".segment \"CUBEDATA\"\n\n")
	out.WriteString(".export cube_vertices\ncube_vertices:\n")
	for state, record := range stored {
		fmt.Fprintf(&out, "    ; state %d\n    .byte ", state)
		for i, vertex := range record {
			if i > 0 {
				out.WriteString(", ")
			}
			fmt.Fprintf(&out, "%d, %d", vertex[0], vertex[1])
		}
		out.WriteString("\n")
	}
	if err := writeTextFile(cubeDataPath, out.String()); err != nil {
		fmt.Printf("Warning: failed to write %s: %v\n", cubeDataPath, err)
		return
	}
	fmt.Printf("Exported %s (%d states of %d vertices, %d bytes)\n", cubeDataPath, cubeHalfLoop, cubeVertices, cubeHalfLoop*cubeVertices*2)
}
