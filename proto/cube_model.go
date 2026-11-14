package main

import "math"

const (
	cubeAxes          = 3
	cubeBackgroundBit = 3
	cubeBlockOrigin   = 3 * morphTileSize
	cubeCheckerShade  = 1
	cubeDotsShade     = 2
	cubeLightWeight   = 3 * 2 * morphSubsamples * morphSubsamples / 4
	cubeShades        = 3
	cubeTolerantGrid  = 3
	cubeWhiteShade    = 0
)

type cubeMatrix [3][3]float64

type cubeVector [3]float64

type cubeLevels [morphTileSize][morphTileSize]int

type cubePolygon struct {
	corners [4][2]float64
	shade   int
}

type cubeCamera struct {
	CentreX, CentreY, HalfSize, Perspective float64
}

type cubeCandidates struct {
	centre [morphTileSize][morphTileSize]int
	masks  [morphTileSize][morphTileSize]uint8
}

var cubeShadeOfAxis = [cubeAxes]int{cubeDotsShade, cubeCheckerShade, cubeWhiteShade}

var cubeShadeGrey = [cubeShades]float64{0xC6, 0x5F, 0x2F}

func cubeIdentity() cubeMatrix {
	return cubeMatrix{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
}

func multiplyCube(a, b cubeMatrix) cubeMatrix {
	var product cubeMatrix
	for i := range product {
		for j := range product[i] {
			for k := range b {
				product[i][j] += a[i][k] * b[k][j]
			}
		}
	}
	return product
}

func transposeCube(m cubeMatrix) cubeMatrix {
	var transposed cubeMatrix
	for i := range m {
		for j := range m[i] {
			transposed[i][j] = m[j][i]
		}
	}
	return transposed
}

func applyCube(m cubeMatrix, v cubeVector) cubeVector {
	var result cubeVector
	for i := range result {
		for j := range v {
			result[i] += m[i][j] * v[j]
		}
	}
	return result
}

func cubeAxisRotation(axis int, angle float64) cubeMatrix {
	cos, sin := math.Cos(angle), math.Sin(angle)
	switch axis {
	case 0:
		return cubeMatrix{{1, 0, 0}, {0, cos, -sin}, {0, sin, cos}}
	case 1:
		return cubeMatrix{{cos, 0, sin}, {0, 1, 0}, {-sin, 0, cos}}
	}
	return cubeMatrix{{cos, -sin, 0}, {sin, cos, 0}, {0, 0, 1}}
}

func cubeRotationAngle(a, b cubeMatrix) float64 {
	relative := multiplyCube(transposeCube(a), b)
	cosine := (relative[0][0] + relative[1][1] + relative[2][2] - 1) / 2
	return math.Acos(math.Max(-1, math.Min(1, cosine)))
}

func (camera cubeCamera) project(rotation cubeMatrix, v cubeVector) [2]float64 {
	rotated := applyCube(rotation, v)
	scale := camera.HalfSize / (1 - camera.Perspective*rotated[2])
	return [2]float64{camera.CentreX + rotated[0]*scale, camera.CentreY + rotated[1]*scale}
}

func (camera cubeCamera) faceCorners(rotation cubeMatrix, axis int, sign float64) [4][2]float64 {
	var corners [4][2]float64
	first, second := (axis+1)%cubeAxes, (axis+2)%cubeAxes
	for i, corner := range [4][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
		var vertex cubeVector
		vertex[axis], vertex[first], vertex[second] = sign, corner[0], corner[1]
		corners[i] = camera.project(rotation, vertex)
	}
	return corners
}

// A face is seen when the camera, which sits at 1/Perspective on the view axis, is in front
// of its plane: the normal's view component must exceed Perspective, not just zero
func (camera cubeCamera) visibleFaces(rotation cubeMatrix) []cubePolygon {
	var faces []cubePolygon
	for axis := 0; axis < cubeAxes; axis++ {
		for _, sign := range [2]float64{-1, 1} {
			var normal cubeVector
			normal[axis] = sign
			if applyCube(rotation, normal)[2] <= camera.Perspective {
				continue
			}
			faces = append(faces, cubePolygon{corners: camera.faceCorners(rotation, axis, sign), shade: cubeShadeOfAxis[axis]})
		}
	}
	return faces
}

func insideCubePolygon(polygon *cubePolygon, x, y float64) bool {
	side := 0.0
	for i := range polygon.corners {
		from, to := polygon.corners[i], polygon.corners[(i+1)%len(polygon.corners)]
		cross := (to[0]-from[0])*(y-from[1]) - (to[1]-from[1])*(x-from[0])
		if cross == 0 {
			continue
		}
		if side == 0 {
			side = cross
		} else if side*cross < 0 {
			return false
		}
	}
	return true
}

func cubeShadeAt(faces []cubePolygon, x, y float64) int {
	for i := range faces {
		if insideCubePolygon(&faces[i], x, y) {
			return faces[i].shade
		}
	}
	return -1
}

func cubePixelOn(shade, x, y int) bool {
	switch shade {
	case cubeWhiteShade:
		return true
	case cubeCheckerShade:
		return (x+y)%2 == 0
	case cubeDotsShade:
		return x%2 == 0 && y%2 == 0
	}
	return false
}

func cubeCandidateOn(bit, x, y int) bool {
	return bit != cubeBackgroundBit && cubePixelOn(bit, x, y)
}

// Per pixel, every shade that covers some sample of a 3x3 grid, plus the background when
// the pixel is not fully covered, and the shade at the pixel's centre. Where dithered faces
// meet, a ZX pixel can belong to either of them, so any of them may explain it.
func (camera cubeCamera) candidates(rotation cubeMatrix) cubeCandidates {
	var result cubeCandidates
	faces := camera.visibleFaces(rotation)
	for y := range result.masks {
		for x := range result.masks[y] {
			var mask uint8
			covered := 0
			for subY := 0; subY < cubeTolerantGrid; subY++ {
				for subX := 0; subX < cubeTolerantGrid; subX++ {
					shade := cubeShadeAt(faces, float64(x)+(float64(subX)+0.5)/cubeTolerantGrid, float64(y)+(float64(subY)+0.5)/cubeTolerantGrid)
					if shade >= 0 {
						mask |= 1 << uint(shade)
						covered++
					}
				}
			}
			if covered < cubeTolerantGrid*cubeTolerantGrid {
				mask |= 1 << cubeBackgroundBit
			}
			result.masks[y][x] = mask
			result.centre[y][x] = cubeShadeAt(faces, float64(x)+0.5, float64(y)+0.5)
		}
	}
	return result
}

func (candidates *cubeCandidates) explains(x, y int, lit bool) bool {
	for bit := 0; bit <= cubeBackgroundBit; bit++ {
		if candidates.masks[y][x]&(1<<uint(bit)) != 0 && cubeCandidateOn(bit, x, y) == lit {
			return true
		}
	}
	return false
}

func (candidates *cubeCandidates) centreOn(x, y int) bool {
	return candidates.centre[y][x] >= 0 && cubePixelOn(candidates.centre[y][x], x, y)
}

// Hard: no candidate shade explains the ZX pixel. Strict: the centre sample alone does not.
func (candidates *cubeCandidates) mismatches(target *morphTile) (hard, strict int) {
	for y := range target {
		for x, lit := range target[y] {
			if !candidates.explains(x, y, lit) {
				hard++
			} else if candidates.centreOn(x, y) != lit {
				strict++
			}
		}
	}
	return hard, strict
}

func (camera cubeCamera) tolerantCost(rotation cubeMatrix, target *morphTile) float64 {
	candidates := camera.candidates(rotation)
	hard, strict := candidates.mismatches(target)
	return float64(hard) + 0.1*float64(strict)
}

// What the C64 renderer shows. The white face is light grey, the checker face dark grey and the
// dots face a checkerboard of dark grey and black (the ZX shows 100%, 50% and 25% white).
// Every pixel has 2x2 subpixels, each worth the brightness of its face (light grey twice dark
// grey; a dots subpixel is dark grey on the checkerboard's lit pixels and black elsewhere).
// A pixel is light grey from 75% of the full brightness, dark grey when anything is lit, and
// black otherwise, like the renderer's 3-4 and 1-2 subpixels.
func (camera cubeCamera) subpixelLevels(rotation cubeMatrix) cubeLevels {
	var levels cubeLevels
	faces := camera.visibleFaces(rotation)
	for y := range levels {
		for x := range levels[y] {
			weight := 0
			for subY := 0; subY < morphSubsamples; subY++ {
				for subX := 0; subX < morphSubsamples; subX++ {
					shade := cubeShadeAt(faces, float64(x)+(float64(subX)+0.5)/morphSubsamples, float64(y)+(float64(subY)+0.5)/morphSubsamples)
					weight += cubeSubpixelWeight(shade, x, y)
				}
			}
			levels[y][x] = cubeLevelOfWeight(weight)
		}
	}
	return levels
}

func cubeSubpixelWeight(shade, x, y int) int {
	switch shade {
	case cubeWhiteShade:
		return 2
	case cubeCheckerShade:
		return 1
	case cubeDotsShade:
		if (x+y)%2 == 0 {
			return 1
		}
	}
	return 0
}

func cubeLevelOfWeight(weight int) int {
	switch {
	case weight >= cubeLightWeight:
		return morphFullCoverage
	case weight > 0:
		return 1
	}
	return 0
}
