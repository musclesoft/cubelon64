package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
)

const (
	cubeComparePath = "golden/cube_compare.png"
	cubeFirstFrame  = 2829
	cubeFitPath     = "golden/cube_fit.json"
	cubeFrameStep   = 2
	cubeStates      = 56
)

type cubeFit struct {
	Camera cubeCamera
	Motion cubeMotion
}

var cubeStartCamera = cubeCamera{CentreX: 11.925, CentreY: 11.959, HalfSize: 5.248, Perspective: 0.1232}

// Fits a 3D cube to the ZX frames of the cube scene (the block of the centre ring shows the
// animation's 55 states from frame 2829 on, half a period) and compares the model with them.
//
//	go run . cube          fits (takes minutes), writes golden/cube_fit.json and the comparison
//	go run . cube compare  only draws the comparison for the saved fit
func runCube(args []string) {
	targets, err := loadCubeTargets()
	if err != nil {
		fmt.Printf("No ZX reference: %v\n", err)
		return
	}
	var fit cubeFit
	if len(args) > 0 && args[0] == "compare" {
		if fit, err = readCubeFit(); err != nil {
			fmt.Printf("No saved fit: %v\n", err)
			return
		}
	} else {
		fit = fitCube(targets)
		if err := writeCubeFit(fit); err != nil {
			fmt.Printf("Warning: failed to write %s: %v\n", cubeFitPath, err)
		}
	}
	writeCubeComparison(targets, fit)
}

func loadCubeTargets() ([]morphTile, error) {
	targets := make([]morphTile, cubeStates)
	for state := range targets {
		screen, err := loadReferenceScreen(cubeFirstFrame + cubeFrameStep*state)
		if err != nil {
			return nil, err
		}
		targets[state] = screen.tile(cubeBlockOrigin, cubeBlockOrigin)
	}
	return targets, nil
}

func fitCube(targets []morphTile) cubeFit {
	search := cubeSearch{camera: cubeStartCamera, targets: targets}
	poses := search.fitPoses()
	fmt.Printf("Free poses: tolerant cost %.1f over %d states\n", cubeTotalCost(cubeStartCamera, targets, func(state int) cubeMatrix { return poses[state] }), cubeStates)
	motion := fitCubeMotion(poses)
	fmt.Printf("Motion fitted to the poses: %.2f degrees rms\n", motionError(motion, poses)*180/math.Pi)
	fit := cubeFit{Camera: cubeStartCamera, Motion: motion}
	fmt.Printf("Motion: tolerant cost %.1f\n", cubeFitCost(fit, targets))
	fit = polishCube(fit, targets)
	fmt.Printf("Polished with the camera: tolerant cost %.1f\n", cubeFitCost(fit, targets))
	return fit
}

func cubeFitCost(fit cubeFit, targets []morphTile) float64 {
	return cubeTotalCost(fit.Camera, targets, func(state int) cubeMatrix { return fit.Motion.rotationAt(float64(state)) })
}

func cubeTotalCost(camera cubeCamera, targets []morphTile, rotationOf func(state int) cubeMatrix) float64 {
	costs := make([]float64, len(targets))
	parallelStates(len(targets), func(state int) {
		costs[state] = camera.tolerantCost(rotationOf(state), &targets[state])
	})
	total := 0.0
	for _, cost := range costs {
		total += cost
	}
	return total
}

func writeCubeFit(fit cubeFit) error {
	data, err := json.MarshalIndent(fit, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll("golden", 0755); err != nil {
		return err
	}
	return os.WriteFile(cubeFitPath, data, 0644)
}

func readCubeFit() (cubeFit, error) {
	var fit cubeFit
	data, err := os.ReadFile(cubeFitPath)
	if err != nil {
		return fit, err
	}
	return fit, json.Unmarshal(data, &fit)
}
