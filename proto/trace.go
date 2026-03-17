package main

import "fmt"

func traceEdge01() {
	initTables()
	frameNum := byte(6) // frame 6 = 27°
	c := calcRotation(frameNum)

	// Scale 2x
	type sc struct{ xLo, xHi, yLo, yHi byte }
	var scaled [4]sc
	for i, cn := range c {
		scaled[i] = sc{cn.xLo, cn.xHi, cn.yLo, cn.yHi}
	}
	fmt.Println("Scaled 2x corners (integer coords, 48x48 space):")
	for i, s := range scaled {
		fmt.Printf("  C%d: x=%-3d  y=%d\n", i, int(int8(s.xHi)), int(int8(s.yHi)))
	}

	e := initEdge(scaled[0].xLo, scaled[0].xHi, scaled[0].yLo, scaled[0].yHi,
		scaled[1].xLo, scaled[1].xHi, scaled[1].yLo, scaled[1].yHi, nil)
	dyFull := uint16(e.dyHi)<<8 | uint16(e.dyLo)
	remFull := uint16(e.remHi)<<8 | uint16(e.remLo)
	dxAbs := uint16(e.baseStep)*dyFull + remFull // reconstruct for display only
	fmt.Printf("\nEdge 0→1: yStart=%d yEnd=%d x0=%d  dxAbs=%d dyFull=%d dxSign=%d baseStep=%d rem=%d\n",
		e.yStart, e.yEnd, e.xCurrent, dxAbs, dyFull, e.dxSign, e.baseStep, remFull)
	fmt.Printf("  true slope: %.2f X per Y\n", float64(dxAbs)/float64(dyFull))

	fmt.Printf("\n  %-5s %-8s %-16s %-10s\n", "y", "x_post", "err(hi:lo)", "true_x")
	x0Fixed := uint16(scaled[0].xHi)<<8 | uint16(scaled[0].xLo)
	x0 := float64(int16(x0Fixed)) / 256.0
	y0Fixed := uint16(scaled[0].yHi)<<8 | uint16(scaled[0].yLo)
	y0Start := float64(int16(y0Fixed)) / 256.0
	for y := e.yStart; y < e.yEnd; y++ {
		var ic byte
		var sx1, sx2 byte
		stepEdge(&e, y, &ic, &sx1, &sx2)
		trueX := x0 + (float64(y)-y0Start+1)*float64(dxAbs)/float64(dyFull)
		flag := ""
		if int(e.xCurrent) != int(trueX) {
			flag = fmt.Sprintf("← should be %d (off by %d)", int(trueX), int(e.xCurrent)-int(trueX))
		}
		fmt.Printf("  %-5d %-8d %02X:%02X            %-10.1f  %s\n",
			y, e.xCurrent, e.errHi, e.errLo, trueX, flag)
	}
}

func debugAngle2() {
	// frame 1 = 4.5° (closest to old angle=2)
	ref, refMed := refRenderSquare(1)
	var tempBuf [128]byte
	renderSquare(1, tempBuf[:])
	dda, ddaMed := unpackPrecomputed(tempBuf[:])
	fmt.Println("frame=1 (4.5°) rows 5-20  (ref | dda):")
	for row := 5; row <= 20; row++ {
		fmt.Printf("  row %2d: ref=%02X %02X %02X (med %02X %02X %02X)  dda=%02X %02X %02X (med %02X %02X %02X)\n",
			row,
			ref[row], ref[64+row], ref[128+row], refMed[row], refMed[64+row], refMed[128+row],
			dda[row], dda[64+row], dda[128+row], ddaMed[row], ddaMed[64+row], ddaMed[128+row])
	}
}

func debugA2Ref() {
	initTables()
	frameNum := byte(1) // frame 1 = 4.5°
	corners := calcRotation(frameNum)
	fmt.Println("frame 1 (4.5°) corners (Full Fixed88 in 48x48 supersample space):")
	for i, c := range corners {
		xF := int16(uint16(c.xHi)<<8 | uint16(c.xLo))
		yF := int16(uint16(c.yHi)<<8 | uint16(c.yLo))
		fmt.Printf("  C%d: x=%d.%02d (raw %04X)  y=%d.%02d (raw %04X)\n",
			i, xF>>8, (int(xF)&0xFF)*100/256, uint16(xF),
			yF>>8, (int(yF)&0xFF)*100/256, uint16(yF))
	}

	// Trace edge 3→0 (left side, active during output rows 6-12)
	fmt.Println("\nEdge 3→0 trace (supersample space):")
	e30 := initEdge(corners[3].xLo, corners[3].xHi, corners[3].yLo, corners[3].yHi,
		corners[0].xLo, corners[0].xHi, corners[0].yLo, corners[0].yHi, nil)
	dyF := uint16(e30.dyHi)<<8 | uint16(e30.dyLo)
	remF := uint16(e30.remHi)<<8 | uint16(e30.remLo)
	fmt.Printf("  yStart=%d yEnd=%d x0=%d  dy=%d dxSign=%d baseStep=%d rem=%d errInit=%02X:%02X\n",
		e30.yStart, e30.yEnd, e30.xCurrent, dyF, e30.dxSign, e30.baseStep, remF, e30.errHi, e30.errLo)
	for y := e30.yStart; y < e30.yEnd; y++ {
		var ic byte; var sx1, sx2 byte
		stepEdge(&e30, y, &ic, &sx1, &sx2)
		fmt.Printf("    ssY=%2d → x=%d (next=%d)\n", y, sx1, e30.xCurrent)
	}

	ref, _ := refRenderSquare(2)
	var tempBuf2 [128]byte
	renderSquare(2, tempBuf2[:])
	dda, _ := unpackPrecomputed(tempBuf2[:])
	fmt.Println("\nangle=2 (ref | dda)  c0  c1  c2:")
	for row := 0; row < 24; row++ {
		rc0, rc1, rc2 := ref[row], ref[64+row], ref[128+row]
		dc0, dc1, dc2 := dda[row], dda[64+row], dda[128+row]
		marker := ""
		if rc0 != dc0 || rc1 != dc1 || rc2 != dc2 {
			marker = " ← DIFF"
		}
		fmt.Printf("  row %2d:  ref=%02X %02X %02X  dda=%02X %02X %02X%s\n",
			row, rc0, rc1, rc2, dc0, dc1, dc2, marker)
	}
}

func traceMain() {
	traceEdge01()
	fmt.Println()
	debugA2Ref()
}
